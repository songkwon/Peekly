package capture

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	extractIcon        = windows.NewLazySystemDLL("shell32.dll").NewProc("ExtractIconExW")
	clientGDI          = windows.NewLazySystemDLL("gdi32.dll")
	clientUser         = windows.NewLazySystemDLL("user32.dll")
	createCompatibleDC = clientGDI.NewProc("CreateCompatibleDC")
	deleteDC           = clientGDI.NewProc("DeleteDC")
	createDIBSection   = clientGDI.NewProc("CreateDIBSection")
	selectObject       = clientGDI.NewProc("SelectObject")
	deleteObject       = clientGDI.NewProc("DeleteObject")
	flushGDI           = clientGDI.NewProc("GdiFlush")
	drawIconEx         = clientUser.NewProc("DrawIconEx")
	destroyIcon        = clientUser.NewProc("DestroyIcon")
)

func applicationDetails(path string) (string, string) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", ""
	}
	var handle uintptr
	result, _, _ := extractIcon.Call(uintptr(unsafe.Pointer(name)), 0, 0, uintptr(unsafe.Pointer(&handle)), 1)
	if result != 1 || handle == 0 {
		return "", ""
	}
	defer destroyIcon.Call(handle)

	var encoded bytes.Buffer
	if icon := windowsIconImage(handle); icon != nil && png.Encode(&encoded, icon) == nil {
		return "", base64.StdEncoding.EncodeToString(encoded.Bytes())
	}
	return "", ""
}

func windowsIconImage(icon uintptr) *image.NRGBA {
	const size = 16
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dc, _, _ := createCompatibleDC.Call(0)
	if dc == 0 {
		return nil
	}
	defer deleteDC.Call(dc)

	header := struct {
		length            uint32
		width, height     int32
		planes, bitCount  uint16
		compression, size uint32
		xPixels, yPixels  int32
		colors, important uint32
	}{length: 40, width: size, height: -size, planes: 1, bitCount: 32}
	var address unsafe.Pointer
	bitmap, _, _ := createDIBSection.Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&address)), 0, 0)
	if bitmap == 0 {
		return nil
	}
	defer deleteObject.Call(bitmap)
	if address == nil {
		return nil
	}
	previous, _, _ := selectObject.Call(dc, bitmap)
	if previous == 0 || previous == ^uintptr(0) {
		return nil
	}
	defer selectObject.Call(dc, previous)

	pixels := unsafe.Slice((*byte)(address), size*size*4)
	clear(pixels)
	const drawNormal = 3
	if ok, _, _ := drawIconEx.Call(dc, 0, 0, icon, size, size, 0, 0, drawNormal); ok == 0 {
		return nil
	}
	flushGDI.Call()
	black := append([]byte(nil), pixels...)
	for i := range pixels {
		pixels[i] = 255
	}
	if ok, _, _ := drawIconEx.Call(dc, 0, 0, icon, size, size, 0, 0, drawNormal); ok == 0 {
		return nil
	}

	flushGDI.Call()

	// Drawing over black and white recovers opacity for both alpha icons and legacy mask icons.
	result := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < len(pixels); i += 4 {
		transparency := (int(pixels[i]) - int(black[i]) + int(pixels[i+1]) - int(black[i+1]) + int(pixels[i+2]) - int(black[i+2])) / 3
		alpha := 255 - min(255, max(0, transparency))
		if alpha != 0 {
			restore := func(channel byte) uint8 { return uint8(min(255, (int(channel)*255+alpha/2)/alpha)) }
			result.SetNRGBA(i/4%size, i/4/size, color.NRGBA{
				R: restore(black[i+2]), G: restore(black[i+1]), B: restore(black[i]), A: uint8(alpha),
			})
		}
	}
	return result
}
