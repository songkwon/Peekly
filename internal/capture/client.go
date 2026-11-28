package capture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"sync"
)

type clientKey struct{}

type Client struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Icon    string `json:"icon,omitempty"`
	png     string
}

func connectionClient(ctx context.Context, conn net.Conn) context.Context {
	peer, peerOK := conn.RemoteAddr().(*net.TCPAddr)
	local, localOK := conn.LocalAddr().(*net.TCPAddr)
	if !peerOK || !localOK {
		return ctx
	}
	return context.WithValue(ctx, clientKey{}, sync.OnceValue(func() *Client {
		client := &Client{Address: peer.IP.String()}
		if peer.IP.IsLoopback() || peer.IP.Equal(local.IP) {
			var path string
			client.Name, client.PID, path = localProcess(ctx, peer, local)
			if path != "" {
				app := cachedApplication(path)
				if app.name != "" {
					client.Name = app.name
				}
				client.Icon, client.png = app.key, app.png
			}
		}
		return client
	}))
}

func requestClient(r *http.Request) *Client {
	if client, ok := r.Context().Value(clientKey{}).(func() *Client); ok {
		return client()
	}
	return nil
}

type applicationInfo struct {
	key, name, png string
}

var applicationCache = struct {
	sync.Mutex
	items map[string]applicationInfo
}{items: make(map[string]applicationInfo)}

func cachedApplication(path string) applicationInfo {
	applicationCache.Lock()
	defer applicationCache.Unlock()

	if info, ok := applicationCache.items[path]; ok {
		return info
	}
	name, png := applicationDetails(path)
	info := applicationInfo{name: name, png: png}
	if png != "" {
		digest := sha256.Sum256([]byte(path))
		info.key = hex.EncodeToString(digest[:12])
	}
	if len(applicationCache.items) == 128 {
		for key := range applicationCache.items {
			delete(applicationCache.items, key)
			break
		}
	}
	applicationCache.items[path] = info
	return info
}
