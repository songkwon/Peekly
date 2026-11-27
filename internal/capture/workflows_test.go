package capture

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestMobileSetupAndHTTPRewrite(t *testing.T) {
	engine := testEngine(t)
	client := clientFor(t, engine, nil)
	res, err := client.Get("http://peekly.test/")
	if got := responseText(t, res, err); !strings.Contains(got, "已通过 Peekly 连接") {
		t.Fatal("missing proxy connection confirmation")
	}
	res, err = client.Get("http://peekly.test/Peekly.cer")
	cert, err := x509.ParseCertificate([]byte(responseText(t, res, err)))
	if err != nil || !cert.IsCA {
		t.Fatal("setup endpoint must return a DER public root certificate")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, r.Header.Get("X-Peekly-Debug")+":"+string(body))
	}))
	defer server.Close()
	source := `function onRequest(request) {
		if (typeof fetch !== "undefined" || typeof require !== "undefined") throw new Error("unexpected API");
		request.headers["X-Peekly-Debug"] = ["enabled"];
		request.body = request.body.toUpperCase();
		return request;
	}
	function onResponse(response) {
		response.status = 201;
		response.headers["X-Peekly-Response"] = ["script"];
		response.body += "!";
		return response;
	}`
	rules := []Rule{{Enabled: true, Kind: "script", Match: "contains", Pattern: server.URL + "/test", Value: source}}
	if err := engine.SetTrafficRules(rules); err != nil {
		t.Fatal(err)
	}
	res, err = client.Post(server.URL+"/test", "text/plain", strings.NewReader("hi"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != 201 || res.Header.Get("X-Peekly-Response") != "script" || string(raw) != "enabled:HI!" {
		t.Fatalf("script response: %d, %q, %v", res.StatusCode, raw, err)
	}
	detail := waitForDetail(t, engine, 1)
	if detail.Client == nil || detail.Client.Address != "127.0.0.1" || detail.Rule != "script" || detail.ScriptError != "" {
		t.Fatalf("script metadata: %#v, %s", detail.Row, detail.ScriptError)
	}
	t.Logf("loopback client: %#v", detail.Client)
	label := detail.Client.Name
	if label == "" {
		label = detail.Client.Address
		if detail.Client.PID != 0 {
			label = "PID " + strconv.Itoa(detail.Client.PID)
		}
	}
	ids, err := engine.Matches([]Filter{{Field: "client", Op: "is", Value: label}}, false)
	if err != nil || len(ids) != 1 || ids[0] != detail.ID {
		t.Fatalf("exact client filter did not match: %v, %v", ids, err)
	}
	rules[0].Value = `function onRequest(request) { for (;;) {} }`
	if err := engine.SetTrafficRules(rules); err != nil {
		t.Fatal(err)
	}
	res, err = client.Post(server.URL+"/test", "text/plain", strings.NewReader("still forwarded"))
	if got := responseText(t, res, err); got != ":still forwarded" {
		t.Fatal(got)
	}
	if got := waitForDetail(t, engine, 2).ScriptError; !strings.Contains(got, "脚本运行超时") {
		t.Fatalf("script timeout was not recorded: %q", got)
	}
}

func TestSecureWebSocketFramesThroughAllDomains(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		for {
			kind, text, err := conn.Read(r.Context())
			if err != nil || conn.Write(r.Context(), kind, text) != nil {
				return
			}
		}
	}))
	defer server.Close()
	engine := testEngine(t)
	trust := x509.NewCertPool()
	trust.AddCert(server.Certificate())
	engine.transport.TLSClientConfig = &tls.Config{RootCAs: trust, MinVersion: tls.VersionTLS12}
	if _, err := engine.SetRules([]string{"*"}); err != nil {
		t.Fatal(err)
	}
	root, err := engine.Certificate()
	if err != nil || !trust.AppendCertsFromPEM([]byte(root.PEM)) {
		t.Fatal("cannot trust test certificate", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, reply, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(server.URL, "https")+"/socket", &websocket.DialOptions{HTTPClient: clientFor(t, engine, trust)})
	if err != nil {
		t.Fatalf("dial: %v; reply: %v; rows: %#v", err, reply, engine.Snapshot(0).Rows)
	}
	defer conn.CloseNow()
	if err := conn.Write(ctx, websocket.MessageText, []byte("echo")); err != nil {
		t.Fatal(err)
	}
	if _, body, err := conn.Read(ctx); err != nil || string(body) != "echo" {
		t.Fatalf("echo %q: %v", body, err)
	}
	detail := waitForDetail(t, engine, 1)
	if detail.Kind != "WebSocket" || detail.Status != 101 || len(detail.Frames) < 2 ||
		detail.Frames[0].Direction != "send" || detail.Frames[1].Direction != "receive" ||
		string(detail.Frames[0].Data) != "echo" || string(detail.Frames[1].Data) != "echo" {
		t.Fatalf("unexpected WebSocket capture: %#v, %#v", detail.Row, detail.Frames)
	}
	engine.Stop()
	if _, _, err := conn.Read(ctx); err == nil || ctx.Err() != nil {
		t.Fatalf("stopping the proxy did not close the WSS relay promptly: %v", err)
	}
}
