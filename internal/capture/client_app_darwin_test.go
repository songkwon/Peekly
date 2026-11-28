//go:build darwin && cgo

package capture

import (
	"encoding/base64"
	"image/png"
	"strings"
	"testing"
)

func TestMacApplicationIcon(t *testing.T) {
	info := cachedApplication("/System/Library/CoreServices/Finder.app")
	if info.name == "" || info.key == "" {
		t.Fatalf("missing Finder name or icon")
	}
	icon, err := png.Decode(base64.NewDecoder(base64.StdEncoding, strings.NewReader(info.png)))
	if err != nil {
		t.Fatal(err)
	}
	if icon.Bounds().Dx() != 32 || icon.Bounds().Dy() != 32 {
		t.Fatalf("unexpected icon size: %v", icon.Bounds())
	}
}
