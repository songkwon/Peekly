package capture

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	xproxy "golang.org/x/net/proxy"
)

type NetworkOptions struct {
	Bind     string            `json:"bind"`
	Clients  []string          `json:"clients"`
	Upstream string            `json:"upstream"`
	Reverse  string            `json:"reverse"`
	DNS      map[string]string `json:"dns"`
}

type networkConfig struct {
	NetworkOptions
	prefixes []netip.Prefix
	upstream *url.URL
	reverse  *url.URL
	dns      map[string]string
}

func LocalAddresses() []string {
	result := []string{"127.0.0.1"}
	addresses, _ := net.InterfaceAddrs()
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err == nil && ip.To4() != nil && !ip.IsLoopback() {
			result = append(result, ip.String())
		}
	}
	return result
}

func (e *Engine) Network() NetworkOptions {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.network.NetworkOptions
}

func (e *Engine) SetNetwork(input NetworkOptions) error {
	next, err := compileNetwork(input)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.run != nil {
		return errors.New("停止抓包后才可修改网络设置")
	}
	previous := e.transport
	transport := newTransport(next)
	e.transport, e.network = transport, next
	previous.CloseIdleConnections()
	return nil
}

func newTransport(config networkConfig) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if config.upstream != nil {
		transport.Proxy = http.ProxyURL(config.upstream)
	}
	transport.DialContext = config.dial
	transport.ResponseHeaderTimeout = 45 * time.Second
	transport.DisableCompression = true
	return transport
}

func compileNetwork(input NetworkOptions) (networkConfig, error) {
	config := networkConfig{NetworkOptions: input, dns: make(map[string]string)}
	config.Bind = strings.TrimSpace(input.Bind)
	if config.Bind == "" {
		config.Bind = "127.0.0.1"
	}
	matched := false
	for _, address := range LocalAddresses() {
		if config.Bind == address {
			matched = true
			break
		}
	}
	if !matched {
		return config, errors.New("仅支持绑定本机 IPv4 地址")
	}
	if len(input.Clients) > 32 || len(input.DNS) > 64 {
		return config, errors.New("网络规则数量过多")
	}
	for _, raw := range input.Clients {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			ip, err := netip.ParseAddr(raw)
			if err != nil {
				return config, fmt.Errorf("无效客户端地址 %q", raw)
			}
			prefix = netip.PrefixFrom(ip, ip.BitLen())
		}
		if prefix.Bits() == 0 {
			return config, errors.New("请输入具体的客户端地址或子网")
		}
		config.prefixes = append(config.prefixes, prefix)
	}
	if config.Bind != "127.0.0.1" && len(config.prefixes) == 0 {
		return config, errors.New("局域网监听需要至少一个客户端地址")
	}
	for name, val := range input.DNS {
		host := normalizeHost(name)
		ip := net.ParseIP(strings.TrimSpace(val))
		if !validHost(host) || ip == nil {
			return config, fmt.Errorf("无效 DNS 映射 %q", name)
		}
		config.dns[host] = ip.String()
	}
	config.DNS = config.dns
	if input.Upstream != "" {
		proxy, err := url.Parse(strings.TrimSpace(input.Upstream))
		if err != nil || proxy.Hostname() == "" || proxy.User != nil || proxy.RawQuery != "" || proxy.Fragment != "" || proxy.Path != "" && proxy.Path != "/" ||
			proxy.Scheme != "http" && proxy.Scheme != "https" && proxy.Scheme != "socks5" {
			return config, errors.New("上游代理需要 HTTP、HTTPS 或 SOCKS5 URL")
		}
		config.upstream = proxy
	}
	if input.Reverse != "" {
		dest, err := url.Parse(strings.TrimSpace(input.Reverse))
		if err != nil || dest.Hostname() == "" || dest.User != nil || dest.Scheme != "http" && dest.Scheme != "https" || dest.RawQuery != "" || dest.Fragment != "" {
			return config, errors.New("反向代理需要 HTTP 或 HTTPS 目标 URL")
		}
		config.reverse = dest
	}
	return config, nil
}

func (c networkConfig) permits(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.String() == c.Bind {
		return true
	}
	for _, allowed := range c.prefixes {
		if allowed.Contains(ip) {
			return true
		}
	}
	return false
}

func (c networkConfig) dial(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	host, port, err := net.SplitHostPort(address)
	if err == nil {
		if mapped, exists := c.dns[normalizeHost(host)]; exists {
			address = net.JoinHostPort(mapped, port)
		}
	}
	return dialer.DialContext(ctx, network, address)
}

func (e *Engine) networkConfig() networkConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.network
}

func (e *Engine) roundTrip(req *http.Request) (*http.Response, error) {
	e.mu.Lock()
	transport := e.transport
	e.mu.Unlock()
	return transport.RoundTrip(req)
}

func (c networkConfig) tunnel(ctx context.Context, target string) (net.Conn, error) {
	upstream := c.upstream
	if upstream == nil {
		return c.dial(ctx, "tcp", target)
	}
	const timeout = 15 * time.Second
	host, port := upstream.Hostname(), upstream.Port()
	if port == "" {
		switch upstream.Scheme {
		case "https":
			port = "443"
		case "socks5":
			port = "1080"
		default:
			port = "80"
		}
	}
	address := net.JoinHostPort(host, port)
	if upstream.Scheme == "socks5" {
		dialer, err := xproxy.SOCKS5("tcp", address, nil, &net.Dialer{Timeout: timeout})
		if err != nil {
			return nil, err
		}
		return dialer.(xproxy.ContextDialer).DialContext(ctx, "tcp", target)
	}
	conn, err := c.dial(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	base := conn
	succeeded := false
	defer func() {
		if !succeeded {
			_ = base.Close()
		}
	}()
	_ = base.SetDeadline(time.Now().Add(timeout))
	if upstream.Scheme == "https" {
		secure := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := secure.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = secure
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Host: target}, Host: target, Header: make(http.Header)}
	if err := req.Write(conn); err != nil {
		return nil, err
	}
	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("上游代理返回 %d", resp.StatusCode)
	}
	_ = base.SetDeadline(time.Time{})
	succeeded = true
	return &bufferedConn{Conn: conn, reader: r}, nil
}
