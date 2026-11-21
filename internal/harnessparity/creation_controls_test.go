//go:build harness_parity

package harnessparity

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Each row runs the same dedicated lane. Environment refusal must occur before
// legacy calls; the valid 077 lane executes all 47 rows; the 0644 expected-mode
// mutant must reach and fail the real comparator, not setup or provenance.
func testCreationPolicyControls(t *testing.T) {
	if os.Getenv("HARNESS_PARITY_CONTROL_CHILD") == "1" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name, mask, mutant string
		exit               int
	}{
		{"022-environment-refusal", "0022", "", creationFailureExit},
		{"077-complete-pass", "0077", "", 0},
		{"0644-golden-comparator-refusal", "0077", "0644", 1},
	} {
		t.Run(row.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.v", "-test.run", "^(TestHomeByteIdentity|TestHomeByteIdentityMutationControl|TestAllAdaptersHaveBaselineCases)$")
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
			if row.mask == "0022" {
				if receipt.Code != creationFailure || receipt.Executed != 0 || receipt.Invocations != 0 || receipt.EffectiveUmask != "0022" {
					t.Fatal("022 did not fail before legacy operations")
				}
			} else {
				if receipt.Code != "" || receipt.Executed != expectedBaselineCases || receipt.Expected != expectedBaselineCases || receipt.EffectiveUmask != "0077" || receipt.Invocations < expectedBaselineCases {
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
