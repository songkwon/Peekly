package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
)

type Rule struct {
	Enabled bool   `json:"enabled"`
	Kind    string `json:"kind"`
	Match   string `json:"match"`
	Pattern string `json:"pattern"`
	Name    string `json:"name"`
	Value   string `json:"value"`
	Number  int    `json:"number"`
}

type compiledRule struct {
	Rule
	regex   *regexp.Regexp
	program *goja.Program
}

var headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func validHeaderName(name string) bool { return headerName.MatchString(name) }

func invalidHeaderValue(value string) bool {
	return strings.ContainsAny(value, "\r\n\x00")
}

func compileRules(input []Rule) ([]compiledRule, error) {
	if len(input) > 100 {
		return nil, errors.New("最多保存 100 条流量规则")
	}
	rules := make([]compiledRule, 0, len(input))
	for i, source := range input {
		rule := compiledRule{Rule: source}
		rule.Pattern = strings.TrimSpace(rule.Pattern)
		if len(rule.Pattern) == 0 || len(rule.Pattern) > 2048 || len(rule.Value) > maxBody {
			return nil, fmt.Errorf("规则 %d 的匹配表达式或值无效", i+1)
		}
		var err error
		switch rule.Match {
		case "contains":
		case "glob":
			pattern := strings.ReplaceAll(regexp.QuoteMeta(rule.Pattern), "\\*", ".*")
			rule.regex, err = regexp.Compile("^" + pattern + "$")
		case "regex":
			rule.regex, err = regexp.Compile(rule.Pattern)
		default:
			err = errors.New("无效的匹配方式")
		}
		if err != nil {
			return nil, fmt.Errorf("规则 %d: %w", i+1, err)
		}
		switch rule.Kind {
		case "block", "allow", "bypass", "no-cache", "break-request", "break-response":
		case "script":
			rule.program, err = goja.Compile("peekly.js", rule.Value, true)
			if err != nil {
				return nil, fmt.Errorf("规则 %d 的脚本有语法错误: %w", i+1, err)
			}
		case "mock":
			if rule.Number < 200 || rule.Number > 599 {
				return nil, fmt.Errorf("规则 %d 的模拟状态码需在 200–599 之间", i+1)
			}
		case "file", "directory":
			if !filepath.IsAbs(rule.Value) || rule.Kind == "directory" && !strings.HasPrefix(rule.Name, "/") {
				return nil, fmt.Errorf("规则 %d 需要绝对路径，目录映射还需要 URL 路径前缀", i+1)
			}
		case "redirect":
			target, err := url.Parse(rule.Value)
			if err != nil || target.Hostname() == "" || target.User != nil || (target.Scheme != "http" && target.Scheme != "https") {
				return nil, fmt.Errorf("规则 %d 的目标 URL 无效", i+1)
			}
		case "delay", "throttle":
			if rule.Number < 1 || rule.Number > 30000 {
				return nil, fmt.Errorf("规则 %d 的限速或延迟需在 1–30000 之间", i+1)
			}
		case "request-set", "request-add", "request-remove", "response-set", "response-add", "response-remove":
			// Framing, routing, and proxy credentials are managed by the HTTP transport.
			name := http.CanonicalHeaderKey(rule.Name)
			switch name {
			case "Host", "Content-Length", "Transfer-Encoding", "Connection", "Upgrade", "Proxy-Authorization", "Proxy-Authenticate", "Trailer":
				return nil, fmt.Errorf("规则 %d 无法修改 %s", i+1, name)
			}
			if !validHeaderName(name) || invalidHeaderValue(rule.Value) {
				return nil, fmt.Errorf("规则 %d 的标头格式无效", i+1)
			}
			rule.Name = name
		default:
			return nil, fmt.Errorf("规则 %d 的类型无效", i+1)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (e *Engine) TrafficRules() []Rule {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Rule, 0, len(e.trafficRules))
	for _, rule := range e.trafficRules {
		out = append(out, rule.Rule)
	}
	return out
}

func (e *Engine) SetTrafficRules(input []Rule) error {
	rules, err := compileRules(input)
	if err != nil {
		return err
	}
	list := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		list = append(list, rule.Rule)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err = writeConfig(e.dir, "traffic-rules.json", data); err != nil {
		return err
	}
	e.trafficRules = rules
	return nil
}

func writeConfig(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".peekly-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	_, err = temp.Write(data)
	if err = errors.Join(err, temp.Close()); err != nil {
		return err
	}
	return os.Rename(temp.Name(), filepath.Join(dir, name))
}

func (e *Engine) matchingRules(address string) []compiledRule {
	matches, _ := e.requestRules(address)
	return matches
}

func (e *Engine) requestRules(address string) ([]compiledRule, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var matches []compiledRule
	var hasAllow, allowed bool
	for _, rule := range e.trafficRules {
		if !rule.Enabled {
			continue
		}
		if rule.Kind == "allow" {
			hasAllow = true
		}
		if rule.matches(address) {
			matches = append(matches, rule)
			if rule.Kind == "allow" {
				allowed = true
			}
		}
	}
	return matches, hasAllow && !allowed
}

func (rule compiledRule) matches(address string) bool {
	if rule.regex != nil {
		return rule.regex.MatchString(address)
	}
	return strings.Contains(address, rule.Pattern)
}

func updateRuleHeader(r compiledRule, headers http.Header, stage string) {
	if !strings.HasPrefix(r.Kind, stage+"-") {
		return
	}
	switch strings.TrimPrefix(r.Kind, stage+"-") {
	case "set":
		headers.Set(r.Name, r.Value)
	case "add":
		headers.Add(r.Name, r.Value)
	case "remove":
		headers.Del(r.Name)
	}
}

func syntheticResponse(status int, contentType string, size int64, body io.ReadCloser) *http.Response {
	headers := make(http.Header)
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	headers.Set("Content-Length", strconv.FormatInt(size, 10))
	return &http.Response{
		StatusCode: status, Proto: "HTTP/1.1", Header: headers, Body: body, ContentLength: size,
	}
}

func stringResponse(status int, contentType, body string) *http.Response {
	if status == 204 || status == 304 {
		body = ""
	}
	return syntheticResponse(status, contentType, int64(len(body)), io.NopCloser(strings.NewReader(body)))
}

func mappedResponse(rule compiledRule, req *http.Request) (*http.Response, error) {
	var f *os.File
	var err error
	if rule.Kind == "directory" {
		prefix := strings.TrimSuffix(rule.Name, "/")
		path := req.URL.Path
		if path != prefix && !strings.HasPrefix(path, prefix+"/") {
			return nil, nil
		}
		name := strings.TrimPrefix(strings.TrimPrefix(path, prefix), "/")
		if name == "" {
			name = "index.html"
		}
		root, err := os.OpenRoot(rule.Value)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		f, err = root.Open(filepath.FromSlash(name))
		if err != nil {
			return nil, err
		}
	} else {
		f, err = os.Open(rule.Value)
		if err != nil {
			return nil, err
		}
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("本地映射需要普通文件")
	}
	contentType := mime.TypeByExtension(filepath.Ext(info.Name()))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return syntheticResponse(200, contentType, info.Size(), f), nil
}

func pause(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type throttledWriter struct {
	context context.Context
	out     io.Writer
	delay   time.Duration
}

func (w *throttledWriter) Write(p []byte) (int, error) {
	if err := pause(w.context, w.delay*time.Duration(len(p))); err != nil {
		return 0, err
	}
	return w.out.Write(p)
}
