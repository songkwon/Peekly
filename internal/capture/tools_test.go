package capture

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestComposeRepeatRulesAndHAR(t *testing.T) {
	e := testEngine(t)
	if err := e.SetTrafficRules([]Rule{
		{Enabled: true, Kind: "mock", Match: "contains", Pattern: "/example", Number: 201, Value: `{"ok":true}`},
		{Enabled: true, Kind: "response-set", Match: "glob", Pattern: "http://*/example*", Name: "X-Test", Value: "local"},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := e.Compose(context.Background(), RequestInput{Method: "POST", URL: "http://localhost:8800/example?token=secret", Headers: http.Header{"Authorization": {"Bearer fake-token"}}, Body: `{"privateData":42}`})
	if err != nil || result.Status != 201 || string(result.ResponseBody) != `{"ok":true}` || result.ResponseHeaders.Get("X-Test") != "local" {
		t.Fatalf("compose %#v: %v", result.Row, err)
	}
	repeated, err := e.Repeat(context.Background(), result.ID)
	if err != nil || repeated.ID == result.ID || string(repeated.RequestBody) != string(result.RequestBody) {
		t.Fatalf("repeat %#v: %v", repeated.Row, err)
	}
	ids, err := e.Matches([]Filter{{Field: "requestBody", Op: "contains", Value: "privateData"}, {Field: "status", Op: "is", Value: "201"}}, false)
	if err != nil || len(ids) != 2 {
		t.Fatalf("filters returned %v, %v", ids, err)
	}
	report, err := e.ExportHAR(ids, true)
	for _, secret := range []string{"secret", "fake-token", "privateData"} {
		if strings.Contains(string(report), secret) {
			t.Errorf("redacted export contained %q", secret)
		}
	}
	if err != nil || !strings.Contains(string(report), "_peeklyRedacted") {
		t.Fatal("missing redaction flag", err)
	}
	imported := New(t.TempDir())
	count, err := imported.ImportHAR(report)
	if err != nil || count != 2 {
		t.Fatal(count, err)
	}
	if got := waitForDetail(t, imported, 2); got.Source != "HAR" || got.Status != 201 {
		t.Fatal("invalid imported row", got.Row)
	}
}

func TestResponseBreakpointChangesStatusHeadersAndBody(t *testing.T) {
	e := testEngine(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "before")
	}))
	defer target.Close()
	if err := e.SetTrafficRules([]Rule{{Enabled: true, Kind: "break-response", Match: "contains", Pattern: "/pause"}}); err != nil {
		t.Fatal(err)
	}
	client := clientFor(t, e, nil)
	type reply struct {
		status int
		body   string
		header string
		err    error
	}
	response := make(chan reply, 1)
	go func() {
		r, err := client.Get(target.URL + "/pause")
		if err != nil {
			response <- reply{err: err}
			return
		}
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		response <- reply{r.StatusCode, string(body), r.Header.Get("X-Breakpoint"), err}
	}()
	var id uint64
	for range 100 {
		rows := e.Snapshot(0).Rows
		if len(rows) == 1 && rows[0].Breakpoint == "response" {
			id = rows[0].ID
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if id == 0 {
		t.Fatal("response did not pause")
	}
	if err := e.ResolveBreakpoint(id, BreakpointAction{Action: "continue", ReplaceBody: true, Body: "after", Status: 418, Headers: http.Header{"X-Breakpoint": {"checked"}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-response:
		if got.err != nil || got.status != 418 || got.body != "after" || got.header != "checked" {
			t.Fatal(got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("response did not continue")
	}
}

func TestHTTPSNegotiatesHTTP2AndCapturesTrailers(t *testing.T) {
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("upstream %s", r.Proto)
		}
		w.Header().Set("Trailer", "Grpc-Status")
		w.Header().Set("Content-Type", "application/grpc")
		_, _ = w.Write([]byte{0, 0, 0, 0, 1, 1})
		w.Header().Set("Grpc-Status", "0")
	}))
	target.EnableHTTP2 = true
	target.StartTLS()
	defer target.Close()
	e := testEngine(t)
	roots := x509.NewCertPool()
	roots.AddCert(target.Certificate())
	e.transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	cert, err := e.Certificate()
	if err != nil {
		t.Fatal(err)
	}
	clientRoots := x509.NewCertPool()
	clientRoots.AppendCertsFromPEM([]byte(cert.PEM))
	if _, err = e.SetRules([]string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	client := clientFor(t, e, clientRoots)
	client.Transport.(*http.Transport).ForceAttemptHTTP2 = true
	r, err := client.Get(target.URL + "/test.Service/Run")
	if err != nil {
		t.Fatal(err)
	}
	_ = responseText(t, r, err)
	if r.ProtoMajor != 2 || r.Trailer.Get("Grpc-Status") != "0" {
		t.Fatalf("downstream %s trailer %#v", r.Proto, r.Trailer)
	}
	if got := waitForDetail(t, e, 1); got.ResponseTrailers.Get("Grpc-Status") != "0" || got.Protocol != "HTTP/2.0" {
		t.Fatal(got.Row)
	}
}

func TestWebSocketParserHandlesMaskedPartialAndEmptyFrames(t *testing.T) {
	var frames []Frame
	parser := &frameParser{direction: "send", onFrame: func(frame Frame) { frames = append(frames, frame) }}
	data := []byte{0x81, 0x82, 1, 2, 3, 4, byte('O') ^ 1, byte('K') ^ 2, 0x89, 0}
	for _, value := range data {
		_, _ = parser.Write([]byte{value})
	}
	if len(frames) != 2 || string(frames[0].Data) != "OK" || frames[0].Opcode != 1 || frames[1].Opcode != 9 {
		t.Fatalf("frames %#v", frames)
	}
}

func TestProxyNetworkingReverseDNSAndUpstream(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.RequestURI())
	}))
	defer target.Close()
	hop := testEngine(t)

	e := New(t.TempDir())
	if err := e.SetNetwork(NetworkOptions{
		Bind: "127.0.0.1", Reverse: target.URL + "/api", DNS: map[string]string{"local.test": "127.0.0.1"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Start(0, false); err != nil {
		t.Fatal(err)
	}
	defer e.Stop()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://" + e.Snapshot(0).Endpoint + "/probe?x=1")
	if result := responseText(t, response, err); result != "/api/probe?x=1" {
		t.Fatal(result)
	}

	client = clientFor(t, e, nil)
	response, err = client.Get(strings.Replace(target.URL, "127.0.0.1", "local.test", 1) + "/dns")
	if result := responseText(t, response, err); result != "/dns" {
		t.Fatal(result)
	}
	e.Stop()
	if err := e.SetNetwork(NetworkOptions{Upstream: "http://" + hop.Snapshot(0).Endpoint}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Start(0, false); err != nil {
		t.Fatal(err)
	}
	client = clientFor(t, e, nil)
	response, err = client.Get(target.URL + "/chain")
	if result := responseText(t, response, err); result != "/chain" {
		t.Fatal(result)
	}
	if got := waitForDetail(t, hop, 1); got.Path != "/chain" {
		t.Fatal(got.Row)
	}
}
