package testgitenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A bounded post-commit maintenance actor plants a real lock when the fixture
// permits automatic/background work. Removing it between complete images must
// fail the old setup and a forced-overlap control; the quiescent setup passes.
// This deterministic file-protocol control does not emulate Git's scheduler.
func TestDecoyQuiescentBeforeSnapshot(t *testing.T) {
	const childMode = "CURATOR_TEST_DECOY_MAINTENANCE"
	if mode := os.Getenv(childMode); mode != "" {
		planted, omitted := false, 0
		root := newDecoyWithGit(t, func(t *testing.T, root string, args ...string) []byte {
			if len(args) == 3 && args[0] == "config" && mode == "old-setup" {
				switch args[1] {
				case "maintenance.auto", "gc.auto", "gc.autoDetach":
					omitted++
					return nil // Narrowing plant: reproduce the old setup's config.
				}
			}
			// Suppress real asynchronous work in this fault-injection harness:
			// the actor below models it from the repository's actual policy.
			// These process-only options do not populate the local config read
			// by the actor, so omitting the constructor policy remains observable.
			safeArgs := append([]string{"-c", "maintenance.auto=false", "-c", "gc.auto=0", "-c", "gc.autoDetach=false"}, args...)
			out := fixtureGit(t, root, safeArgs...)
			for _, arg := range args {
				if arg != "commit" {
					continue
				}
				// Read real repository configuration at the setup commit boundary,
				// independently of the constructor's list of config commands.
				auto := strings.TrimSpace(string(fixtureGit(t, root, "config", "--default", "true", "--type=bool", "--get", "maintenance.auto")))
				gc := strings.TrimSpace(string(fixtureGit(t, root, "config", "--default", "6700", "--type=int", "--get", "gc.auto")))
				detach := strings.TrimSpace(string(fixtureGit(t, root, "config", "--default", "true", "--type=bool", "--get", "gc.autoDetach")))
				if mode == "forced-overlap" || auto != "false" || gc != "0" || detach != "false" {
					if err := os.WriteFile(filepath.Join(root, ".git", "objects", "maintenance.lock"), []byte("synthetic maintenance\n"), 0600); err != nil {
						t.Fatal(err)
					}
					planted = true
				}
			}
			return out
		})
		if mode == "old-setup" && omitted != 3 {
			t.Fatal("old-setup narrowing plant not applied")
		}
		if planted != (mode != "quiescent") {
			t.Fatal("maintenance actor did not reach the intended state")
		}
		before := decoyImage(t, root)
		if planted {
			if err := os.Remove(filepath.Join(root, ".git", "objects", "maintenance.lock")); err != nil {
				t.Fatal(err)
			}
		}
		requireDecoyImage(t, before, decoyImage(t, root))
		return
	}
	for _, row := range []struct {
		mode string
		exit int
	}{{"quiescent", 0}, {"old-setup", 1}, {"forced-overlap", 1}} {
		t.Run(row.mode, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestDecoyQuiescentBeforeSnapshot$", "-test.v", "-test.parallel=2")
			command.Env = append(os.Environ(), childMode+"="+row.mode)
			out, err := command.CombinedOutput()
			code := 0
			if err != nil {
				exit, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if code != row.exit {
				t.Fatalf("maintenance witness exit %d, want %d:\n%s", code, row.exit, out)
			}
			if row.exit == 1 && (!strings.Contains(string(out), "decoy entry count changed") || !strings.Contains(string(out), "decoy changed:") || !strings.Contains(string(out), "maintenance.lock")) {
				t.Fatalf("overlap did not fail the complete lock preimage assertion:\n%s", out)
			}
			t.Logf("%s: child exit %d; complete-image witness checked", row.mode, code)
		})
	}
}
