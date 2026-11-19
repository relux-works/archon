package ciworkflow

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type prMutationGateStep struct {
	Name            string            `yaml:"name"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	If              string            `yaml:"if"`
	Shell           string            `yaml:"shell"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	Env             map[string]string `yaml:"env"`
}

type prMutationGateJob struct {
	If    string               `yaml:"if"`
	Needs []string             `yaml:"needs"`
	Steps []prMutationGateStep `yaml:"steps"`
}

type prMutationGateWorkflow struct {
	Jobs map[string]prMutationGateJob `yaml:"jobs"`
}

func checkPRMutationGate(data []byte) error {
	var wf prMutationGateWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return err
	}
	job, ok := wf.Jobs["test"]
	if !ok {
		return fmt.Errorf("Test job missing")
	}
	wantNeeds := []string{"windows-test-shards", "unix-test-shards", "store-v1-compatibility", "curator-mutation-pr"}
	if job.If != "${{ !cancelled() }}" || !reflect.DeepEqual(job.Needs, wantNeeds) {
		return fmt.Errorf("Test must admit shard failures and depend on the PR mutation gate")
	}

	checkout, initialize, gate, setupGo := -1, -1, -1, -1
	var gateStep prMutationGateStep
	for i, s := range job.Steps {
		if checkout < 0 && s.Uses == "actions/checkout@v4" {
			checkout = i
		}
		if initialize < 0 && s.Run == isolationCommand && s.If == "runner.os == 'Linux'" {
			initialize = i
		}
		if gate < 0 && s.Name == "Require PR mutation gate success" {
			gate = i
			gateStep = s
		}
		if setupGo < 0 && strings.HasPrefix(s.Uses, "actions/setup-go@") {
			setupGo = i
		}
	}
	if checkout < 0 || initialize < 0 || gate < 0 || setupGo < 0 || checkout >= initialize || initialize >= gate || gate >= setupGo {
		return fmt.Errorf("PR mutation result gate must follow checkout and private Go initialization, and precede Go consumers")
	}
	wantRun := `if [[ "$GITHUB_EVENT_NAME" == "pull_request" ]]; then
  test "$CURATOR_MUTATION_RESULT" = success
fi`
	if gateStep.If != "always()" || gateStep.Shell != "bash" || gateStep.ContinueOnError ||
		gateStep.Env["CURATOR_MUTATION_RESULT"] != "${{ needs.curator-mutation-pr.result }}" ||
		strings.TrimSpace(gateStep.Run) != wantRun {
		return fmt.Errorf("PR mutation result gate must require success on pull requests")
	}
	return nil
}

func rewritePRMutationGate(t *testing.T, data []byte, mutate func(*prMutationGateJob, int)) []byte {
	t.Helper()
	var wf prMutationGateWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	job, ok := wf.Jobs["test"]
	if !ok {
		t.Fatal("Test job missing")
	}
	gate := -1
	for i, s := range job.Steps {
		if s.Name == "Require PR mutation gate success" {
			if gate >= 0 {
				t.Fatal("mutation gate plant target is duplicated")
			}
			gate = i
		}
	}
	if gate < 0 {
		t.Fatal("mutation gate plant target missing")
	}
	mutate(&job, gate)
	wf.Jobs["test"] = job
	mutant, err := yaml.Marshal(wf)
	if err != nil {
		t.Fatal(err)
	}
	return mutant
}

// TestPRMutationGateOrderAndDependency checks the gate's dependency and order contract.
// Its negative control restores the former pre-initialization order.
func TestPRMutationGateOrderAndDependency(t *testing.T) {
	data := workflowBytes(t)
	t.Run("positive_dependency_and_order", func(t *testing.T) {
		if err := checkPRMutationGate(data); err != nil {
			t.Fatalf("valid mutation gate control: %v", err)
		}
	})
	t.Run("negative_legacy_pre_initialization_order", func(t *testing.T) {
		mutant := rewritePRMutationGate(t, data, func(job *prMutationGateJob, gate int) {
			gateStep := job.Steps[gate]
			job.Steps = append(job.Steps[:gate], job.Steps[gate+1:]...)
			job.Steps = append([]prMutationGateStep{gateStep}, job.Steps...)
		})
		if err := checkPRMutationGate(mutant); err == nil || !strings.Contains(err.Error(), "must follow checkout and private Go initialization") {
			t.Fatalf("want pre-initialization gate refusal, got %v", err)
		}
	})
}

// TestPRMutationGateRejectsBypasses verifies the command cannot ignore a failed mutation job. It checks the workflow structure, not hosted scheduling.
func TestPRMutationGateRejectsBypasses(t *testing.T) {
	data := workflowBytes(t)
	for name, mutate := range map[string]func(*prMutationGateStep){
		"weakened_result_command": func(step *prMutationGateStep) { step.Run += "true\n" },
		"continue_on_error":       func(step *prMutationGateStep) { step.ContinueOnError = true },
	} {
		t.Run(name, func(t *testing.T) {
			mutant := rewritePRMutationGate(t, data, func(job *prMutationGateJob, gate int) {
				mutate(&job.Steps[gate])
			})
			if err := checkPRMutationGate(mutant); err == nil {
				t.Fatal("mutation gate bypass admitted")
			}
		})
	}
}
