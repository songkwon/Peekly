//go:build !darwin && !windows

package capture

import (
	"context"
	"net"
)

func localProcess(context.Context, *net.TCPAddr, *net.TCPAddr) (string, int) {
	return "", 0
}
