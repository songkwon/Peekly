package main

import (
	"net"
	"strings"
	"testing"
	"time"

	"peekly/internal/capture"
	"peekly/internal/systemproxy"
)

type recordingSystemProxy struct {
	endpoint string
}

func (p *recordingSystemProxy) Status() systemproxy.Status {
	return systemproxy.Status{Active: p.endpoint != ""}
}

func (p *recordingSystemProxy) Enable(endpoint string) error {
	p.endpoint = endpoint
	return nil
}

func (p *recordingSystemProxy) Restore(bool) error {
	p.endpoint = ""
	return nil
}

func TestAutomaticSystemProxyUsesBoundPortWhenPreferredPortIsOccupied(t *testing.T) {
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = occupied.Close() })

	port := occupied.Addr().(*net.TCPAddr).Port
	auto := &recordingSystemProxy{}
	desktop := &Desktop{engine: capture.New(t.TempDir()), systemProxy: auto}
	t.Cleanup(func() { _ = desktop.Stop() })

	// A manually configured client continues to use the preferred port.
	if _, err := desktop.Start(port, false); err == nil || !strings.Contains(err.Error(), "已被占用") {
		t.Fatalf("expected useful manual port error, got %v", err)
	}
	if desktop.Snapshot(0).Running || auto.endpoint != "" {
		t.Fatal("manual collision started a listener or changed system settings")
	}

	endpoint, err := desktop.Start(port, true)
	if err != nil {
		t.Fatal(err)
	}
	state := desktop.Snapshot(0)
	if endpoint == occupied.Addr().String() || endpoint != state.Endpoint || endpoint != auto.endpoint || !state.Running {
		t.Fatalf("reported %q, listener %q, system settings %q", endpoint, state.Endpoint, auto.endpoint)
	}
	for _, address := range []string{endpoint, occupied.Addr().String()} {
		conn, err := net.DialTimeout("tcp4", address, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
	}
	if err := desktop.Stop(); err != nil {
		t.Fatal(err)
	}
	if desktop.Snapshot(0).Running || auto.endpoint != "" {
		t.Fatal("stop failed to clear the alternate listener or system proxy")
	}
}
