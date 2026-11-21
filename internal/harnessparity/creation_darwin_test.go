//go:build harness_parity && darwin

package harnessparity

import "syscall"

// Establish the admitted capture policy before the unchanged legacy API.
func selectCreationMask(control string) {
	mask := 0o077
	if control == "0022" {
		mask = 0o022
	}
	syscall.Umask(mask)
}
