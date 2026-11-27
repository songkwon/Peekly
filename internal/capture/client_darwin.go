package capture

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var processLookups = make(chan struct{}, 4)

func localProcess(ctx context.Context, peer, local *net.TCPAddr) (string, int) {
	select {
	case processLookups <- struct{}{}:
		defer func() { <-processLookups }()
	default:
		return "", 0
	}
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	data, _ := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "+c", "0", "-a", "-iTCP:"+strconv.Itoa(peer.Port), "-sTCP:ESTABLISHED", "-Fpcn").Output()
	want := peer.String() + "->" + local.String()
	var pid int
	var name string
	for _, line := range strings.Split(string(data), "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
			name = ""
		case 'c':
			name = line[1:]
		case 'n':
			if line[1:] == want && pid > 0 {
				return name, pid
			}
		}
	}
	return "", 0
}
