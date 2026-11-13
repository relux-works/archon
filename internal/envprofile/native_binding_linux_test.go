//go:build linux

package envprofile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/relux-works/curator/internal/envmarker"
	"os"
	"path/filepath"
	"strings"
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

// A marker replaced after metadata admission refuses before opening the
// synthetic credential payload. The plant must actually reach that boundary.
func TestNativeHomeBindingPurgeMarkerSwap(t *testing.T) {
	manager := bindingDirectory(t)
	bindings := bindingPurgeHomes(t)
	native := bindings["codex_cli"]
	bindingPurgeMarker(t, native, "sample", "AGENTS.md")
	if err := os.WriteFile(filepath.Join(native, "auth.json"), []byte("opaque fixture data\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fd := bindingPayloadWatch(t, native)
	previous := beforeNativePurgeMarkerRead
	defer func() { beforeNativePurgeMarkerRead = previous }()
	planted := false
	beforeNativePurgeMarkerRead = func(home string) error {
		if home != native {
			return nil
		}
		path := filepath.Join(home, envmarker.Name)
		if err := os.Rename(path, path+".saved"); err != nil {
			return err
		}
		if err := os.Symlink("auth.json", path); err != nil {
			return err
		}
		planted = true
		return nil
	}
	err := purgeHomesWithBindings(manager, "sample", bindings)
	if !planted {
		t.Fatal("marker swap was not applied")
	}
	if err == nil || !strings.Contains(err.Error(), envmarker.DiagMarkerUnreadable) {
		t.Fatalf("swapped marker refusal=%v", err)
	}
	if err := bindingNoPayloadOpen(fd); err != nil {
		t.Fatalf("credential opened after marker swap: %v", err)
	}
}
