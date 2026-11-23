//go:build harness_parity

package harnessparity

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Refusal and comparator controls run in child lanes. The enclosing parity
// test provides the positive 47-case execution under the admitted policy.
func testCreationPolicyControls(t *testing.T) {
	if os.Getenv("HARNESS_PARITY_CONTROL_CHILD") == "1" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		name, mask, mutant, run string
		exit                    int
	}{
		{"022-environment-refusal", "0022", "", "^$", creationFailureExit},
		{"0644-golden-comparator-refusal", "0077", "0644", "^TestHomeByteIdentity$", 1},
	}
	if runtime.GOOS == "windows" {
		rows = []struct {
			name, mask, mutant, run string
			exit                    int
		}{
			{"windows-mode-declaration-refusal", "0077", "", "^$", creationFailureExit},
		}
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.v", "-test.run", row.run)
			cmd.Env = append(os.Environ(), "HARNESS_PARITY_CONTROL_CHILD=1", "HARNESS_PARITY_POLICY_CONTROL="+row.mask, "HARNESS_PARITY_GOLDEN_MODE_CONTROL="+row.mutant)
			output, runErr := cmd.CombinedOutput()
			exit := 0
			if runErr != nil {
				failure, ok := runErr.(*exec.ExitError)
				if !ok {
					t.Fatal("creation control did not execute")
				}
				exit = failure.ExitCode()
			}
			receipt, err := parseCreationReceipt(output)
			if err != nil || exit != row.exit {
				t.Fatalf("creation row=%s exit=%d want=%d receipt_error=%v", row.name, exit, row.exit, err)
			}
			if runtime.GOOS == "windows" && row.mask == "0077" {
				if receipt.Code != creationFailure || receipt.Executed != 0 || receipt.Invocations != 0 || receipt.EffectiveUmask != "not-applicable" {
					t.Fatal("Windows accepted a POSIX creation mode declaration")
				}
			} else if row.mask == "0022" {
				if receipt.Code != creationFailure || receipt.Executed != 0 || receipt.Invocations != 0 || receipt.EffectiveUmask != "0022" {
					t.Fatal("022 did not fail before legacy operations")
				}
			} else {
				wantUmask := "0077"
				if runtime.GOOS == "windows" {
					wantUmask = "not-applicable"
				}
				if receipt.Code != "" || receipt.Executed != expectedBaselineCases || receipt.Expected != expectedBaselineCases || receipt.EffectiveUmask != wantUmask || receipt.Invocations < expectedBaselineCases {
					t.Fatal("077 control lacked complete measured execution")
				}
				if row.mutant != "" && (!receipt.MutantApplied || !strings.Contains(string(output), ".cursor/rules/.csk-managed.json: kind/mode/link mismatch") || !strings.Contains(string(output), "one in-memory 0644 golden-mode control applied")) {
					t.Fatal("golden-mode control had an unrelated failure")
				}
			}
			t.Logf("control=%s exit=%d baseline_cases=%d/%d invocations=%d effective_umask=%s", row.name, exit, receipt.Executed, receipt.Expected, receipt.Invocations, receipt.EffectiveUmask)
		})
	}
}
