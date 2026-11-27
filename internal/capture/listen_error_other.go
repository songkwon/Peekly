//go:build !windows

package capture

import "syscall"

const addressInUse = syscall.EADDRINUSE
