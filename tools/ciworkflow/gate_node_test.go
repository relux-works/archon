package ciworkflow

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func checkGateSelftestNode(data []byte) error {
	var wf workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return err
	}
	ready, setups, gates := false, 0, 0
	for _, s := range wf.Jobs["gate-selftest"].Steps {
		if s.Uses == "actions/setup-node@v4" {
			if s.With["node-version"] != "22" || s.If != "" || s.Continue {
				return fmt.Errorf("gate-selftest requires unconditional Node 22 provisioning")
			}
			ready = true
			setups++
		}
		if s.Run == "bash .github/ci/gate-selftest.sh" {
			if !ready {
				return fmt.Errorf("gate-selftest must provision Node before shell environment framing")
			}
			gates++
		}
	}
	if setups != 1 || gates != 1 {
		return fmt.Errorf("gate-selftest requires one Node setup and one gate invocation")
	}
	return nil
}

// A self-hosted runner need not have Node on PATH: the native gate must provision
// Node 22 before framing. Missing, conditional, soft-failing and late setup are
// rejected; these workflow controls do not qualify the installed Node runtime.
func TestGateSelftestNodeProvisioning(t *testing.T) {
	data := workflowBytes(t)
	if err := checkGateSelftestNode(data); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing", "wrong-version", "conditional", "soft-failure", "after-gate"} {
		t.Run(name, func(t *testing.T) {
			var wf workflow
			if err := yaml.Unmarshal(data, &wf); err != nil {
				t.Fatal(err)
			}
			job := wf.Jobs["gate-selftest"]
			index := -1
			for i, s := range job.Steps {
				if s.Uses == "actions/setup-node@v4" {
					index = i
					break
				}
			}
			if index < 0 {
				t.Fatal("Node provisioning plant not applied")
			}
			s := job.Steps[index]
			switch name {
			case "missing":
				job.Steps = append(job.Steps[:index], job.Steps[index+1:]...)
			case "wrong-version":
				job.Steps[index].With["node-version"] = "20"
			case "conditional":
				job.Steps[index].If = "runner.os != 'Linux'"
			case "soft-failure":
				job.Steps[index].Continue = true
			case "after-gate":
				job.Steps = append(job.Steps[:index], job.Steps[index+1:]...)
				job.Steps = append(job.Steps, s)
			}
			wf.Jobs["gate-selftest"] = job
			mutant, err := yaml.Marshal(wf)
			if err != nil {
				t.Fatal(err)
			}
			want := "requires unconditional Node 22 provisioning"
			if name == "missing" || name == "after-gate" {
				want = "must provision Node before shell environment framing"
			}
			if err := checkGateSelftestNode(mutant); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("want Node provisioning refusal %q, got %v", want, err)
			}
		})
	}
}
