package capture

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

func (e *Engine) Matches(filters []Filter, anyMatch bool) ([]uint64, error) {
	if len(filters) > 20 {
		return nil, errors.New("最多 20 个筛选条件")
	}
	type condition struct {
		Filter
		re *regexp.Regexp
	}
	compiled := make([]condition, 0, len(filters))
	for _, f := range filters {
		if len(f.Value) > 2048 {
			return nil, errors.New("筛选内容过长")
		}
		item := condition{Filter: f}
		switch f.Op {
		case "contains", "is", "not", "starts", "ends":
		case "regex":
			var err error
			item.re, err = regexp.Compile("(?i)" + f.Value)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("无效的运算符")
		}
		compiled = append(compiled, item)
	}
	ids := make([]uint64, 0)
	for _, row := range e.store.entries() {
		result := !anyMatch
		for _, f := range compiled {
			value := filterValue(row, f.Field)
			want := strings.ToLower(f.Value)
			var match bool
			switch f.Op {
			case "contains":
				match = strings.Contains(strings.ToLower(value), want)
			case "is":
				match = strings.EqualFold(value, f.Value)
			case "not":
				match = !strings.Contains(strings.ToLower(value), want)
			case "starts":
				match = strings.HasPrefix(strings.ToLower(value), want)
			case "ends":
				match = strings.HasSuffix(strings.ToLower(value), want)
			case "regex":
				match = f.re.MatchString(value)
			}
			if anyMatch && match || !anyMatch && !match {
				result = match
				break
			}
		}
		if result {
			ids = append(ids, row.ID)
		}
	}
	return ids, nil
}

func filterValue(d Detail, field string) string {
	switch field {
	case "url":
		return d.URL
	case "host":
		return d.Host
	case "client":
		if d.Client == nil {
			return d.Source
		}
		if d.Client.Name != "" {
			return d.Client.Name
		}
		if d.Client.PID != 0 {
			return "PID " + strconv.Itoa(d.Client.PID)
		}
		return d.Client.Address
	case "method":
		return d.Method
	case "status":
		return strconv.Itoa(d.Status)
	case "type":
		return d.ContentType
	case "requestBody":
		return string(d.RequestBody)
	case "responseBody":
		return string(d.ResponseBody)
	case "requestHeaders", "responseHeaders":
		headers := d.RequestHeaders
		if field == "responseHeaders" {
			headers = d.ResponseHeaders
		}
		var out strings.Builder
		for name, values := range headers {
			for _, value := range values {
				out.WriteString(name)
				out.WriteString(": ")
				out.WriteString(value)
				out.WriteByte('\n')
			}
		}
		return out.String()
	}
	return ""
}
