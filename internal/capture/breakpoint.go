package capture

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type BreakpointAction struct {
	Action      string      `json:"action"`
	ReplaceBody bool        `json:"replaceBody"`
	Body        string      `json:"body"`
	Status      int         `json:"status"`
	Headers     http.Header `json:"headers"`
}

func (e *Engine) ResolveBreakpoint(id uint64, action BreakpointAction) error {
	if action.Action != "continue" && action.Action != "drop" {
		return errors.New("无效的断点操作")
	}
	if len(action.Body) > maxBody || action.Status != 0 && (action.Status < 200 || action.Status > 599) {
		return errors.New("断点正文或响应状态码无效")
	}
	for key, values := range action.Headers {
		if !validHeaderName(key) {
			return errors.New("无效的标头名称")
		}
		for _, value := range values {
			if invalidHeaderValue(value) {
				return errors.New("无效的标头值")
			}
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if ch, ok := e.breakpoints[id]; ok {
		select {
		case ch <- action:
			return nil
		default:
		}
	}
	return errors.New("断点已继续或失效")
}

func (e *Engine) interceptBreakpoint(ctx context.Context, id uint64, detail *Detail, stage string, response *http.Response, request *http.Request) error {
	var headers http.Header
	var body *io.ReadCloser
	var contentLength *int64
	if request != nil {
		headers, body, contentLength = request.Header, &request.Body, &request.ContentLength
	} else {
		headers, body, contentLength = response.Header, &response.Body, &response.ContentLength
	}
	if *body != nil && *contentLength > 0 && *contentLength <= maxBody {
		original := *body
		data, err := io.ReadAll(io.LimitReader(original, maxBody+1))
		if err != nil {
			return err
		}
		*body = &closeOnceReader{Reader: io.MultiReader(bytes.NewReader(data), original), closer: original}
		if request != nil {
			detail.RequestBody = data
		} else {
			detail.ResponseBody = data
		}
	}

	channel := make(chan BreakpointAction, 1)
	e.mu.Lock()
	e.breakpoints[id] = channel
	e.mu.Unlock()
	detail.Breakpoint = stage
	e.store.update(id, *detail)

	defer func() {
		e.mu.Lock()
		delete(e.breakpoints, id)
		e.mu.Unlock()
		detail.Breakpoint = ""
	}()
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	var action BreakpointAction
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	case action = <-channel:
	}
	if action.Action == "drop" {
		return errors.New("已在断点处中止此请求")
	}
	for key, values := range action.Headers {
		if breakpointHeader(key) {
			headers.Del(key)
			for _, value := range values {
				headers.Add(key, value)
			}
		}
	}
	if response != nil && action.Status != 0 {
		response.StatusCode = action.Status
	}
	if action.ReplaceBody {
		if *body != nil {
			_ = (*body).Close()
		}
		*body = io.NopCloser(bytes.NewBufferString(action.Body))
		*contentLength = int64(len(action.Body))
		headers.Del("Content-Encoding")
		headers.Del("ETag")
		if request == nil {
			headers.Set("Content-Length", strconv.Itoa(len(action.Body)))
			response.Trailer = nil
		} else {
			request.TransferEncoding, request.Trailer = nil, nil
		}
	}
	if response != nil && (response.StatusCode == 204 || response.StatusCode == 304) {
		_ = response.Body.Close()
		response.Body = http.NoBody
		response.ContentLength = 0
		response.Header.Del("Content-Length")
	}
	return nil
}

func breakpointHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Host", "Content-Length", "Transfer-Encoding", "Connection", "Upgrade", "Proxy-Authorization", "Proxy-Authenticate", "Trailer", "Content-Encoding":
		return false
	}
	return true
}

type closeOnceReader struct {
	io.Reader
	closer io.Closer
	once   sync.Once
	err    error
}

func (r *closeOnceReader) Close() error {
	r.once.Do(func() { r.err = r.closer.Close() })
	return r.err
}
