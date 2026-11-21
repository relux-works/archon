package harnessprobe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/relux-works/curator/internal/harnessapi"
	"github.com/relux-works/curator/internal/harnessregistry"
)

// These modes are reachable only in this test executable. Children carry no
// provider/auth channels; their inherited standard streams are deliberate data.
func runProbeContractHelper() {
	mode := os.Getenv("HARNESS_PROBE_TEST_HELPER")
	if mode != "pipe-parent-v1" && mode != "pipe-child-v1" && mode != "clean-output-v1" && mode != "sleep-v1" {
		return
	}
	if len(os.Args) != 2 || os.Args[1] != "--version" {
		os.Exit(73)
	}
	switch mode {
	case "clean-output-v1":
		if _, err := fmt.Fprint(os.Stdout, "fixture 1.0.0\n"); err != nil {
			os.Exit(76)
		}
		if _, err := fmt.Fprint(os.Stderr, "fixture-note\n"); err != nil {
			os.Exit(76)
		}
		os.Exit(0)
	case "sleep-v1":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "pipe-child-v1":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "pipe-parent-v1":
		child := exec.Command(os.Args[0], "--version")
		child.Env = append(os.Environ(), "HARNESS_PROBE_TEST_HELPER=pipe-child-v1")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if child.Start() != nil {
			os.Exit(74)
		}
		path := os.Getenv("HARNESS_PROBE_CHILD_PID")
		if os.WriteFile(path+".tmp", []byte(strconv.Itoa(child.Process.Pid)), 0600) != nil || os.Rename(path+".tmp", path) != nil {
			_ = child.Process.Kill()
			os.Exit(75)
		}
		if _, err := fmt.Fprint(os.Stdout, "partial-version\n"); err != nil {
			os.Exit(76)
		}
		os.Exit(0)
	}
}

type probeResult struct {
	output           harnessapi.ProbeOutput
	err              error
	waitDelayExpired bool
}

// Real inert pipes prove caller cancellation and observed-exit cleanup refuse
// incomplete output. The zero-delay mutation must remain blocked until the
// fixture releases its own holder, then succeed: an unrelated error/skip cannot
// satisfy that instrument control. Timing windows allow OS scheduling; the
// independent command-tuple assertion pins the actual cleanup field to100ms.
func testProbePipeContract(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec, err := harnessregistry.ProbePolicy("claude_code")
	if err != nil {
		t.Fatal(err)
	}
	launch := func(cmd *exec.Cmd) error {
		if cmd.Env != nil || cmd.WaitDelay != 100*time.Millisecond {
			return errors.New("command cleanup/environment contract changed")
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		t.Logf("owned inert probe parent pid=%d", cmd.Process.Pid)
		return cmd.Wait()
	}
	t.Run("clean-output", func(t *testing.T) {
		t.Setenv("HARNESS_PROBE_TEST_HELPER", "clean-output-v1")
		out, err := run(context.Background(), "claude_code", spec, func(string) (string, error) { return executable, nil }, launch)
		if err != nil || out.ExitCode() != 0 || !bytes.Equal(out.Stdout(), []byte("fixture 1.0.0\n")) || !bytes.Equal(out.Stderr(), []byte("fixture-note\n")) {
			t.Fatalf("clean output refused or changed: %v", err)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		t.Setenv("HARNESS_PROBE_TEST_HELPER", "sleep-v1")
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		out, err := run(ctx, "claude_code", spec, func(string) (string, error) { return executable, nil }, launch)
		requireProbeFailure(t, out, err, "harness_probe_timeout")
	})
	for _, row := range []struct {
		name         string
		cancel, zero bool
	}{
		{"cancellation-open-pipe", true, false},
		{"zero-exit-open-pipe", false, false},
		{"known-zero-delay", false, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			root := t.TempDir()
			pidPath := filepath.Join(root, "holder.pid")
			t.Setenv("HARNESS_PROBE_TEST_HELPER", "pipe-parent-v1")
			t.Setenv("HARNESS_PROBE_CHILD_PID", pidPath)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan probeResult, 1)
			go func() {
				var waitErr error
				out, err := run(ctx, "claude_code", spec, func(string) (string, error) { return executable, nil }, func(cmd *exec.Cmd) error {
					if cmd.Env != nil || cmd.WaitDelay != 100*time.Millisecond {
						return errors.New("inert command setup mismatch")
					}
					if row.zero {
						cmd.WaitDelay = 0
					}
					if err := cmd.Start(); err != nil {
						return err
					}
					t.Logf("owned inert probe parent pid=%d", cmd.Process.Pid)
					waitErr = cmd.Wait()
					return waitErr
				})
				done <- probeResult{output: out, err: err, waitDelayExpired: errors.Is(waitErr, exec.ErrWaitDelay)}
			}()
			var holder *os.Process
			t.Cleanup(func() {
				if holder != nil {
					_ = holder.Kill()
					_ = holder.Release()
				}
			})
			ready := time.Now().Add(3 * time.Second)
			for holder == nil && time.Now().Before(ready) {
				payload, readErr := os.ReadFile(pidPath)
				if readErr == nil {
					pid, parseErr := strconv.Atoi(string(payload))
					if parseErr != nil {
						t.Fatal(parseErr)
					}
					holder, err = os.FindProcess(pid)
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("owned inert pipe holder pid=%d", pid)
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if holder == nil {
				cancel()
				t.Fatal("inert pipe holder absent; control did not run")
			}
			if row.cancel {
				cancel()
			}
			select {
			case result := <-done:
				if row.zero {
					t.Fatalf("zero-delay violation was not observed: %v", result.err)
				}
				want := "harness_probe_failed"
				if !row.cancel && !result.waitDelayExpired {
					t.Fatal("zero-exit control did not observe the intended ErrWaitDelay")
				}
				if row.cancel {
					want = "harness_probe_timeout"
				}
				requireProbeFailure(t, result.output, result.err, want)
			case <-time.After(3 * time.Second):
				if !row.zero {
					t.Error("caller blocked beyond scheduling-tolerant cleanup window")
				}
				if row.zero {
					t.Log("known zero-delay bound violation observed before owned-holder release")
				}
				if err := holder.Kill(); err != nil {
					t.Fatal("could not stop own recorded pipe holder")
				}
				select {
				case result := <-done:
					if row.zero && (result.err != nil || result.output.ExitCode() != 0 || !bytes.Equal(result.output.Stdout(), []byte("partial-version\n"))) {
						t.Fatalf("zero-delay instrument had unrelated failure: %v", result.err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("owned runner did not finish after holder cleanup")
				}
			}
		})
	}
}

func requireProbeFailure(t *testing.T, out harnessapi.ProbeOutput, err error, want string) {
	t.Helper()
	if refusalCode(err) != want || len(out.Stdout()) != 0 || len(out.Stderr()) != 0 {
		t.Fatalf("failure contract: err=%v want=%s", err, want)
	}
}
