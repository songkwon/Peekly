package capture

import (
	"context"
	"net"
	"net/http"
	"sync"
)

type clientKey struct{}

type Client struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
	PID     int    `json:"pid,omitempty"`
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
			client.Name, client.PID = localProcess(ctx, peer, local)
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
