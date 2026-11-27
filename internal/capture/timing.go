package capture

import (
	"context"
	"crypto/tls"
	"net/http/httptrace"
	"sync"
	"time"
)

type Timing struct {
	DNS     int64  `json:"dns"`
	Connect int64  `json:"connect"`
	TLS     int64  `json:"tls"`
	Wait    int64  `json:"wait"`
	Receive int64  `json:"receive"`
	Remote  string `json:"remote"`
	Reused  bool   `json:"reused"`
}

type timingTrace struct {
	mu                                sync.Mutex
	dns, connect, tls, written, first time.Time
	value                             Timing
}

func newTrace(ctx context.Context) (context.Context, *timingTrace) {
	t := &timingTrace{value: Timing{DNS: -1, Connect: -1, TLS: -1, Wait: -1, Receive: -1}}
	now := func(field *time.Time) {
		t.mu.Lock()
		defer t.mu.Unlock()
		*field = time.Now()
	}
	record := func(beginning *time.Time, duration *int64) {
		t.mu.Lock()
		defer t.mu.Unlock()
		if !beginning.IsZero() {
			*duration = time.Since(*beginning).Milliseconds()
		}
	}
	trace := &httptrace.ClientTrace{
		DNSStart:     func(httptrace.DNSStartInfo) { now(&t.dns) },
		DNSDone:      func(httptrace.DNSDoneInfo) { record(&t.dns, &t.value.DNS) },
		ConnectStart: func(string, string) { now(&t.connect) },
		ConnectDone: func(_, _ string, err error) {
			if err == nil {
				record(&t.connect, &t.value.Connect)
			}
		},
		TLSHandshakeStart:    func() { now(&t.tls) },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { record(&t.tls, &t.value.TLS) },
		WroteRequest:         func(httptrace.WroteRequestInfo) { now(&t.written) },
		GotFirstResponseByte: func() { now(&t.first); record(&t.written, &t.value.Wait) },
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.value.Remote, t.value.Reused = info.Conn.RemoteAddr().String(), info.Reused
		},
	}
	return httptrace.WithClientTrace(ctx, trace), t
}

func (t *timingTrace) result() *Timing {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := t.value
	if !t.first.IsZero() {
		result.Receive = time.Since(t.first).Milliseconds()
	}
	return &result
}
