//go:build darwin || linux

package testtoolchain

import (
	"errors"

	"golang.org/x/sys/unix"
)

func hostGOROOTProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := unix.Kill(pid, 0)
	return err == nil || errors.Is(err, unix.EPERM)
}
