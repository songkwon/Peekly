package capture

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type RequestInput struct {
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}

func (e *Engine) Compose(ctx context.Context, input RequestInput) (Detail, error) {
	if len(input.Body) > maxBody {
		return Detail{}, errors.New("编辑器支持最多 64 KiB 的请求正文")
	}
	return e.send(ctx, input.Method, input.URL, input.Headers, []byte(input.Body))
}

func (e *Engine) Repeat(ctx context.Context, id uint64) (Detail, error) {
	detail, err := e.Detail(id)
	if err != nil {
		return Detail{}, err
	}
	if detail.Pending || detail.RequestTruncated || detail.Method == http.MethodConnect || detail.Kind == "WebSocket" {
		return Detail{}, errors.New("当前记录不能直接重发")
	}
	return e.send(ctx, detail.Method, detail.URL, detail.RequestHeaders, detail.RequestBody)
}

func (e *Engine) send(ctx context.Context, method, address string, headers http.Header, body []byte) (Detail, error) {
	target, err := url.Parse(strings.TrimSpace(address))
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" || target.User != nil {
		return Detail{}, errors.New("请输入有效的 HTTP 或 HTTPS URL")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" || method == http.MethodConnect {
		return Detail{}, errors.New("请输入有效的 HTTP 方法")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return Detail{}, err
	}
	for key, values := range headers {
		if !validHeaderName(key) {
			return Detail{}, errors.New("无效的请求标头名称")
		}
		for _, value := range values {
			if invalidHeaderValue(value) {
				return Detail{}, errors.New("无效的请求标头值")
			}
			req.Header.Add(key, value)
		}
	}
	removeHopHeaders(req.Header)
	for _, name := range []string{"Host", "Content-Length"} {
		req.Header.Del(name)
	}
	e.mu.Lock()
	run := e.run
	e.mu.Unlock()
	if run != nil {
		req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, proxyAddr(run.address)))
	}
	w := &discardResponse{header: make(http.Header)}
	id := e.forward(nil, w, req, "")
	if id == 0 && w.status < 400 {
		return Detail{}, errors.New("请求已发送，命中的绕过规则使它没有抓包记录")
	}
	if id == 0 {
		return Detail{}, errors.New("请求未发送")
	}
	return e.Detail(id)
}

type proxyAddr string

func (a proxyAddr) Network() string { return "tcp" }
func (a proxyAddr) String() string  { return string(a) }

type discardResponse struct {
	header http.Header
	status int
}

func (w *discardResponse) Header() http.Header         { return w.header }
func (w *discardResponse) WriteHeader(status int)      { w.status = status }
func (w *discardResponse) Write(p []byte) (int, error) { return io.Discard.Write(p) }
func (w *discardResponse) Flush()                      {}
