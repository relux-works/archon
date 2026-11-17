package testtoolchain

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relux-works/curator/internal/managerlock"
)

type hostGOROOTLease struct {
	owner     hostGOROOTOwner
	lock      *HostGOROOTLock
	inherited bool
	tests     []string
}

var hostGOROOTLeases = struct {
	sync.Mutex
	byRoot map[string]*hostGOROOTLease
}{byRoot: make(map[string]*hostGOROOTLease)}

const hostGOROOTLockMarker = "CURATOR_TEST_HOST_GOROOT_LOCK"

type hostGOROOTLockMarkerValue struct {
	Owner hostGOROOTOwner `json:"owner"`
	Tests []string        `json:"tests"`
}

// InheritHostGOROOTLock marks one selected `go test -run ...` child to share
// this test's host-toolchain lock. The child must validate the marker against
// the owner record, parent PID, and still-held operating-system lock.
func InheritHostGOROOTLock(t testing.TB, command *exec.Cmd) {
	t.Helper()
	_ = inheritHostGOROOTLockAt(t, command, hostGOROOTLockRoot())
}

func inheritHostGOROOTLockAt(t testing.TB, command *exec.Cmd, root string) bool {
	if command == nil {
		return false
	}
	env := command.Env
	if env == nil {
		env = os.Environ()
	}
	env = withoutHostGOROOTMarker(env)
	command.Env = env
	if !isGoTestCommand(command) {
		return false
	}
	patterns := goTestRunPatterns(command.Args)
	if len(patterns) == 0 {
		return false
	}
	manager, err := managerlock.New(root)
	if err != nil {
		return false
	}
	root = manager.Home()
	lease := hostGOROOTLeaseFor(root, t.Name())
	if lease == nil {
		lease = inheritHostGOROOTLease(root, t.Name())
	}
	if lease != nil {
		marker, err := json.Marshal(hostGOROOTLockMarkerValue{Owner: lease.owner, Tests: patterns})
		if err == nil {
			env = append(env, hostGOROOTLockMarker+"="+string(marker))
		}
	}
	command.Env = env
	return lease != nil
}

// canonicalHostGOROOTRoot resolves a lock root to the same spelling the
// manager lock uses for its home, so aliases of one root (for example the
// /var and /private/var spellings of a macOS temporary directory, a symlink,
// or a case variant on Windows) address one lease. An unresolvable root keeps
// its given spelling and simply finds no lease.
func canonicalHostGOROOTRoot(root string) string {
	manager, err := managerlock.New(root)
	if err != nil {
		return root
	}
	return manager.Home()
}

func hostGOROOTLeaseFor(root, owner string) *hostGOROOTLease {
	root = canonicalHostGOROOTRoot(root)
	hostGOROOTLeases.Lock()
	lease := hostGOROOTLeases.byRoot[root]
	if lease == nil || !hostGOROOTLeaseAdmits(lease, owner) {
		hostGOROOTLeases.Unlock()
		return nil
	}
	clone := *lease
	clone.tests = append([]string(nil), lease.tests...)
	hostGOROOTLeases.Unlock()
	if !clone.inherited || hostGOROOTInheritedLeaseStillHeld(root, clone.owner) {
		return &clone
	}

	// Remove only the entry that failed validation. A concurrent replacement
	// must remain available to its own admitted test scope.
	hostGOROOTLeases.Lock()
	if hostGOROOTLeases.byRoot[root] == lease {
		delete(hostGOROOTLeases.byRoot, root)
	}
	hostGOROOTLeases.Unlock()
	return nil
}

func hostGOROOTInheritedLeaseStillHeld(root string, owner hostGOROOTOwner) bool {
	root = canonicalHostGOROOTRoot(root)
	if !hostGOROOTProcessAlive(owner.PID) || !hostGOROOTOwnerEqual(readHostGOROOTOwner(filepath.Join(root, "holder.json")), owner) {
		return false
	}
	manager, err := managerlock.New(root)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	probe, err := manager.AcquireExistingHomeOnly(ctx)
	cancel()
	if err == nil {
		_ = probe.Close()
		return false
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return false
	}
	return hostGOROOTProcessAlive(owner.PID) && hostGOROOTOwnerEqual(readHostGOROOTOwner(filepath.Join(root, "holder.json")), owner)
}

func releaseInheritedHostGOROOTLease(root string, owner hostGOROOTOwner) {
	root = canonicalHostGOROOTRoot(root)
	hostGOROOTLeases.Lock()
	defer hostGOROOTLeases.Unlock()
	lease := hostGOROOTLeases.byRoot[root]
	if lease != nil && lease.inherited && hostGOROOTOwnerEqual(&lease.owner, owner) {
		delete(hostGOROOTLeases.byRoot, root)
	}
}

