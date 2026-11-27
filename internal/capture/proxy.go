package capture

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

type Engine struct {
	network      networkConfig
	mu           sync.Mutex
	run          *listener
	dir          string
	authority    *authority
	rules        []string
	store        *store
	transport    *http.Transport
	trafficRules []compiledRule
	breakpoints  map[uint64]chan BreakpointAction
}

type listener struct {
	address string
	server  *http.Server
	cancel  context.CancelFunc
	mu      sync.Mutex
	conns   map[net.Conn]struct{}
	closed  bool
}

type State struct {
	Running  bool     `json:"running"`
	Endpoint string   `json:"endpoint"`
	Rules    []string `json:"rules"`
	Revision uint64   `json:"revision"`
	Changed  bool     `json:"changed"`
	Rows     []Row    `json:"rows"`
}

func New(dir string) *Engine {
	config := networkConfig{NetworkOptions: NetworkOptions{Bind: "127.0.0.1"}}
	engine := &Engine{dir: dir, network: config, store: newStore(), transport: newTransport(config), breakpoints: make(map[uint64]chan BreakpointAction)}
	if data, err := os.ReadFile(filepath.Join(dir, "traffic-rules.json")); err == nil {
		var rules []Rule
		if json.Unmarshal(data, &rules) == nil {
			engine.trafficRules, _ = compileRules(rules)
		}
	}
	return engine
}

func (e *Engine) Start(port int, allowAlternatePort bool) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.run != nil {
		return e.run.address, nil
	}
	if port < 0 || port > 65535 {
		return "", errors.New("代理端口无效")
	}
	ln, err := net.Listen("tcp4", net.JoinHostPort(e.network.Bind, strconv.Itoa(port)))
	if errors.Is(err, addressInUse) {
		if !allowAlternatePort {
			return "", fmt.Errorf("端口 %d 已被占用，请在连接指引中更换监听端口: %w", port, err)
		}
		// Bind to port zero directly so the free port is reserved atomically.
		ln, err = net.Listen("tcp4", net.JoinHostPort(e.network.Bind, "0"))
	}
	if err != nil {
		return "", fmt.Errorf("无法启动代理: %w", err)
	}
	config := e.network
	if config.upstream != nil {
		upstreamPort := config.upstream.Port()
		upstreamIP := net.ParseIP(config.upstream.Hostname())
		if upstreamPort == strconv.Itoa(ln.Addr().(*net.TCPAddr).Port) && (strings.EqualFold(config.upstream.Hostname(), "localhost") || upstreamIP != nil && (upstreamIP.IsLoopback() || upstreamIP.String() == config.Bind)) {
			_ = ln.Close()
			return "", errors.New("上游代理不能指向 Peekly 本身")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &listener{address: ln.Addr().String(), conns: make(map[net.Conn]struct{}), cancel: cancel}
	run.server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !config.permits(r.RemoteAddr) {
				http.Error(w, "此客户端未获准连接 Peekly", http.StatusForbidden)
				return
			}
			if r.Method == http.MethodConnect {
				e.connect(run, w, r)
				return
			}
			e.forward(run, w, r, "")
		}),
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ConnState:         run.connState,
		ReadHeaderTimeout: 12 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}
	e.run = run
	go func() {
		_ = run.server.Serve(ln)
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.run == run {
			run.close()
			e.run = nil
		}
	}()
	return run.address, nil
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.run != nil {
		e.run.close()
		e.run = nil
	}
	e.transport.CloseIdleConnections()
}

func (l *listener) close() {
	l.cancel()
	l.mu.Lock()
	l.closed = true
	for conn := range l.conns {
		_ = conn.Close()
	}
	clear(l.conns)
	l.mu.Unlock()
	_ = l.server.Close()
}

func (l *listener) connState(conn net.Conn, state http.ConnState) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch state {
	case http.StateNew:
		if l.closed {
			_ = conn.Close()
		} else {
			l.conns[conn] = struct{}{}
		}
	case http.StateClosed:
		delete(l.conns, conn)
	}
}

func (l *listener) forget(conn net.Conn) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.conns, conn)
}

