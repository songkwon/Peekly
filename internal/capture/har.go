package capture

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type nameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func headerPairs(headers http.Header) []nameValue {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]nameValue, 0, len(keys))
	for _, key := range keys {
		for _, value := range headers[key] {
			values = append(values, nameValue{Name: key, Value: value})
		}
	}
	return values
}

func queryPairs(rawURL string) []nameValue {
	target, err := url.Parse(rawURL)
	if err != nil {
		return []nameValue{}
	}
	return headerPairs(http.Header(target.Query()))
}

func (e *Engine) HAR(ids []uint64) ([]byte, error) { return e.ExportHAR(ids, false) }

func (e *Engine) ExportHAR(ids []uint64, redact bool) ([]byte, error) {
	selected := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	entries := make([]map[string]any, 0, len(selected))
	for _, d := range e.store.entries() {
		if !selected[d.ID] || d.Pending {
			continue
		}
		if redact {
			d.URL = redactURL(d.URL)
			d.RequestHeaders = redactHeaders(d.RequestHeaders)
			d.ResponseHeaders = redactHeaders(d.ResponseHeaders)
			d.RequestBody, d.ResponseBody = nil, nil
			d.Error = ""
		}
		entry := map[string]any{
			"startedDateTime": d.Started, "time": d.Duration, "_peeklyRedacted": redact,
			"cache":   map[string]any{},
			"timings": map[string]int64{"send": -1, "wait": -1, "receive": -1},
		}
		if d.Timing != nil {
			entry["timings"] = map[string]int64{"dns": d.Timing.DNS, "connect": d.Timing.Connect, "ssl": d.Timing.TLS, "send": -1, "wait": d.Timing.Wait, "receive": d.Timing.Receive}
		}
		request := map[string]any{
			"method": d.Method, "url": d.URL, "httpVersion": d.Protocol,
			"headers": headerPairs(d.RequestHeaders), "queryString": queryPairs(d.URL),
			"cookies": []any{}, "headersSize": -1, "bodySize": d.RequestBytes,
		}
		if len(d.RequestBody) > 0 {
			post := map[string]any{"mimeType": d.RequestHeaders.Get("Content-Type")}
			addHARText(post, d.RequestBody, d.RequestHeaders.Get("Content-Encoding"), "_encoding")
			request["postData"] = post
		}
		content := map[string]any{"size": d.Size, "mimeType": d.ContentType}
		addHARText(content, d.ResponseBody, d.ResponseHeaders.Get("Content-Encoding"), "encoding")
		entry["request"] = request
		entry["response"] = map[string]any{
			"status": d.Status, "statusText": http.StatusText(d.Status), "httpVersion": d.ResponseProtocol,
			"headers": headerPairs(d.ResponseHeaders), "cookies": []any{}, "content": content,
			"headersSize": -1, "bodySize": d.Size, "redirectURL": d.ResponseHeaders.Get("Location"),
		}
		if d.Error != "" {
			entry["_error"] = d.Error
		}
		if d.RequestTruncated || d.ResponseTruncated {
			entry["_peeklyTruncated"] = map[string]bool{"request": d.RequestTruncated, "response": d.ResponseTruncated}
		}
		entries = append(entries, entry)
	}
	return json.MarshalIndent(map[string]any{"log": map[string]any{
		"version": "1.2", "creator": map[string]string{"name": "Peekly", "version": "0.3.0"}, "entries": entries,
	}}, "", "  ")
}

func addHARText(target map[string]any, body []byte, contentEncoding, encodingKey string) {
	if len(body) == 0 {
		return
	}
	if (contentEncoding != "" && contentEncoding != "identity") || !utf8.Valid(body) {
		target["text"] = base64.StdEncoding.EncodeToString(body)
		target[encodingKey] = "base64"
		return
	}
	target["text"] = string(body)
}

type importedMessage struct {
	Method      string      `json:"method"`
	URL         string      `json:"url"`
	HTTPVersion string      `json:"httpVersion"`
	Status      int         `json:"status"`
	Headers     []nameValue `json:"headers"`
	BodySize    int64       `json:"bodySize"`
	PostData    struct {
		Text     string `json:"text"`
		Encoding string `json:"_encoding"`
		MimeType string `json:"mimeType"`
	} `json:"postData"`
	Content struct {
		Text     string `json:"text"`
		Encoding string `json:"encoding"`
		MimeType string `json:"mimeType"`
	} `json:"content"`
}

