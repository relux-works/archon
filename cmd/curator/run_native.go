package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/relux-works/archon-pantheon/pkg/agentic"
	"github.com/relux-works/archon/internal/config"
	"github.com/relux-works/archon/internal/envprofile"
	"github.com/relux-works/archon/internal/envregistry"
)

// cmdNativeRun resolves the target environment in-process and launches the
// agent directly using the launch fragment and curator-runtimes when no
// external curator-run provider binary is installed.
func (c cli) cmdNativeRun(cfg *config.Config, home string, args []string) int {
	var (
		envID      string
		profile    string
		repair     bool
		nativeArgs []string
	)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			nativeArgs = append(nativeArgs, args[i+1:]...)
			break
		}
		if envID == "" {
			switch {
			case arg == "--profile" && i+1 < len(args):
				i++
				profile = args[i]
				continue
			case strings.HasPrefix(arg, "--profile="):
				profile = strings.TrimPrefix(arg, "--profile=")
				continue
			case arg == "--repair":
				repair = true
				continue
			case strings.HasPrefix(arg, "-"):
				_, _ = fmt.Fprintf(c.stderr, "curator: run: unknown flag %s before environment operand\n", arg)
				return exitUsage
			default:
				envID = envregistry.NormalizeEnvID(arg)
				continue
			}
		}
		switch {
		case arg == "--profile" && i+1 < len(args):
			i++
			profile = args[i]
		case strings.HasPrefix(arg, "--profile="):
			profile = strings.TrimPrefix(arg, "--profile=")
		case arg == "--repair":
			repair = true
		default:
			nativeArgs = append(nativeArgs, arg)
		}
	}
	if envID == "" {
		_, _ = fmt.Fprintln(c.stderr, "curator: run <env-id> [--profile <name>] [--repair] [--] [args...]")
		return exitUsage
	}
	adapter, err := envregistry.ByID(envID)
	if err != nil {
		_, _ = fmt.Fprintln(c.stderr, "curator:", err)
		return exitFail
	}
	if _, err := agentic.NormalizeSystemID(agentic.CanonicalSystemID(envID)); err != nil {
		_, _ = fmt.Fprintln(c.stderr, "curator:", err)
		return exitFail
	}
	if profile == "" {
		if scoped, err := envprofile.ScopedCurrents(home); err == nil {
			if p := scoped[envID]; p != "" {
				profile = p
			}
		}
		if profile == "" {
			if cur, err := envprofile.Current(home); err == nil && cur != "" {
				profile = cur
			}
		}
	}
	policy := envprofile.Policy{}
	machine := envregistry.DefaultMachineConfig()
	if cfg != nil {
		policy = envprofile.PolicyFromConfig(cfg)
		machine = machineFromConfig(cfg)
	}
	cwd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintln(c.stderr, "curator:", err)
		return exitFail
	}
	req := envprofile.ResolveRequest{
		Home:      home,
		Profile:   profile,
		EnvID:     envID,
		LaunchDir: cwd,
		Machine:   machine,
		Repair:    repair,
		Policy:    policy,
	}
	res, err := envprofile.Resolve(req)
	if err != nil && !repair && strings.Contains(err.Error(), envprofile.DiagHomeStale) {
		req.Repair = true
		res, err = envprofile.Resolve(req)
	}
	if err != nil {
		_, _ = fmt.Fprintln(c.stderr, "curator:", err)
		return exitFail
	}
	for _, warning := range res.Warnings {
		_, _ = fmt.Fprintln(c.stderr, "warning:", warning)
	}
	frag, err := agentic.DecodeCuratorFragment(res.Document)
	if err != nil {
		_, _ = fmt.Fprintln(c.stderr, "curator:", err)
		return exitFail
	}
	if len(adapter.Probe) == 0 || adapter.Probe[0] == "" {
		_, _ = fmt.Fprintf(c.stderr, "curator: adapter %s has no registered runtime executable\n", adapter.ID)
		return exitFail
	}
	exePath, err := exec.LookPath(adapter.Probe[0])
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "curator: %s: adapter %s requires runtime %s\n", envprofile.DiagAdapterRuntimeMissing, adapter.ID, adapter.Probe[0])
		return exitFail
	}
	var childArgs []string
	if frag != nil {
		if frag.Home != nil && frag.Home.Transport == agentic.CuratorHomeTransportFlag && frag.Home.Path != "" {
			childArgs = append(childArgs, "--user-data-dir", frag.Home.Path)
		}
		if frag.SystemPrompt != nil && frag.SystemPrompt.Path != "" {
			for _, ch := range frag.SystemPrompt.Channels {
				if ch.Kind == agentic.CuratorDescriptorFlag && ch.Flag != "" {
					childArgs = append(childArgs, ch.Flag, frag.SystemPrompt.Path)
					break
				}
			}
		}
		if frag.MCP != nil && frag.MCP.Path != "" {
			for _, ch := range frag.MCP.Channels {
				if ch.Kind == agentic.CuratorDescriptorFlag && ch.Flag != "" {
					childArgs = append(childArgs, ch.Flag, frag.MCP.Path)
					break
				}
			}
		}
	}
	childArgs = append(childArgs, nativeArgs...)
	childEnv := mergeFragmentEnv(c.childEnviron(), frag)
	command := exec.Command(exePath, childArgs...) // #nosec G204 -- resolved runtime binary with validated fragment and operator args
	command.Env = childEnv
	command.Stdin = os.Stdin
	command.Stdout = c.stdout
	command.Stderr = c.stderr
	if err := runUmbrellaProvider(command); err != nil {
		if exit, ok := err.(interface{ ExitCode() int }); ok {
			return exit.ExitCode()
		}
		_, _ = fmt.Fprintln(c.stderr, "curator:", err)
		return exitFail
	}
	return exitOK
}

func mergeFragmentEnv(base []string, frag *agentic.CuratorContext) []string {
	if frag == nil || len(frag.Env) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(frag.Env))
	overridden := make(map[string]bool, len(frag.Env))
	for k := range frag.Env {
		overridden[k] = true
	}
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if overridden[key] {
			continue
		}
		out = append(out, entry)
	}
	for k, v := range frag.Env {
		out = append(out, k+"="+v)
	}
	return out
}