func (e *Engine) Snapshot(since uint64) State {
	e.mu.Lock()
	state := State{Rules: append([]string{}, e.rules...)}
	if e.run != nil {
		state.Running = true
		state.Endpoint = e.run.address
	}
	e.mu.Unlock()
	state.Revision, state.Rows, state.Changed = e.store.snapshot(since)
	return state
}

func (e *Engine) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, channel := range e.breakpoints {
		select {
		case channel <- BreakpointAction{Action: "continue"}:
		default:
		}
	}
	e.store.clear()
}

func (e *Engine) Detail(id uint64) (Detail, error) {
	detail, ok := e.store.get(id)
	if !ok {
		return Detail{}, errors.New("该请求已从当前记录移除")
	}
	return detail, nil
}

var domainLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func normalizeHost(value string) string {
	return strings.ToLower(strings.TrimSuffix(value, "."))
}

func validHost(value string) bool {
	if net.ParseIP(value) != nil {
		return true
	}
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if !domainLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func (e *Engine) SetRules(input []string) ([]string, error) {
	if len(input) > 128 {
		return nil, errors.New("最多支持 128 条 HTTPS 解密规则")
	}
	rules := make([]string, 0, len(input))
	seen := make(map[string]bool, len(input))
	for _, value := range input {
		value = normalizeHost(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		host := strings.TrimPrefix(value, "*.")
		if !validHost(host) || (host != value && (net.ParseIP(host) != nil || !strings.Contains(host, "."))) {
			return nil, fmt.Errorf("无效域名规则 %q，请填写域名、IP 或 *.example.com", value)
		}
		if !seen[value] {
			rules = append(rules, value)
			seen[value] = true
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(rules) != 0 {
		if _, err := e.getAuthority(); err != nil {
			return nil, err
		}
	}
	e.rules = rules
	return append([]string{}, rules...), nil
}

func (e *Engine) Certificate() (Certificate, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	a, err := e.getAuthority()
	if err != nil {
		return Certificate{}, err
	}
	return a.info(), nil
}

func (e *Engine) getAuthority() (*authority, error) {
	if e.authority == nil {
		var err error
		e.authority, err = openAuthority(e.dir)
		if err != nil {
			return nil, err
		}
	}
	return e.authority, nil
}

func (e *Engine) authorityFor(host string) *authority {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, rule := range e.rules {
		if rule == host || (strings.HasPrefix(rule, "*.") && strings.HasSuffix(host, rule[1:])) {
			return e.authority
		}
	}
	return nil
}

func targetAddress(authority string) (string, string, error) {
	target, err := url.Parse("https://" + authority)
	if err != nil || target.User != nil || target.Host != authority || target.Path != "" || target.ForceQuery || target.RawQuery != "" || target.Fragment != "" {
		return "", "", errors.New("目标地址无效")
	}
	host := normalizeHost(target.Hostname())
	if !validHost(host) {
		return "", "", errors.New("目标主机无效")
	}
	port := target.Port()
	if port == "" {
		port = "443"
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", "", errors.New("目标端口无效")
	}
	return net.JoinHostPort(host, strconv.Itoa(number)), host, nil
}

func (e *Engine) connect(run *listener, w http.ResponseWriter, r *http.Request) {
	target, host, err := targetAddress(r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if e.isProxyAddress(url.URL{Host: target}, r.Context()) {
		http.Error(w, "代理不能转发请求到自身", http.StatusBadRequest)
		return
	}
	var bypass bool
	for _, rule := range e.matchingRules("https://" + target) {
		switch rule.Kind {
		case "block":
			row := Row{Started: time.Now(), Method: "CONNECT", URL: "https://" + target, Host: host, Path: "HTTPS 隧道", Kind: "Tunnel", Status: 403, Rule: "block"}
			e.store.insert(row)
			http.Error(w, "请求被 Peekly 规则阻止", http.StatusForbidden)
			return
		case "bypass":
			bypass = true
		}
	}
	if a := e.authorityFor(host); a != nil && !bypass {
		e.intercept(run, w, r, target, host, a)
		return
	}
	row := Row{Started: time.Now(), Method: "CONNECT", URL: "https://" + target, Host: host, Path: "HTTPS 隧道", Kind: "Tunnel", Pending: true}
	var id uint64
	if !bypass {
		id = e.store.insert(row)
	}
	detail := Detail{Row: row, Protocol: r.Proto}
	upstream, err := e.networkConfig().tunnel(r.Context(), target)
	if err != nil {
		e.fail(w, id, detail, err)
		return
	}
	defer upstream.Close()
	conn, buffered, err := hijack(w)
	if err != nil {
		e.fail(w, id, detail, err)
		return
	}
	defer run.forget(conn)
	defer conn.Close()
	if err := establish(buffered); err != nil {
		detail.Error = err.Error()
	} else {
		detail.Status = http.StatusOK
	}
	detail.Duration = time.Since(row.Started).Milliseconds()
	detail.Pending = false
	e.store.update(id, detail)
	if detail.Status == http.StatusOK {
		relay(conn, buffered.Reader, upstream, upstream)
	}
}

func (e *Engine) intercept(run *listener, w http.ResponseWriter, r *http.Request, target, host string, a *authority) {
	cert, err := a.leaf(host)
	if err != nil {
		http.Error(w, "无法生成站点证书", http.StatusBadGateway)
		return
	}
	conn, buffered, err := hijack(w)
	if err != nil {
		http.Error(w, "无法开启 HTTPS 隧道", http.StatusBadGateway)
		return
	}
	defer run.forget(conn)
	defer conn.Close()
	if err := establish(buffered); err != nil {
		return
	}
	client := tls.Server(&bufferedConn{Conn: conn, reader: buffered.Reader}, &tls.Config{
		Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2", "http/1.1"}, MinVersion: tls.VersionTLS12,
	})
	const handshakeTimeout = 15 * time.Second
	handshake, cancel := context.WithTimeout(r.Context(), handshakeTimeout)
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	err = client.HandshakeContext(handshake)
	cancel()
	_ = conn.SetDeadline(time.Time{})
	if err != nil {
		row := Row{Started: time.Now(), Method: "CONNECT", URL: "https://" + target, Host: host, Path: "TLS 握手", Kind: "HTTPS", Error: err.Error()}
		e.store.insert(row)
		return
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { e.forward(run, w, req, target) })
	if client.ConnectionState().NegotiatedProtocol == "h2" {
		server := &http2.Server{IdleTimeout: 90 * time.Second, MaxConcurrentStreams: 128}
		server.ServeConn(client, &http2.ServeConnOpts{Context: r.Context(), Handler: handler, BaseConfig: &http.Server{MaxHeaderBytes: 64 * 1024}})
		return
	}
	single := newSingleListener(client)
	server := &http.Server{
		Handler:           handler,
		BaseContext:       func(net.Listener) context.Context { return r.Context() },
		ReadHeaderTimeout: 12 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}
	_ = server.Serve(single)
}

func (e *Engine) forward(run *listener, w http.ResponseWriter, r *http.Request, tlsTarget string) uint64 {
	dest := *r.URL
	reverse := false
	if tlsTarget == "" && !dest.IsAbs() && r.Method != http.MethodConnect {
		if target := e.networkConfig().reverse; target != nil {
			dest.Scheme, dest.Host = target.Scheme, target.Host
			dest.Path = strings.TrimSuffix(target.Path, "/") + "/" + strings.TrimPrefix(dest.Path, "/")
			dest.RawPath = ""
			reverse = true
		}
	}
	if tlsTarget != "" {
		host, _, err := targetAddress(r.Host)
		if err != nil || host != tlsTarget || r.Method == http.MethodConnect || dest.IsAbs() {
			http.Error(w, "请求主机与 HTTPS 隧道不一致", http.StatusMisdirectedRequest)
			return 0
		}
		dest.Scheme, dest.Host = "https", tlsTarget
	}
	if (dest.Scheme != "http" && dest.Scheme != "https") || dest.Host == "" || dest.User != nil || dest.Opaque != "" {
		http.Error(w, "请以 HTTP 代理模式连接 Peekly", http.StatusBadRequest)
		return 0
	}
	if e.isProxyAddress(dest, r.Context()) {
		http.Error(w, "代理不能转发请求到自身", http.StatusBadRequest)
		return 0
	}
	kind := strings.ToUpper(dest.Scheme)
	upgrade := websocketUpgrade(r.Header)
	if upgrade {
		kind = "WebSocket"
	}
	row := Row{Started: time.Now(), Method: r.Method, URL: dest.String(), Host: normalizeHost(dest.Hostname()), Path: dest.RequestURI(), Kind: kind, Pending: true}
	matches, denied := e.requestRules(row.URL)
	bypass := false
	for _, rule := range matches {
		if rule.Kind == "bypass" {
			bypass = true
			break
		}
	}
	var id uint64
	if !bypass || denied {
		id = e.store.insert(row)
	}
	detail := Detail{Row: row, RequestHeaders: r.Header.Clone(), Protocol: r.Proto}
	if r.Host != "" {
		detail.RequestHeaders.Set("Host", r.Host)
	}
	e.store.update(id, detail)
	out := r.Clone(r.Context())
	out.URL, out.RequestURI = &dest, ""
	if reverse {
		out.Host = dest.Host
	}
	removeHopHeaders(out.Header)
	if upgrade {
		out.Header.Set("Connection", "Upgrade")
		out.Header.Set("Upgrade", "websocket")
	}

	var resp *http.Response
	var err error
	var rate int
	var noCache, reqBreak, resBreak bool
	if denied {
		resp = stringResponse(http.StatusForbidden, "text/plain; charset=utf-8", "请求未匹配 Peekly 允许列表")
		detail.Rule = "allow"
	}
	for _, rule := range matches {
		if bypass || denied {
			break
		}
		updateRuleHeader(rule, out.Header, "request")
		switch rule.Kind {
		case "allow":
			continue
		case "block":
			resp = stringResponse(http.StatusForbidden, "text/plain; charset=utf-8", "请求被 Peekly 规则阻止")
		case "mock":
			mimeType := rule.Name
			if mimeType == "" {
				mimeType = "application/json; charset=utf-8"
			}
			resp = stringResponse(rule.Number, mimeType, rule.Value)
		case "file", "directory":
			resp, err = mappedResponse(rule, out)
			if resp == nil && err == nil {
				continue
			}
		case "redirect":
			target, _ := url.Parse(rule.Value)
			if e.isProxyAddress(*target, r.Context()) {
				err = errors.New("远程映射不能指向 Peekly 自身")
				break
			}
			if !strings.EqualFold(out.URL.Host, target.Host) || out.URL.Scheme != target.Scheme {
				for _, name := range []string{"Authorization", "Cookie", "Origin", "Referer"} {
					out.Header.Del(name)
				}
			}
			out.URL = target
			out.Host = target.Host
		case "delay":
			err = pause(out.Context(), time.Duration(rule.Number)*time.Millisecond)
		case "throttle":
			rate = rule.Number
		case "no-cache":
			noCache = true
			out.Header.Del("If-Modified-Since")
			out.Header.Del("If-None-Match")
			out.Header.Set("Cache-Control", "no-cache")
		case "break-request":
			reqBreak = true
		case "break-response":
			resBreak = true
		}
		if detail.Rule == "" {
			detail.Rule = rule.Kind
		}
		if err != nil {
			e.fail(w, id, detail, err)
			return id
		}
		if resp != nil {
			break
		}
	}

	if reqBreak && resp == nil {
		if err := e.interceptBreakpoint(out.Context(), id, &detail, "request", nil, out); err != nil {
			e.fail(w, id, detail, err)
			return id
		}
	}
	var request, response preview
	var trace *timingTrace
	if out.Body != nil && out.Body != http.NoBody && id != 0 {
		out.Body = &readCloser{Reader: io.TeeReader(out.Body, &request), Closer: out.Body}
	}
	if resp == nil {
		if id != 0 {
			var ctx context.Context
			ctx, trace = newTrace(out.Context())
			out = out.WithContext(ctx)
		}
		resp, err = e.roundTrip(out)
		if trace != nil {
			detail.Timing = trace.result()
		}
	} else if out.Body != nil && out.ContentLength > 0 && out.ContentLength <= maxBody {
		_, _ = io.Copy(io.Discard, out.Body)
	}
	detail.RequestBody, detail.RequestBytes = request.result()
	detail.RequestTruncated = detail.RequestBytes > int64(len(detail.RequestBody))
	if err != nil {
		e.fail(w, id, detail, err)
		return id
	}
	defer resp.Body.Close()
	for _, rule := range matches {
		if !bypass && !denied {
			updateRuleHeader(rule, resp.Header, "response")
		}
	}
	if noCache {
		resp.Header.Set("Cache-Control", "no-store")
	}
	detail.Status, detail.ContentType = resp.StatusCode, contentType(resp.Header)
	detail.ResponseProtocol = resp.Proto
	detail.ResponseHeaders = resp.Header.Clone()
	detail.Duration = time.Since(row.Started).Milliseconds()
	if resp.StatusCode == http.StatusSwitchingProtocols {
		if run == nil {
			e.fail(w, id, detail, errors.New("编辑器不支持升级为 WebSocket"))
			return id
		}
		e.websocket(run, w, r, id, detail, resp)
		return id
	}
	if resBreak {
		if err := e.interceptBreakpoint(r.Context(), id, &detail, "response", resp, nil); err != nil {
			e.fail(w, id, detail, err)
			return id
		}
		detail.Status, detail.ContentType = resp.StatusCode, contentType(resp.Header)
		detail.ResponseHeaders = resp.Header.Clone()
	}
	if detail.Rule != "" {
		detail.RequestHeaders = out.Header.Clone()
		if out.Host != "" {
			detail.RequestHeaders.Set("Host", out.Host)
		}
	}
	headers := resp.Header.Clone()
	removeHopHeaders(headers)
	for key, values := range headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	for key := range resp.Trailer {
		w.Header().Add("Trailer", key)
	}
	w.WriteHeader(resp.StatusCode)
	e.store.update(id, detail)
	streaming := detail.ContentType == "text/event-stream" || resp.ContentLength < 0 || rate > 0
	body := io.Writer(w)
	if streaming {
		body = flushWriter{w}
	}
	if rate > 0 {
		body = &throttledWriter{context: r.Context(), out: body, delay: time.Second / (time.Duration(rate) * 1024)}
	}
	if id != 0 {
		body = io.MultiWriter(&response, body)
		if detail.ContentType == "text/event-stream" || strings.HasPrefix(detail.ContentType, "application/grpc") {
			body = &snapshotWriter{out: body, update: func() {
				pending := detail
				pending.ResponseBody, pending.Size = response.result()
				pending.ResponseTruncated = pending.Size > int64(len(pending.ResponseBody))
				pending.Duration = time.Since(row.Started).Milliseconds()
				e.store.update(id, pending)
			}}
		}
	}
	if r.Method != http.MethodHead && resp.StatusCode != 204 && resp.StatusCode != 304 {
		if _, err := io.Copy(body, resp.Body); err != nil {
			detail.Error = err.Error()
		}
	}
	detail.ResponseBody, detail.Size = response.result()
	if trace != nil {
		detail.Timing = trace.result()
	}
	detail.ResponseTruncated = detail.Size > int64(len(detail.ResponseBody))
	detail.Pending = false
	detail.Duration = time.Since(row.Started).Milliseconds()
	for key, values := range resp.Trailer {
		w.Header()[key] = values
	}
	detail.ResponseTrailers = resp.Trailer.Clone()
	e.store.update(id, detail)
	return id
}

func (e *Engine) isProxyAddress(dest url.URL, ctx context.Context) bool {
	local, ok := ctx.Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return false
	}
	_, port, err := net.SplitHostPort(local.String())
	if err != nil || dest.Port() != port {
		return false
	}
	host := normalizeHost(dest.Hostname())
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && (ip.IsLoopback() || ip.IsUnspecified() || ip.String() == e.networkConfig().Bind))
}

func (e *Engine) fail(w http.ResponseWriter, id uint64, detail Detail, err error) {
	detail.Error, detail.Status = err.Error(), http.StatusBadGateway
	detail.Duration, detail.Pending = time.Since(detail.Started).Milliseconds(), false
	e.store.update(id, detail)
	http.Error(w, "Peekly 无法连接目标服务", http.StatusBadGateway)
}

func removeHopHeaders(header http.Header) {
	for _, line := range header.Values("Connection") {
		for _, name := range strings.Split(line, ",") {
			header.Del(strings.TrimSpace(name))
		}
	}
	for _, name := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		header.Del(name)
	}
}

func websocketUpgrade(head http.Header) bool {
	for _, val := range head.Values("Connection") {
		for _, token := range strings.Split(val, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return strings.EqualFold(head.Get("Upgrade"), "websocket")
			}
		}
	}
	return false
}

func (e *Engine) websocket(run *listener, w http.ResponseWriter, r *http.Request, id uint64, detail Detail, resp *http.Response) {
	upstream, ok := resp.Body.(io.ReadWriteCloser)
	if !ok || !websocketUpgrade(resp.Header) {
		e.fail(w, id, detail, errors.New("无效的 WebSocket 握手"))
		return
	}
	conn, buffered, err := hijack(w)
	if err != nil {
		e.fail(w, id, detail, err)
		return
	}
	defer run.forget(conn)
	defer conn.Close()
	stop := context.AfterFunc(r.Context(), func() { _ = conn.Close(); _ = upstream.Close() })
	defer stop()
	_, err = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	headers := resp.Header.Clone()
	removeHopHeaders(headers)
	headers.Set("Connection", "Upgrade")
	headers.Set("Upgrade", "websocket")
	if err == nil {
		err = headers.Write(buffered)
	}
	if err == nil {
		_, err = buffered.WriteString("\r\n")
	}
	if err == nil {
		err = buffered.Flush()
	}
	detail.Pending = false
	if err != nil {
		detail.Error = err.Error()
	}
	e.store.update(id, detail)
	if err == nil {
		outbound := &frameParser{direction: "send", onFrame: func(frame Frame) { e.store.addFrame(id, frame) }}
		inbound := &frameParser{direction: "receive", onFrame: func(frame Frame) { e.store.addFrame(id, frame) }}
		relay(conn, io.TeeReader(buffered.Reader, outbound), upstream, io.TeeReader(upstream, inbound))
	}
}

func hijack(w http.ResponseWriter) (net.Conn, *bufio.ReadWriter, error) {
	conn, buffered, err := http.NewResponseController(w).Hijack()
	if err == nil {
		_ = conn.SetDeadline(time.Time{})
	}
	return conn, buffered, err
}

func establish(buffered *bufio.ReadWriter) error {
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return err
	}
	return buffered.Flush()
}

