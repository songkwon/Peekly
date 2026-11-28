//go:build darwin && cgo

package capture

/*
#cgo LDFLAGS: -framework AppKit -framework CoreGraphics
#include <stdlib.h>
char *peekly_process_bundle(int pid);
void peekly_application_details(const char *path, char **name, char **png);
*/
import "C"

import "unsafe"

func processBundle(pid int) string {
	path := C.peekly_process_bundle(C.int(pid))
	defer C.free(unsafe.Pointer(path))
	return C.GoString(path)
}

func applicationDetails(path string) (string, string) {
	input := C.CString(path)
	defer C.free(unsafe.Pointer(input))
	var name, png *C.char
	C.peekly_application_details(input, &name, &png)
	defer C.free(unsafe.Pointer(name))
	defer C.free(unsafe.Pointer(png))
	return C.GoString(name), C.GoString(png)
}
