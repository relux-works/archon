//go:build harness_parity && linux

package harnessparity

import "syscall"

func selectCreationMask(control string) {
	mask := 0o077
	if control == "0022" {
		mask = 0o022
	}
	syscall.Umask(mask)
}
