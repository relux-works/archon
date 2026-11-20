package ciworkflow

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type pullRequestStep struct {
	Name string `yaml:"name"`
}

type pullRequestTestJob struct {
	Needs []string          `yaml:"needs"`
	Steps []pullRequestStep `yaml:"steps"`
}

type pullRequestWorkflow struct {
	Jobs map[string]pullRequestTestJob `yaml:"jobs"`
}

func checkPullRequestDefersPostLandingTests(data []byte) error {
	var workflow pullRequestWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return err
	}
	if _, ok := workflow.Jobs["curator-mutation-pr"]; ok {
		return fmt.Errorf("deferred mutation job is wired into pull-request CI")
	}
	job, ok := workflow.Jobs["test"]
	if !ok {
		return fmt.Errorf("test job is missing")
	}
	wantNeeds := []string{"windows-test-shards", "unix-test-shards", "store-v1-compatibility"}
	if !reflect.DeepEqual(job.Needs, wantNeeds) {
		return fmt.Errorf("test job has unexpected dependencies: %v", job.Needs)
	}
	for _, step := range job.Steps {
		if step.Name == "Require PR mutation gate success" {
			return fmt.Errorf("test job still requires a PR mutation gate")
		}
	}
	return nil
}

// TestPullRequestDefersPostLandingTests keeps the slow mutation and exhaustive
// set out of the PR package gate. Its controls reject restoring a PR job,
// dependency, or result bridge; it does not establish main-push execution.
func TestPullRequestDefersPostLandingTests(t *testing.T) {
	good := workflowBytes(t)
	if err := checkPullRequestDefersPostLandingTests(good); err != nil {
		t.Fatalf("valid PR deferral rejected: %v", err)
	}
	for name, pair := range map[string][2]string{
		"restored_pr_job": {
			"  default-build-transport:",
			"  curator-mutation-pr:\n    runs-on: macos-latest\n  default-build-transport:",
		},
		"restored_pr_dependency": {
			"needs: [windows-test-shards, unix-test-shards, store-v1-compatibility]",
			"needs: [windows-test-shards, unix-test-shards, store-v1-compatibility, curator-mutation-pr]",
		},
		"restored_pr_result_bridge": {
			"      - name: Checkout authoritative protocol suite",
			"      - name: Require PR mutation gate success\n        run: exit 1\n      - name: Checkout authoritative protocol suite",
		},
	} {
		t.Run(name, func(t *testing.T) {
			mutant := strings.Replace(string(good), pair[0], pair[1], 1)
			if mutant == string(good) {
				t.Fatal("PR deferral plant not applied")
			}
			if err := checkPullRequestDefersPostLandingTests([]byte(mutant)); err == nil {
				t.Fatal("pull-request deferred-suite violation admitted")
			}
		})
	}
}
