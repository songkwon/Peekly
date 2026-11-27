package capture

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/dop251/goja"
)

type scriptMessage struct {
	URL     string      `json:"url"`
	Method  string      `json:"method"`
	Headers http.Header `json:"headers"`
	Body    *string     `json:"body"`
	Status  int         `json:"status,omitempty"`
}

func executeScript(program *goja.Program, hook string, input scriptMessage) (scriptMessage, error) {
	vm := goja.New()
	vm.SetMaxCallStackSize(128)
	data, err := json.Marshal(input)
	if err != nil {
		return input, err
	}
	if err := vm.Set("__peekly_data", string(data)); err != nil {
		return input, err
	}
	timeout := time.AfterFunc(40*time.Millisecond, func() { vm.Interrupt("脚本运行超时") })
	defer timeout.Stop()
	if _, err = vm.RunProgram(program); err != nil {
		return input, err
	}
	value, err := vm.RunString(`(function () {
		const message = JSON.parse(__peekly_data);
		const result = typeof ` + hook + ` === "function" ? ` + hook + `(message) : message;
		return JSON.stringify(result === undefined ? message : result);
	})()`)
	if err != nil {
		return input, err
	}
	encoded, ok := value.Export().(string)
	if !ok || len(encoded) > 512*1024 {
		return input, errors.New("脚本结果应为大小有限的消息对象")
	}
	var output scriptMessage
	if err := json.Unmarshal([]byte(encoded), &output); err != nil || output.Headers == nil {
		return input, errors.New("脚本消息的 headers 应为标头名到字符串列表的对象")
	}
	if output.Body != nil && len(*output.Body) > maxBody {
		return input, errors.New("脚本正文不能超过 64 KiB")
	}
	if output.Status != input.Status && (input.Status == 0 || output.Status < 200 || output.Status > 599) {
		return input, errors.New("脚本响应状态码需在 200–599 之间")
	}
	return output, nil
}

func scriptHeaders(original, result http.Header) (http.Header, error) {
	headers := make(http.Header, len(result))
	for name, values := range result {
		key := http.CanonicalHeaderKey(name)
		if !validHeaderName(key) {
			return nil, errors.New("脚本返回了无效标头名称")
		}
		for _, value := range values {
			if invalidHeaderValue(value) {
				return nil, errors.New("脚本返回了无效标头值")
			}
		}
		headers[key] = append(headers[key], values...)
	}
	for _, name := range []string{"Host", "Content-Length", "Content-Encoding", "Transfer-Encoding", "Connection", "Proxy-Authorization", "Proxy-Authenticate", "Upgrade", "Trailer", "Te", "Keep-Alive", "Proxy-Connection"} {
		if !slices.Equal(headers[name], original[name]) {
			return nil, errors.New("脚本不能修改连接或正文编码标头 " + name)
		}
	}
	return headers, nil
}

func scriptBody(body *io.ReadCloser, size int64, headers http.Header) (*string, error) {
	if headers.Get("Content-Encoding") != "" && headers.Get("Content-Encoding") != "identity" {
		return nil, nil
	}
	if *body == nil || *body == http.NoBody {
		empty := ""
		return &empty, nil
	}
	if size < 0 || size > maxBody || contentType(headers) == "text/event-stream" {
		return nil, nil
	}
	original := *body
	raw, err := io.ReadAll(io.LimitReader(original, maxBody+1))
	*body = &readCloser{Reader: io.MultiReader(bytes.NewReader(raw), original), Closer: original}
	if err != nil || len(raw) > maxBody || !utf8.Valid(raw) {
		return nil, err
	}
	value := string(raw)
	return &value, nil
}

func applyScript(program *goja.Program, hook string, detail Detail, headers http.Header, body *io.ReadCloser, length *int64, status *int) (bool, error) {
	input := scriptMessage{URL: detail.URL, Method: detail.Method, Headers: headers.Clone()}
	if input.Headers == nil {
		input.Headers = make(http.Header)
	}
	var err error
	input.Body, err = scriptBody(body, *length, headers)
	if err != nil {
		return false, err
	}
	if status != nil {
		input.Status = *status
	}
	result, err := executeScript(program, hook, input)
	if err != nil {
		return false, err
	}
	next, err := scriptHeaders(input.Headers, result.Headers)
	if err != nil {
		return false, err
	}
	modified := result.Body != nil && (input.Body == nil || *result.Body != *input.Body)
	if modified && (input.Body == nil || status != nil && (detail.Method == http.MethodHead || result.Status == 204 || result.Status == 304)) {
		return false, errors.New("当前正文不可改写")
	}
	if status != nil && result.Status != input.Status && (result.Status == 204 || result.Status == 304 || result.Status < 200) {
		return false, errors.New("脚本不能改为无正文的响应状态")
	}
	clear(headers)
	for name, values := range next {
		headers[name] = values
	}
	if modified {
		if *body != nil {
			_ = (*body).Close()
		}
		*body = io.NopCloser(bytes.NewBufferString(*result.Body))
		*length = int64(len(*result.Body))
		headers.Set("Content-Length", strconv.FormatInt(*length, 10))
	}
	if status != nil {
		*status = result.Status
	}
	return modified, nil
}
