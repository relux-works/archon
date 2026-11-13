//go:build linux

package envprofile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func bindingPayloadWatch(t *testing.T, home string) int {
	t.Helper()
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(fd) })
	if _, err := unix.InotifyAddWatch(fd, home, unix.IN_OPEN); err != nil {
		t.Fatal(err)
	}
	return fd
}

// The watcher sees successful opens of this synthetic credential fixture.
// It does not claim whole-host syscall coverage or exclude kernel atime.
func bindingNoPayloadOpen(fd int) error {
	var buffer [4096]byte
	for {
		n, err := unix.Read(fd, buffer[:])
		if errors.Is(err, unix.EAGAIN) {
			return nil
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		for offset := 0; offset+unix.SizeofInotifyEvent <= n; {
			length := int(binary.NativeEndian.Uint32(buffer[offset+12 : offset+16]))
			end := offset + unix.SizeofInotifyEvent + length
			if end > n {
				return fmt.Errorf("truncated fixture open event")
			}
			name := string(bytes.TrimRight(buffer[offset+unix.SizeofInotifyEvent:end], "\x00"))
			if name == "auth.json" {
				return fmt.Errorf("native credential payload opened")
			}
			offset = end
		}
	}
}

// Real bound purge does not open the synthetic native credential payload;
// an intentional read is detected by the same checker as a nearby control.
func TestNativeHomeBindingPurgeNoCredentialOpen(t *testing.T) {
	manager := t.TempDir()
	bindings := bindingPurgeHomes(t)
	native := bindings["codex_cli"]
	bindingPurgeMarker(t, native, "sample", "AGENTS.md")
	payload := filepath.Join(native, "auth.json")
	if err := os.WriteFile(payload, []byte("opaque fixture data\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fd := bindingPayloadWatch(t, native)
	if err := purgeHomesWithBindings(manager, "sample", bindings); err != nil {
		t.Fatal(err)
	}
	if err := bindingNoPayloadOpen(fd); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(payload); err != nil {
		t.Fatal(err)
	}
	if err := bindingNoPayloadOpen(fd); err == nil || err.Error() != "native credential payload opened" {
		t.Fatalf("payload-open checker missed intended plant: %v", err)
	}
}
