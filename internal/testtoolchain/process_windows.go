//go:build windows

package testtoolchain

import (
	"errors"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code GetExitCodeProcess reports for a running
// process (STILL_ACTIVE). It is only a fallback for a failed wait below.
const stillActive = 259

func hostGOROOTProcessAlive(pid int) bool {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// A protected process refuses the open but still exists, matching
		// the EPERM case of the Unix probe. Every other refusal, including
		// an unknown PID, fails closed.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	event, err := windows.WaitForSingleObject(handle, 0)
	if err == nil {
		return event == uint32(windows.WAIT_TIMEOUT)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}
