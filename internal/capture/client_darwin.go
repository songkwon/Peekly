package capture

import (
	"context"
	"encoding/hex"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var processLookups = make(chan struct{}, 4)

func localProcess(ctx context.Context, peer, local *net.TCPAddr) (string, int, string) {
	select {
	case processLookups <- struct{}{}:
		defer func() { <-processLookups }()
	default:
		return "", 0, ""
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
				return decodeLSOFName(name), pid, processBundle(pid)
			}
		}
	}
	return "", 0, ""
}

func decodeLSOFName(name string) string {
	if !strings.Contains(name, `\x`) {
		return name
	}
	var decoded strings.Builder
	for len(name) > 0 {
		if len(name) >= 2 && name[:2] == `\\` {
			decoded.WriteByte('\\')
			name = name[2:]
			continue
		}
		if len(name) >= 4 && name[:2] == `\x` {
			octet, err := hex.DecodeString(name[2:4])
			if err == nil {
				decoded.WriteByte(octet[0])
				name = name[4:]
				continue
			}
		}
		decoded.WriteByte(name[0])
		name = name[1:]
	}
	return strings.ToValidUTF8(decoded.String(), string(utf8.RuneError))
}