func relay(client net.Conn, clientReader io.Reader, upstream io.ReadWriteCloser, upstreamReader io.Reader) {
	done := make(chan struct{})
	copyTo := func(dst io.WriteCloser, src io.Reader) {
		_, err := io.Copy(dst, src)
		if half, ok := dst.(interface{ CloseWrite() error }); ok && err == nil {
			_ = half.CloseWrite()
		} else {
			_ = client.Close()
			_ = upstream.Close()
		}
	}
	go func() {
		defer close(done)
		copyTo(upstream, clientReader)
	}()
	copyTo(client, upstreamReader)
	<-done
}

type flushWriter struct{ http.ResponseWriter }

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.ResponseWriter.Write(p)
	if err == nil {
		err = http.NewResponseController(f.ResponseWriter).Flush()
	}
	return n, err
}

type readCloser struct {
	io.Reader
	io.Closer
}

type bufferedConn struct {
	net.Conn
	reader io.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

func (c *bufferedConn) CloseWrite() error {
	if half, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return half.CloseWrite()
	}
	return c.Conn.Close()
}

type singleListener struct {
	conn *closingConn
	once sync.Once
	done chan struct{}
}

type closingConn struct {
	net.Conn
	l *singleListener
}

func (c *closingConn) Close() error {
	err := c.Conn.Close()
	c.l.once.Do(func() { close(c.l.done) })
	return err
}

func newSingleListener(conn net.Conn) *singleListener {
	l := &singleListener{done: make(chan struct{})}
	l.conn = &closingConn{Conn: conn, l: l}
	return l
}

func (l *singleListener) Accept() (net.Conn, error) {
	if l.conn == nil {
		<-l.done
		return nil, net.ErrClosed
	}
	conn := l.conn
	l.conn = nil
	return conn, nil
}

func (l *singleListener) Addr() net.Addr { return &net.TCPAddr{} }
func (l *singleListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

type snapshotWriter struct {
	out    io.Writer
	update func()
	next   time.Time
}

func (w *snapshotWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	if time.Now().After(w.next) {
		w.update()
		w.next = time.Now().Add(300 * time.Millisecond)
	}
	return n, err
}