func (e *Engine) ImportHAR(data []byte) (int, error) {
	var document struct {
		Log *struct {
			Entries []struct {
				Started  time.Time       `json:"startedDateTime"`
				Time     float64         `json:"time"`
				Request  importedMessage `json:"request"`
				Response importedMessage `json:"response"`
			} `json:"entries"`
		} `json:"log"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return 0, fmt.Errorf("无法读取 HAR: %w", err)
	}
	if document.Log == nil {
		return 0, errors.New("文件缺少 HAR 日志")
	}
	entries := document.Log.Entries
	if len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}
	count := 0
	for _, entry := range entries {
		target, err := url.Parse(entry.Request.URL)
		if err != nil || target.Hostname() == "" || (target.Scheme != "http" && target.Scheme != "https") || target.User != nil {
			continue
		}
		started := entry.Started
		if started.IsZero() {
			started = time.Now()
		}
		content := entry.Response.Content.MimeType
		kind := strings.ToUpper(target.Scheme)
		if strings.EqualFold(entry.Request.Method, "CONNECT") {
			kind = "Tunnel"
		}
		if entry.Response.Status == 101 {
			kind = "WebSocket"
		}
		row := Row{
			Started: started, Method: entry.Request.Method, URL: target.String(),
			Host: normalizeHost(target.Hostname()), Path: target.RequestURI(), Kind: kind,
			Status: entry.Response.Status, Duration: int64(max(0, entry.Time)),
			Size: max(0, entry.Response.BodySize), ContentType: content, Source: "HAR",
		}
		requestBody, reqTruncated := readHARBody(entry.Request.PostData.Text, entry.Request.PostData.Encoding)
		responseBody, resTruncated := readHARBody(entry.Response.Content.Text, entry.Response.Content.Encoding)
		row.Size = max(row.Size, int64(len(responseBody)))
		id := e.store.insert(row)
		detail := Detail{
			Row: row, Protocol: entry.Request.HTTPVersion, ResponseProtocol: entry.Response.HTTPVersion,
			RequestHeaders: pairsHeader(entry.Request.Headers), ResponseHeaders: pairsHeader(entry.Response.Headers),
			RequestBody: requestBody, ResponseBody: responseBody, RequestBytes: max(entry.Request.BodySize, int64(len(requestBody))),
			RequestTruncated: reqTruncated, ResponseTruncated: resTruncated,
		}
		e.store.update(id, detail)
		count++
	}
	return count, nil
}

func readHARBody(text, encoding string) ([]byte, bool) {
	if text == "" {
		return nil, false
	}
	data := []byte(text)
	if encoding == "base64" {
		var err error
		data, err = base64.StdEncoding.DecodeString(text)
		if err != nil {
			return nil, false
		}
	}
	if len(data) > maxBody {
		return append([]byte(nil), data[:maxBody]...), true
	}
	return data, false
}

func pairsHeader(values []nameValue) http.Header {
	headers := make(http.Header)
	for _, value := range values {
		if validHeaderName(value.Name) && !invalidHeaderValue(value.Value) {
			headers.Add(value.Name, value.Value)
		}
	}
	return headers
}

func redactURL(raw string) string {
	target, err := url.Parse(raw)
	if err != nil {
		return "[redacted]"
	}
	target.User, target.Fragment = nil, ""
	query := target.Query()
	for name := range query {
		query[name] = []string{"[redacted]"}
	}
	target.RawQuery = query.Encode()
	return target.String()
}

func redactHeaders(headers http.Header) http.Header {
	// Preserve debugging metadata and omit application-specific header values.
	out := make(http.Header)
	for key, values := range headers {
		switch http.CanonicalHeaderKey(key) {
		case "Content-Type", "Content-Length", "Content-Encoding", "Accept", "Accept-Encoding", "Cache-Control", "Connection", "Date":
			out[key] = values
		default:
			out[key] = []string{"[redacted]"}
		}
	}
	return out
}
