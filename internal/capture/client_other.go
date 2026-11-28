//go:build !darwin && !windows

package capture

import (
	"context"
	"net"
)

func localProcess(context.Context, *net.TCPAddr, *net.TCPAddr) (string, int, string) {
	return "", 0, ""
}

func applicationDetails(string) (string, string) { return "", "" }
