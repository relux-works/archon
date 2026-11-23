package ciworkflow

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const pinnedNativeWorkflowFile = ".github/workflows/original-capture-native.yml"
const pinnedNativeCaptureTestSources = "testdata/originalcapture"
const pinnedNativeWorkflowSparsePaths = pinnedNativeWorkflowFile + "\n" + pinnedNativeCaptureTestSources

func checkPinnedWorkflowCheckouts(data []byte) error {
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Uses string         `yaml:"uses"`
				With map[string]any `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return err
	}
	expectedJobs := map[string]bool{
		"native-capture-workflow-contract": false,
		"test":                             false,
		"test-self-hosted":                 false,
		"race":                             false,
		"candidate-conformance":            false,
	}
	for jobName, job := range workflow.Jobs {
		for _, step := range job.Steps {
			repository, _ := step.With["repository"].(string)
			if repository != "relux-works/curator" {
				continue
			}
			if step.Uses != "actions/checkout@v4" {
				return fmt.Errorf("%s: pinned workflow must use actions/checkout@v4", jobName)
			}
			if _, expected := expectedJobs[jobName]; !expected {
				return fmt.Errorf("%s: unexpected pinned workflow checkout", jobName)
			}
			if expectedJobs[jobName] {
				return fmt.Errorf("%s: duplicate pinned workflow checkout", jobName)
			}
			expectedJobs[jobName] = true
			path, _ := step.With["path"].(string)
			if path != ".temp/capture-source-44ff" {
				return fmt.Errorf("%s: pinned workflow checkout path changed", jobName)
			}
			sparse, _ := step.With["sparse-checkout"].(string)
			coneMode, coneModeIsBool := step.With["sparse-checkout-cone-mode"].(bool)
			if strings.TrimSpace(sparse) != pinnedNativeWorkflowSparsePaths || !coneModeIsBool || coneMode {
				return fmt.Errorf("%s: pinned checkout must select exactly the workflow file and required capture test sources %q with cone mode disabled", jobName, pinnedNativeCaptureTestSources)
			}
		}
	}
	for jobName, found := range expectedJobs {
		if !found {
			return fmt.Errorf("%s: pinned workflow checkout missing", jobName)
		}
	}
	return nil
}

func TestPinnedWorkflowCheckoutsAreSparse(t *testing.T) {
	good := workflowBytes(t)
	if err := checkPinnedWorkflowCheckouts(good); err != nil {
		t.Fatalf("valid sparse workflow checkouts rejected: %v", err)
	}
	sparseBlock := "sparse-checkout: |\n            " + pinnedNativeWorkflowFile + "\n            " + pinnedNativeCaptureTestSources
	for _, test := range []struct {
		name, old, replacement, wantError string
	}{
		{"full-repository-checkout", sparseBlock, "sparse-checkout: .", ""},
		{"missing-capture-test-sources", sparseBlock, "sparse-checkout: |\n            " + pinnedNativeWorkflowFile, "required capture test sources"},
		{"cone-mode-enabled", "sparse-checkout-cone-mode: false", "sparse-checkout-cone-mode: true", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutant := strings.Replace(string(good), test.old, test.replacement, 1)
			if mutant == string(good) {
				t.Fatal("checkout mutation was not applied")
			}
			if err := checkPinnedWorkflowCheckouts([]byte(mutant)); err == nil {
				t.Fatal("unsafe pinned-workflow checkout was admitted")
			} else if test.wantError != "" && !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("missing-source refusal was not clear: %v", err)
			}
		})
	}
}
