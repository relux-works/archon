// Package harnessprobe owns the only process effect in the staged harness seam.
// A complete registry tuple is checked before resolution or command creation.
package harnessprobe

import (
	"bytes"
	"context"
	"os/exec"
	"slices"
	"time"

	"github.com/relux-works/archon/internal/harnessapi"
	"github.com/relux-works/archon/internal/harnessregistry"
	"github.com/relux-works/archon/internal/harnessvalue"
)

// pipeCleanupDelayV1 is fixed independently of untrusted probe requests.
const pipeCleanupDelayV1 = 100 * time.Millisecond

// Validate refuses any tuple differing from the exact static probe policy.
func Validate(id harnessapi.ID, spec harnessapi.ProbeSpec) error {
	policy, err := harnessregistry.ProbePolicy(id)
	if err != nil {
		return harnessapi.NewRefusal(harnessapi.ProbeSpecUnsupported)
	}
	if !spec.ValidValue() || spec.ExecutableAlias() != policy.ExecutableAlias() || !slices.Equal(spec.Arguments(), policy.Arguments()) || spec.Timeout() != policy.Timeout() || spec.Timeout() < time.Second || spec.Timeout() > harnessapi.ProbeTimeoutV1 || len(spec.Environment()) != 0 {
		return harnessapi.NewRefusal(harnessapi.ProbeSpecUnsupported)
	}
	return nil
}

// Run executes the validated bare-alias policy. Cmd.Env deliberately stays nil:
// native release discovery inherits the caller's environment unchanged.
func Run(ctx context.Context, id harnessapi.ID, spec harnessapi.ProbeSpec) (harnessapi.ProbeOutput, error) {
	return run(ctx, id, spec, exec.LookPath, func(cmd *exec.Cmd) error { return cmd.Run() })
}

func run(ctx context.Context, id harnessapi.ID, spec harnessapi.ProbeSpec, resolve func(string) (string, error), launch func(*exec.Cmd) error) (harnessapi.ProbeOutput, error) {
	if err := Validate(id, spec); err != nil {
		return harnessapi.ProbeOutput{}, err
	}
	executable, err := resolve(spec.ExecutableAlias())
	if err != nil {
		return harnessapi.ProbeOutput{}, harnessapi.NewRefusal(harnessapi.RuntimeMissing)
	}
	bounded, cancel := context.WithTimeout(ctx, spec.Timeout())
	defer cancel()
	cmd := exec.CommandContext(bounded, executable, spec.Arguments()...) // #nosec G204 -- exact static tuple validated before policy-alias resolution.
	cmd.WaitDelay = pipeCleanupDelayV1
	var stdout, stderr boundedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Do not assign cmd.Env, including an empty non-nil environment.
	err = launch(cmd)
	if stdout.overflow || stderr.overflow {
		return harnessapi.ProbeOutput{}, harnessapi.NewRefusal("harness_probe_output_too_large")
	}
	if bounded.Err() != nil {
		return harnessapi.ProbeOutput{}, harnessapi.NewRefusal("harness_probe_timeout")
	}
	if err != nil {
		// This includes nonzero exits and ErrWaitDelay after a zero exit. No
		// partial output or wait failure can become a successful observation.
		return harnessapi.ProbeOutput{}, harnessapi.NewRefusal("harness_probe_failed")
	}
	return harnessapi.NewProbeOutput(stdout.Bytes(), stderr.Bytes(), 0)
}

type boundedBuffer struct {
	bytes.Buffer
	overflow bool
}

// Write retains only the bounded prefix while consuming the entire stream.
func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > harnessvalue.MaxTextBytes {
		remaining := harnessvalue.MaxTextBytes - b.Len()
		if remaining > 0 {
			_, _ = b.Buffer.Write(p[:remaining])
		}
		b.overflow = true
		// Consume the entire stream without retaining an unbounded suffix.
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
