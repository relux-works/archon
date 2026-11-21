package harnessprobe

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/relux-works/curator/internal/harnessapi"
	"github.com/relux-works/curator/internal/harnessregistry"
	"github.com/relux-works/curator/internal/harnessvalue"
)

func TestMain(m *testing.M) {
	runProbeContractHelper()
	if os.Getenv("HARNESS_PROBE_TEST_HELPER") == "sentinel-v1" {
		// The only child executable is this inert test binary. No environment
		// values are printed, including on the helper's negative path.
		if len(os.Args) != 2 || os.Args[1] != "--version" || os.Getenv("HARNESS_PROBE_SENTINEL") == "" || os.Getenv("HARNESS_PROBE_SENTINEL") != os.Getenv("HARNESS_PROBE_EXPECTED") {
			os.Exit(72)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func refusalCode(err error) string {
	var refusal harnessapi.Refusal
	if errors.As(err, &refusal) {
		return refusal.Code()
	}
	return ""
}

// All five real staged ReleaseProbe methods traverse the same validator,
// resolver and launcher. The policy set, argv, timeout and nil environment are
// asserted against independent baseline rows, with exactly one launch each.
func TestRegisteredProbePolicies(t *testing.T) {
	expected := map[harnessapi.ID]string{"muse": "muse", "claude_code": "claude", "codex_cli": "codex", "opencode": "opencode", "pi": "pi"}
	observed := map[harnessapi.ID]bool{}
	for _, id := range harnessregistry.IDs() {
		adapter, err := harnessregistry.Lookup(id)
		if err != nil {
			t.Fatal(err)
		}
		spec, err := adapter.ReleaseProbe(harnessapi.Context{})
		alias, allowed := expected[id]
		if !allowed {
			if refusalCode(err) != harnessapi.CapabilityUnsupported {
				t.Fatalf("partial probe refusal: %s %v", id, err)
			}
			if _, err := harnessregistry.ProbePolicy(id); refusalCode(err) != harnessapi.CapabilityUnsupported {
				t.Fatalf("partial policy: %s %v", id, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		observed[id] = true
		resolutions, launches := 0, 0
		_, err = run(context.Background(), id, spec, func(got string) (string, error) {
			resolutions++
			if got != alias {
				t.Fatal("wrong alias")
			}
			return "/inert/fixture", nil
		}, func(cmd *exec.Cmd) error {
			launches++
			if cmd.Path != "/inert/fixture" || !reflect.DeepEqual(cmd.Args, []string{"/inert/fixture", "--version"}) || cmd.Env != nil || cmd.WaitDelay != 100*time.Millisecond || spec.Timeout() != 5*time.Second || len(spec.Environment()) != 0 {
				t.Fatalf("invalid command tuple for %s", id)
			}
			return nil
		})
		if err != nil || resolutions != 1 || launches != 1 {
			t.Fatalf("row %s: %v resolve=%d launch=%d", id, err, resolutions, launches)
		}
	}
	if len(observed) != len(expected) {
		t.Fatalf("policy coverage=%v", observed)
	}
}

// Nil Cmd.Env passes a unique caller sentinel unchanged to an inert helper.
// The helper inspects that value and exact argv without printing either env.
func TestProbeEnvironmentPassThrough(t *testing.T) {
	t.Run("pipe-contract", testProbePipeContract)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Base(t.TempDir()) + "-unchanged"
	t.Setenv("HARNESS_PROBE_TEST_HELPER", "sentinel-v1")
	t.Setenv("HARNESS_PROBE_SENTINEL", sentinel)
	t.Setenv("HARNESS_PROBE_EXPECTED", sentinel)
	adapter, err := harnessregistry.Lookup("claude_code")
	if err != nil {
		t.Fatal(err)
	}
	spec, err := adapter.ReleaseProbe(harnessapi.Context{})
	if err != nil {
		t.Fatal(err)
	}
	launches := 0
	output, err := run(context.Background(), adapter.ID(), spec, func(string) (string, error) { return executable, nil }, func(cmd *exec.Cmd) error {
		launches++
		if cmd.Env != nil {
			t.Fatal("caller environment replaced")
		}
		return cmd.Run()
	})
	if err != nil || output.ExitCode() != 0 || launches != 1 {
		t.Fatalf("sentinel helper: %v code=%d calls=%d", err, output.ExitCode(), launches)
	}
	// An unchanged runner cannot conceal a changed child value: the helper must
	// return its intended refusal code when the independent expectation differs.
	t.Setenv("HARNESS_PROBE_EXPECTED", sentinel+"-different")
	observedExit := 0
	output, err = run(context.Background(), adapter.ID(), spec, func(string) (string, error) { return executable, nil }, func(cmd *exec.Cmd) error {
		err := cmd.Run()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			observedExit = exit.ExitCode()
		}
		return err
	})
	if refusalCode(err) != "harness_probe_failed" || observedExit != 72 || len(output.Stdout()) != 0 || len(output.Stderr()) != 0 {
		t.Fatalf("sentinel negative control: %v observed exit=%d", err, observedExit)
	}
}

// Every rejected dimension returns the exact refusal before resolver/launcher
// counters advance. Nearby valid controls run through the identical path.
func TestProbeSpecRefusal(t *testing.T) {
	type row struct {
		name    string
		id      harnessapi.ID
		alias   string
		args    []string
		timeout time.Duration
		env     map[string]string
	}
	rows := []row{
		{"unknown-alias", "claude_code", "unknown", []string{"--version"}, 5 * time.Second, nil},
		{"borrowed-alias", "claude_code", "codex", []string{"--version"}, 5 * time.Second, nil},
		{"absolute-path", "claude_code", "/fixture/claude", []string{"--version"}, 5 * time.Second, nil},
		{"relative-path", "claude_code", "fixture/claude", []string{"--version"}, 5 * time.Second, nil},
		{"shell", "claude_code", "sh", []string{"-c", "fixture"}, 5 * time.Second, nil},
		{"wrapper", "claude_code", "env", []string{"claude", "--version"}, 5 * time.Second, nil},
		{"command-argument", "claude_code", "claude", []string{"-c", "fixture"}, 5 * time.Second, nil},
		{"extra-argument", "claude_code", "claude", []string{"--version", "--extra"}, 5 * time.Second, nil},
		{"missing-argument", "claude_code", "claude", nil, 5 * time.Second, nil},
		{"zero-timeout", "claude_code", "claude", []string{"--version"}, 0, nil},
		{"negative-timeout", "claude_code", "claude", []string{"--version"}, -time.Second, nil},
		{"over-bound-timeout", "claude_code", "claude", []string{"--version"}, 6 * time.Second, nil},
		{"in-bound-but-not-row", "claude_code", "claude", []string{"--version"}, 4 * time.Second, nil},
		{"unknown-id", "unknown", "claude", []string{"--version"}, 5 * time.Second, nil},
		{"partial-id", "cursor", "claude", []string{"--version"}, 5 * time.Second, nil},
	}
	for _, key := range []string{"PATH", "HOME", "LD_PRELOAD", "arbitrary", "KIMI_CODE_HOME"} {
		rows = append(rows, row{"environment-" + key, "claude_code", "claude", []string{"--version"}, 5 * time.Second, map[string]string{key: "fixture"}})
	}
	rows = append(rows, row{"empty-environment-value", "claude_code", "claude", []string{"--version"}, 5 * time.Second, map[string]string{"arbitrary": ""}})
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			resolve, launch := 0, 0
			_, err := run(context.Background(), r.id, harnessapi.NewProbeSpec(r.alias, r.args, r.timeout, r.env), func(string) (string, error) { resolve++; return "fixture", nil }, func(*exec.Cmd) error { launch++; return nil })
			if refusalCode(err) != harnessapi.ProbeSpecUnsupported || resolve != 0 || launch != 0 {
				t.Fatalf("refusal=%v resolve=%d launch=%d", err, resolve, launch)
			}
		})
	}
	valid, err := harnessregistry.ProbePolicy("claude_code")
	if err != nil {
		t.Fatal(err)
	}
	resolve, launch := 0, 0
	_, err = run(context.Background(), "claude_code", valid, func(string) (string, error) { resolve++; return "fixture", nil }, func(*exec.Cmd) error { launch++; return nil })
	if err != nil || resolve != 1 || launch != 1 {
		t.Fatalf("positive control: %v %d %d", err, resolve, launch)
	}
	_, err = run(context.Background(), "claude_code", valid, func(string) (string, error) { return "", exec.ErrNotFound }, func(*exec.Cmd) error { t.Fatal("missing runtime launched"); return nil })
	if refusalCode(err) != harnessapi.RuntimeMissing {
		t.Fatalf("missing runtime: %v", err)
	}
}

// The runner retains at most the documented output bound and explicitly refuses
// overflow; failed/cancelled launches never become successful release output.
func TestProbeOutputBoundAndFailures(t *testing.T) {
	spec, err := harnessregistry.ProbePolicy("muse")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{harnessvalue.MaxTextBytes, harnessvalue.MaxTextBytes + 1} {
		output, err := run(context.Background(), "muse", spec, func(string) (string, error) { return "fixture", nil }, func(cmd *exec.Cmd) error { _, err := cmd.Stdout.Write(make([]byte, size)); return err })
		if size == harnessvalue.MaxTextBytes {
			if err != nil || len(output.Stdout()) != size {
				t.Fatal("exact bound refused")
			}
		} else if refusalCode(err) != "harness_probe_output_too_large" || len(output.Stdout()) != 0 || len(output.Stderr()) != 0 {
			t.Fatalf("overflow: %v", err)
		}
	}
	_, err = run(context.Background(), "muse", spec, func(string) (string, error) { return "fixture", nil }, func(*exec.Cmd) error { return errors.New("inert launch failure") })
	if refusalCode(err) != "harness_probe_failed" {
		t.Fatalf("launch failure: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = run(ctx, "muse", spec, func(string) (string, error) { return "fixture", nil }, func(*exec.Cmd) error { return context.Canceled })
	if refusalCode(err) != "harness_probe_timeout" {
		t.Fatalf("cancelled: %v", err)
	}
}
