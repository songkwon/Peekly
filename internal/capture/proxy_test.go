package capture

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	e := New(t.TempDir())
	if _, err := e.Start(0, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Stop)
	return e
}

func clientFor(t *testing.T, e *Engine, roots *x509.CertPool) *http.Client {
	t.Helper()
	proxy, err := url.Parse("http://" + e.Snapshot(0).Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: http.ProxyURL(proxy), TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

func responseText(t *testing.T, response *http.Response, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("response %d: %q, %v", response.StatusCode, body, err)
	}
	return string(body)
}

func waitForDetail(t *testing.T, e *Engine, count int) Detail {
	t.Helper()
	for range 150 {
		rows := e.Snapshot(0).Rows
		if len(rows) == count && !rows[0].Pending {
			result, err := e.Detail(rows[0].ID)
			if err == nil {
				return result
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no completed capture among rows %#v", e.Snapshot(0).Rows)
	return Detail{}
}

func TestHTTPProxyCapturesBoundedBodiesAndExportsHAR(t *testing.T) {
	body := strings.Repeat("p", maxBody+512)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if string(data) != body || r.Header.Get("X-Sample") != "peekly" || r.Header.Get("Proxy-Authorization") != "" {
			t.Error("unexpected bytes or headers at upstream")
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()
	e := testEngine(t)
	client := clientFor(t, e, nil)
	request, _ := http.NewRequest("POST", server.URL+"/ping?project=peekly", strings.NewReader(body))
	request.Header.Set("X-Sample", "peekly")
	request.Header.Set("Proxy-Authorization", "Basic local-only")
	response, err := client.Do(request)
	if result := responseText(t, response, err); result != body {
		t.Fatal("proxy modified response")
	}
	detail := waitForDetail(t, e, 1)
	if detail.Method != "POST" || detail.Path != "/ping?project=peekly" || detail.Status != 200 ||
		detail.RequestBytes != int64(len(body)) || detail.Size != int64(len(body)) ||
		!detail.RequestTruncated || !detail.ResponseTruncated || len(detail.RequestBody) != maxBody || len(detail.ResponseBody) != maxBody {
		t.Fatalf("capture mismatch: %#v", detail.Row)
	}
	har, err := e.HAR([]uint64{detail.ID})
	if err != nil || !json.Valid(har) || !strings.Contains(string(har), "_peeklyTruncated") {
		t.Fatal("invalid HAR")
	}
	state := e.Snapshot(0)
	if e.Snapshot(state.Revision).Changed {
		t.Fatal("unchanged snapshot returned rows")
	}
	e.Clear()
	if e.Snapshot(state.Revision).Rows == nil || len(e.Snapshot(0).Rows) != 0 {
		t.Fatal("clear did not propagate")
	}
	ep := e.Snapshot(0).Endpoint
	e.Stop()
	if e.Snapshot(0).Running {
		t.Fatal("listener still running")
	}
	conn, err := net.DialTimeout("tcp", ep, time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("stopped listener still accepts connections")
	}
}

func TestHTTPSPassThroughAndDomainScopedDecryption(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	e := testEngine(t)
	roots := x509.NewCertPool()
	roots.AddCert(upstream.Certificate())
	plain := clientFor(t, e, roots)
	response, err := plain.Get(upstream.URL + "/untouched")
	if got := responseText(t, response, err); got != `{"ok":true}` {
		t.Fatal(got)
	}
	if got := waitForDetail(t, e, 1); got.Kind != "Tunnel" || len(got.ResponseBody) != 0 {
		t.Fatal("unmatched TLS traffic was decrypted")
	}
	persisted, err := e.Certificate()
	if err != nil || !roots.AppendCertsFromPEM([]byte(persisted.PEM)) {
		t.Fatal("invalid root certificate", err)
	}
	reopened, err := New(e.dir).Certificate()
	if err != nil || reopened.Fingerprint != persisted.Fingerprint {
		t.Fatal("root certificate was not persisted")
	}
	upstreamRoots := x509.NewCertPool()
	upstreamRoots.AddCert(upstream.Certificate())
	e.transport.TLSClientConfig = &tls.Config{RootCAs: upstreamRoots, MinVersion: tls.VersionTLS12}
	if _, err := e.SetRules([]string{"*.example.com", "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if e.authorityFor("example.com") != nil || e.authorityFor("a.example.com") == nil {
		t.Fatal("wildcard applies outside subdomains")
	}
	if _, err := e.SetRules([]string{"*"}); err == nil {
		t.Fatal("global decryption accepted")
	}
	via := clientFor(t, e, roots)
	response, err = via.Get(upstream.URL + "/decoded?q=1")
	if got := responseText(t, response, err); got != `{"ok":true}` {
		t.Fatal(got)
	}
	detail := waitForDetail(t, e, 2)
	if detail.Kind != "HTTPS" || detail.Path != "/decoded?q=1" || string(detail.ResponseBody) != `{"ok":true}` {
		t.Fatalf("TLS capture mismatch: %#v", detail.Row)
	}
}

func TestStreamFlushesBeforeCompletionAndUpgradeRelays(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-release
		_, _ = io.WriteString(w, "data: end\n\n")
	}))
	defer server.Close()
	defer close(release)
	e := testEngine(t)
	client := clientFor(t, e, nil)
	res, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	if err != nil || line != "data: first\n" || !e.Snapshot(0).Rows[0].Pending {
		t.Fatalf("first stream chunk: %q, %v", line, err)
	}

	websocket := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, buffered, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		_ = buffered.Flush()
		buf := make([]byte, 5)
		if _, err := io.ReadFull(buffered, buf); err == nil {
			_, _ = conn.Write(buf)
		}
	}))
	defer websocket.Close()
	conn, err := net.DialTimeout("tcp", e.Snapshot(0).Endpoint, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	target, _ := url.Parse(websocket.URL)
	_, err = io.WriteString(conn, "GET "+websocket.URL+"/socket HTTP/1.1\r\nHost: "+target.Host+"\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
	b := bufio.NewReader(conn)
	ws, err := http.ReadResponse(b, nil)
	if err != nil || ws.StatusCode != 101 {
		t.Fatalf("upgrade: %#v, %v", ws, err)
	}
	_, _ = io.WriteString(conn, "hello")
	var echo [5]byte
	if _, err := io.ReadFull(b, echo[:]); err != nil || string(echo[:]) != "hello" {
		t.Fatalf("echo %q: %v", echo, err)
	}
	if got := waitForDetail(t, e, 2); got.Kind != "WebSocket" || got.Status != 101 {
		t.Fatalf("upgrade capture mismatch: %#v", got.Row)
	}
}
