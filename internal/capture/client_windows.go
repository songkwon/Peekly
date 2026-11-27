package capture

import (
	"context"
	"encoding/binary"
	"net"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var tcpOwners = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

func localProcess(_ context.Context, peer, local *net.TCPAddr) (string, int) {
	const tableOwnerConnections = 4
	size := uint32(0)
	if err := tcpOwners.Find(); err != nil {
		return "", 0
	}
	_, _, _ = tcpOwners.Call(0, uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, tableOwnerConnections, 0)
	for attempt := 0; attempt < 2 && size >= 4 && size <= 8<<20; attempt++ {
		data := make([]byte, size)
		code, _, _ := tcpOwners.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, tableOwnerConnections, 0)
		if code == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			continue
		}
		if code != 0 {
			return "", 0
		}
		count := min(int(binary.LittleEndian.Uint32(data)), (len(data)-4)/24)
		for index := 0; index < count; index++ {
			row := data[4+index*24:][:24]
			if !peer.IP.Equal(net.IP(row[4:8])) || !local.IP.Equal(net.IP(row[12:16])) ||
				peer.Port != int(binary.BigEndian.Uint16(row[8:10])) || local.Port != int(binary.BigEndian.Uint16(row[16:18])) {
				continue
			}
			pid := binary.LittleEndian.Uint32(row[20:24])
			handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			if err != nil {
				return "", int(pid)
			}
			defer windows.CloseHandle(handle)
			var image [1024]uint16
			length := uint32(len(image))
			if err := windows.QueryFullProcessImageName(handle, 0, &image[0], &length); err != nil {
				return "", int(pid)
			}
			return filepath.Base(windows.UTF16ToString(image[:length])), int(pid)
		}
		break
	}
	return "", 0
}
