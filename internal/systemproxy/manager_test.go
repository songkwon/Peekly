package systemproxy

import (
	"errors"
	"fmt"
	"testing"
)

const endpoint = "127.0.0.1:9090"

type memoryDriver struct {
	state          fields
	failAfterWrite bool
	calls          []bool
}

func (d *memoryDriver) prepare(host string, port int) (record, error) {
	return record{Service: "LAN", Before: d.state, After: fields{"server": fmt.Sprintf("%s:%d", host, port), "flag": "enabled"}}, nil
}

func (d *memoryDriver) current(string) (fields, error) { return d.state, nil }

func (d *memoryDriver) replace(_ string, from, to fields, prompt bool) error {
	if !equal(d.state, from) {
		return errChanged
	}
	d.calls = append(d.calls, prompt)
	d.state = to
	if d.failAfterWrite {
		d.failAfterWrite = false
		return errors.New("error while notifying process")
	}
	return nil
}

func (d *memoryDriver) references(current fields, e string) bool {
	return current["server"] == e && current["flag"] == "enabled"
}

func setup(t *testing.T) (*Manager, *memoryDriver, fields) {
	t.Helper()
	manager := New(t.TempDir())
	before := fields{"server": "original:8888", "flag": "enabled"}
	d := &memoryDriver{state: before}
	manager.driver = d
	return manager, d, before
}

func TestRestoreOriginalAfterRestart(t *testing.T) {
	m, d, original := setup(t)
	if err := m.Enable(endpoint); err != nil {
		t.Fatal(err)
	}
	if !m.Status().Active || !d.references(d.state, endpoint) || len(d.calls) != 1 || !d.calls[0] {
		t.Fatal("system mode unavailable")
	}
	reopened := &Manager{path: m.path, driver: d}
	if !reopened.Status().Pending {
		t.Fatal("missing recovery record")
	}
	if err := reopened.Restore(false); err != nil {
		t.Fatal(err)
	}
	if !equal(original, d.state) || reopened.Status().Pending || len(d.calls) != 2 || d.calls[1] {
		t.Fatal("settings were not restored without prompting")
	}
}

func TestRestorePreservesExternalChange(t *testing.T) {
	m, d, _ := setup(t)
	if err := m.Enable(endpoint); err != nil {
		t.Fatal(err)
	}
	external := fields{"server": "replacement:8080", "flag": "enabled"}
	d.state = external
	if err := m.Restore(false); err != nil {
		t.Fatal(err)
	}
	if !equal(external, d.state) || m.Status().Pending || len(d.calls) != 1 {
		t.Fatal("external state changed")
	}
}

func TestErrorAfterWriteRollsBackAndRetainsRecordOnConflict(t *testing.T) {
	m, d, original := setup(t)
	d.failAfterWrite = true
	if err := m.Enable(endpoint); err == nil {
		t.Fatal("missing failed apply")
	}
	if !equal(original, d.state) || m.Status().Pending {
		t.Fatal("failed to roll back")
	}

	if err := m.Enable(endpoint); err != nil {
		t.Fatal(err)
	}
	d.state = fields{"server": endpoint, "flag": "enabled", "unexpected": true}
	if err := m.Restore(false); err == nil {
		t.Fatal("attempted to override settings changed externally")
	}
	reopened := &Manager{path: m.path, driver: d}
	if !reopened.Status().Pending {
		t.Fatal("cleared a recovery record while still referenced")
	}
}
