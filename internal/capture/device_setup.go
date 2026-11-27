package capture

import (
	"encoding/pem"
	"io"
	"net/http"
)

func (e *Engine) deviceSetup(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Scheme != "http" || (r.URL.Host != "peekly.test" && r.URL.Host != "peekly.test:80") || r.URL.User != nil {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	switch r.URL.Path {
	case "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Peekly 连接检查</title><h1>已通过 Peekly 连接</h1><p>此设备的 HTTP 代理已生效。查看 HTTPS 正文需要安装并信任 Peekly 根证书。</p><p><a href="/Peekly.cer">下载 Peekly 根证书</a></p></html>`)
		}
	case "/Peekly.cer":
		certificate, err := e.Certificate()
		if err != nil {
			http.Error(w, "无法准备 Peekly 根证书", http.StatusInternalServerError)
			return true
		}
		block, _ := pem.Decode([]byte(certificate.PEM))
		if block == nil || block.Type != "CERTIFICATE" {
			http.Error(w, "无法读取 Peekly 根证书", http.StatusInternalServerError)
			return true
		}
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Header().Set("Content-Disposition", "attachment; filename=Peekly.cer")
		if r.Method == http.MethodGet {
			_, _ = w.Write(block.Bytes)
		}
	default:
		http.NotFound(w, r)
	}
	return true
}
