//go:build darwin && cgo

package systemproxy

import "testing"

func TestNativeSystemSettingsReadOnly(t *testing.T) {
	d := darwinDriver{}
	plan, err := d.prepare("127.0.0.1", 19090)
	if err != nil {
		t.Skipf("active network service unavailable: %v", err)
	}
	current, err := d.current(plan.Service)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Before) != 8 || !equal(plan.Before, current) || !d.references(plan.After, "127.0.0.1:19090") {
		t.Fatal("native proxy snapshot did not round-trip")
	}
}