func hostGOROOTLeaseAdmits(lease *hostGOROOTLease, owner string) bool {
	if sameTestOrSubtest(lease.owner.Name, owner) {
		return true
	}
	if lease.inherited {
		for _, pattern := range lease.tests {
			if hostGOROOTRunPatternMatches(pattern, owner) {
				return true
			}
		}
	}
	return false
}

func sameTestOrSubtest(parent, child string) bool {
	return parent != "" && (child == parent || strings.HasPrefix(child, parent+"/"))
}

func registerHostGOROOTLease(root string, lease *hostGOROOTLease) {
	root = canonicalHostGOROOTRoot(root)
	hostGOROOTLeases.Lock()
	hostGOROOTLeases.byRoot[root] = lease
	hostGOROOTLeases.Unlock()
	if lease.lock != nil {
		lease.lock.onClose = func() {
			hostGOROOTLeases.Lock()
			if hostGOROOTLeases.byRoot[root] == lease {
				delete(hostGOROOTLeases.byRoot, root)
			}
			hostGOROOTLeases.Unlock()
		}
	}
}

func inheritHostGOROOTLease(root, owner string) *hostGOROOTLease {
	raw, ok := os.LookupEnv(hostGOROOTLockMarker)
	if !ok {
		return nil
	}
	// Consume the process-local copy. A descendant receives the marker only
	// when its own selected go test command is explicitly marked below.
	_ = os.Unsetenv(hostGOROOTLockMarker)
	if raw == "" || len(raw) > 8192 {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var marker hostGOROOTLockMarkerValue
	if err := decoder.Decode(&marker); err != nil {
		return nil
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil
	}
	if marker.Owner.Name == "" || marker.Owner.PID <= 0 || marker.Owner.PID == os.Getpid() || marker.Owner.Process == "" || marker.Owner.AcquiredAt.IsZero() || marker.Owner.AcquiredAt.After(time.Now()) || len(marker.Tests) == 0 {
		return nil
	}
	allowed := false
	for _, pattern := range marker.Tests {
		if hostGOROOTRunPatternMatches(pattern, owner) {
			allowed = true
			break
		}
	}
	if !allowed || !hostGOROOTInheritedLeaseStillHeld(root, marker.Owner) {
		return nil
	}
	lease := &hostGOROOTLease{owner: marker.Owner, inherited: true, tests: append([]string(nil), marker.Tests...)}
	registerHostGOROOTLease(root, lease)
	return lease
}

func hostGOROOTOwnerEqual(actual *hostGOROOTOwner, expected hostGOROOTOwner) bool {
	return actual != nil && actual.Name == expected.Name && actual.PID == expected.PID && actual.Process == expected.Process && actual.AcquiredAt.Equal(expected.AcquiredAt)
}

func hostGOROOTRunPatternMatches(pattern, testName string) bool {
	patterns := strings.Split(pattern, "/")
	names := strings.Split(testName, "/")
	for index, name := range names {
		if index >= len(patterns) {
			break
		}
		part := patterns[index]
		expression, err := regexp.Compile(part)
		if err != nil || !expression.MatchString(name) {
			return false
		}
	}
	return true
}

func goTestRunPatterns(args []string) []string {
	for index := 1; index < len(args); index++ {
		if args[index] == "-run" && index+1 < len(args) {
			if strings.TrimSpace(args[index+1]) == "" {
				return nil
			}
			return []string{args[index+1]}
		}
		if strings.HasPrefix(args[index], "-run=") {
			pattern := strings.TrimPrefix(args[index], "-run=")
			if strings.TrimSpace(pattern) == "" {
				return nil
			}
			return []string{pattern}
		}
	}
	return nil
}

func isGoTestCommand(command *exec.Cmd) bool {
	parts := append([]string{filepath.Base(command.Path)}, command.Args[1:]...)
	for index, part := range parts {
		base := strings.TrimSuffix(filepath.Base(part), ".exe")
		if base == "go" && index+1 < len(parts) && parts[index+1] == "test" {
			return true
		}
	}
	return false
}

func withoutHostGOROOTMarker(env []string) []string {
	clean := make([]string, 0, len(env)+1)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if key != hostGOROOTLockMarker {
			clean = append(clean, entry)
		}
	}
	return clean
}

func readHostGOROOTRecord(file io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(file, 4097))
}
