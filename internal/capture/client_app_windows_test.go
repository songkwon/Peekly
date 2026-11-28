package capture

import (
	"encoding/base64"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsApplicationIcon(t *testing.T) {
	info := cachedApplication(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"))
	if info.key == "" {
		t.Fatal("missing File Explorer icon")
	}
	icon, err := png.Decode(base64.NewDecoder(base64.StdEncoding, strings.NewReader(info.png)))
	if err != nil {
		t.Fatal(err)
	}
	for y := range icon.Bounds().Dy() {
		for x := range icon.Bounds().Dx() {
			if r, g, b, a := icon.At(x, y).RGBA(); a != 0 && (r != 0 || g != 0 || b != 0) {
				return
			}
		}
	}
	t.Fatal("File Explorer icon is blank")
}
