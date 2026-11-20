package ciworkflow

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type pullRequestStep struct {
	Name string    `yaml:"name"`
	If   string    `yaml:"if"`
	Run  string    `yaml:"run"`
	Env  yaml.Node `yaml:"env"`
}

type pullRequestJob struct {
	Name  string            `yaml:"name"`
	If    string            `yaml:"if"`
	Uses  string            `yaml:"uses"`
	Needs yaml.Node         `yaml:"needs"`
	Env   yaml.Node         `yaml:"env"`
	Steps []pullRequestStep `yaml:"steps"`
}

type pullRequestWorkflow struct {
	Jobs map[string]pullRequestJob `yaml:"jobs"`
}

const legacyMutationJobID = "curator-mutation-pr"

var mutationJobResultReference = regexp.MustCompile(`(?i)\bneeds\s*(?:\.\s*([A-Za-z_][A-Za-z0-9_-]*)|\[\s*['"]([^'"]+)['"]\s*\])\s*\.\s*result\b`)
var dynamicNeedsResultReference = regexp.MustCompile(`(?i)\bneeds\s*\[\s*[^'"][^\]]+\]\s*\.\s*result\b`)

func checkPullRequestDefersPostLandingTests(data []byte) error {
	var workflow pullRequestWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return err
	}
	if workflow.Jobs == nil {
		return fmt.Errorf("workflow has no jobs mapping")
	}
	testJob, ok := workflow.Jobs["test"]
	if !ok {
		return fmt.Errorf("test job is missing")
	}
	wantNeeds := []string{"windows-test-shards", "unix-test-shards", "store-v1-compatibility"}
	needs, err := pullRequestNeedIDs(&testJob.Needs)
	if err != nil || testJob.Needs.Kind != yaml.SequenceNode || !reflect.DeepEqual(needs, wantNeeds) {
		return fmt.Errorf("test job has unexpected dependencies: %v", needs)
	}

	mutationJobIDs := map[string]struct{}{legacyMutationJobID: {}}
	for id, job := range workflow.Jobs {
		if isMutationSuiteJob(id, job) {
			mutationJobIDs[strings.ToLower(id)] = struct{}{}
		}
	}
	for id, job := range workflow.Jobs {
		if !canRunOnPullRequest(job.If) {
			continue
		}
		if isMutationSuiteJob(id, job) {
			return fmt.Errorf("mutation suite job %q can run on pull-request CI", id)
		}
		needs, err := pullRequestNeedIDs(&job.Needs)
		if err != nil {
			return fmt.Errorf("job %q has unreadable needs: %w", id, err)
		}
		for _, dependency := range needs {
			if isMutationJobID(dependency, mutationJobIDs) {
				return fmt.Errorf("pull-request job %q depends on deferred mutation job %q", id, dependency)
			}
		}
		if hasMutationResultReference(job.If, mutationJobIDs) || hasMutationResultReferenceInNode(&job.Needs, mutationJobIDs) || hasMutationResultReferenceInNode(&job.Env, mutationJobIDs) {
			return fmt.Errorf("pull-request job %q consumes a deferred mutation result", id)
		}
		for _, step := range job.Steps {
			if hasMutationResultReference(step.If, mutationJobIDs) || hasMutationResultReference(step.Run, mutationJobIDs) || hasMutationResultReferenceInNode(&step.Env, mutationJobIDs) {
				return fmt.Errorf("pull-request job %q consumes a deferred mutation result", id)
			}
		}
	}
	return nil
}

func canRunOnPullRequest(condition string) bool {
	condition = strings.TrimSpace(condition)
	if strings.HasPrefix(condition, "${{") && strings.HasSuffix(condition, "}}") {
		condition = strings.TrimSpace(condition[3 : len(condition)-2])
	}
	condition = strings.Join(strings.Fields(condition), " ")
	switch condition {
	case "vars.ROSE_AIR_RUNNER == 'true' && github.event_name == 'push' && github.ref == 'refs/heads/main'":
		return false
	case "github.event_name == 'workflow_dispatch' && (inputs.candidate_ref != '' || inputs.candidate_root != '')":
		return false
	case "github.event_name == 'workflow_dispatch' && inputs.fixture_isolation_diagnostics":
		return false
	default:
		// Unknown conditions are included so a new or changed condition cannot
		// silently remove a job from the pull-request scan.
		return true
	}
}

func isMutationSuiteJob(id string, job pullRequestJob) bool {
	identity := strings.ToLower(id + " " + job.Name)
	if strings.Contains(identity, legacyMutationJobID) || hasMutationSuiteLabel(identity) || strings.Contains(strings.ToLower(job.Uses), "mutation") {
		return true
	}
	for _, step := range job.Steps {
		run := strings.ToLower(step.Run)
		if hasMutationSuiteLabel(strings.ToLower(step.Name)) || hasMutationSuiteLabel(run) || strings.Contains(run, "tools/mutationinventory") {
			return true
		}
	}
	return false
}

func hasMutationSuiteLabel(identity string) bool {
	if !strings.Contains(identity, "mutation") {
		return false
	}
	return strings.Contains(identity, "suite") || strings.Contains(identity, "control") || strings.Contains(identity, "inventory")
}

func pullRequestNeedIDs(needs *yaml.Node) ([]string, error) {
	if needs == nil || needs.Kind == 0 || needs.Tag == "!!null" {
		return nil, nil
	}
	if needs.Kind == yaml.AliasNode && needs.Alias != nil {
		needs = needs.Alias
	}
	if needs.Kind == yaml.ScalarNode {
		return []string{needs.Value}, nil
	}
	if needs.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("expected scalar or sequence, got %s", needs.ShortTag())
	}
	ids := make([]string, 0, len(needs.Content))
	for _, dependency := range needs.Content {
		if dependency.Kind != yaml.ScalarNode || dependency.Tag != "!!str" {
			return nil, fmt.Errorf("dependency is not a string scalar")
		}
		ids = append(ids, dependency.Value)
	}
	return ids, nil
}

func isMutationJobID(id string, mutationJobIDs map[string]struct{}) bool {
	_, ok := mutationJobIDs[strings.ToLower(id)]
	return ok
}

func hasMutationResultReference(text string, mutationJobIDs map[string]struct{}) bool {
	if dynamicNeedsResultReference.MatchString(text) {
		// The current PR workflow has no dynamic needs.result selectors. Their
		// target cannot be proven non-mutation from the workflow text alone.
		return true
	}
	for _, match := range mutationJobResultReference.FindAllStringSubmatch(text, -1) {
		id := match[1]
		if id == "" {
			id = match[2]
		}
		if isMutationJobID(id, mutationJobIDs) {
			return true
		}
	}
	return false
}

func hasMutationResultReferenceInNode(node *yaml.Node, mutationJobIDs map[string]struct{}) bool {
	for _, value := range workflowScalarValues(node) {
		if hasMutationResultReference(value, mutationJobIDs) {
			return true
		}
	}
	return false
}

func workflowScalarValues(node *yaml.Node) []string {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return workflowScalarValues(node.Alias)
	}
	if node.Kind == yaml.ScalarNode {
		return []string{node.Value}
	}
	var values []string
	for _, child := range node.Content {
		values = append(values, workflowScalarValues(child)...)
	}
	return values
}

type workflowStepAddition struct {
	Name string
	Run  string
	Env  map[string]string
}

func addStepToWorkflowJob(data []byte, jobName string, step workflowStepAddition) ([]byte, error) {
	document, root, err := parseWorkflowDocument(data)
	if err != nil {
		return nil, err
	}
	jobs := workflowMappingValue(root, "jobs")
	job := workflowMappingValue(jobs, jobName)
	if job == nil {
		return nil, fmt.Errorf("workflow job %q is missing", jobName)
	}
	steps := workflowMappingValue(job, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("workflow job %q has no steps sequence", jobName)
	}
	steps.Content = append(steps.Content, workflowStepNode(step))
	return yaml.Marshal(document)
}

func addWorkflowJob(data []byte, jobID, name, run, uses, condition string) ([]byte, error) {
	document, root, err := parseWorkflowDocument(data)
	if err != nil {
		return nil, err
	}
	jobs := workflowMappingValue(root, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("workflow has no jobs mapping")
	}
	if workflowMappingValue(jobs, jobID) != nil {
		return nil, fmt.Errorf("workflow job %q already exists", jobID)
	}
	job := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	appendWorkflowMappingPair(job, "name", workflowStringNode(name))
	if condition != "" {
		appendWorkflowMappingPair(job, "if", workflowStringNode(condition))
	}
	if uses != "" {
		appendWorkflowMappingPair(job, "uses", workflowStringNode(uses))
	} else {
		appendWorkflowMappingPair(job, "runs-on", workflowStringNode("ubuntu-latest"))
		steps := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		if run != "" {
			steps.Content = append(steps.Content, workflowStepNode(workflowStepAddition{Run: run}))
		}
		appendWorkflowMappingPair(job, "steps", steps)
	}
	appendWorkflowMappingPair(jobs, jobID, job)
	return yaml.Marshal(document)
}

func appendWorkflowJobNeed(data []byte, jobID, dependency string) ([]byte, error) {
	document, root, err := parseWorkflowDocument(data)
	if err != nil {
		return nil, err
	}
	job := workflowMappingValue(workflowMappingValue(root, "jobs"), jobID)
	if job == nil {
		return nil, fmt.Errorf("workflow job %q is missing", jobID)
	}
	needs := workflowMappingValue(job, "needs")
	if needs == nil {
		needs = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		appendWorkflowMappingPair(job, "needs", needs)
	} else if needs.Kind == yaml.ScalarNode {
		needs = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{workflowStringNode(needs.Value)}}
		workflowSetMappingValue(job, "needs", needs)
	} else if needs.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("workflow job %q needs is not a sequence", jobID)
	}
	needs.Content = append(needs.Content, workflowStringNode(dependency))
	return yaml.Marshal(document)
}

func workflowStepNode(step workflowStepAddition) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if step.Name != "" {
		appendWorkflowMappingPair(node, "name", workflowStringNode(step.Name))
	}
	if step.Run != "" {
		appendWorkflowMappingPair(node, "run", workflowStringNode(step.Run))
	}
	if len(step.Env) != 0 {
		env := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := make([]string, 0, len(step.Env))
		for key := range step.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			appendWorkflowMappingPair(env, key, workflowStringNode(step.Env[key]))
		}
		appendWorkflowMappingPair(node, "env", env)
	}
	return node
}

func parseWorkflowDocument(data []byte) (*yaml.Node, *yaml.Node, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("workflow document has no root mapping")
	}
	return &document, document.Content[0], nil
}

func workflowMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func appendWorkflowMappingPair(mapping *yaml.Node, key string, value *yaml.Node) {
	mapping.Content = append(mapping.Content, workflowStringNode(key), value)
}

func workflowSetMappingValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	appendWorkflowMappingPair(mapping, key, value)
}

func workflowStringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// TestPullRequestDefersPostLandingTests keeps the slow mutation and exhaustive
// set out of the PR package gate. Its controls reject suite jobs, dependencies,
// and result bridges across PR-eligible jobs; it does not establish main-push execution.
func TestPullRequestDefersPostLandingTests(t *testing.T) {
	good := workflowBytes(t)
	if err := checkPullRequestDefersPostLandingTests(good); err != nil {
		t.Fatalf("valid PR deferral rejected: %v", err)
	}
	tests := []struct {
		name  string
		plant func([]byte) ([]byte, error)
	}{
		{
			name: "restored_pr_job",
			plant: func(data []byte) ([]byte, error) {
				return addWorkflowJob(data, legacyMutationJobID, "Curator mutation controls", "go run ./tools/mutationinventory -mode inventory", "", "")
			},
		},
		{
			name: "renamed_pr_mutation_job",
			plant: func(data []byte) ([]byte, error) {
				return addWorkflowJob(data, "deferred-suite", "Curator mutation controls", "go run ./tools/mutationinventory -mode inventory", "", "")
			},
		},
		{
			name: "mutation_job_with_unknown_condition",
			plant: func(data []byte) ([]byte, error) {
				return addWorkflowJob(data, "suite-lane", "Scheduled analysis", "go run ./tools/mutationinventory -mode inventory", "", "github.event_name == 'pull_request' || inputs.run_suite")
			},
		},
		{
			name: "renamed_reusable_suite_job",
			plant: func(data []byte) ([]byte, error) {
				return addWorkflowJob(data, "quality-check", "Quality check", "", "./.github/workflows/post-landing-mutation-tests.yml", "")
			},
		},
		{
			name: "restored_pr_dependency",
			plant: func(data []byte) ([]byte, error) {
				return appendWorkflowJobNeed(data, "test", legacyMutationJobID)
			},
		},
		{
			name: "legacy_bridge_in_linux_build_controls",
			plant: func(data []byte) ([]byte, error) {
				return addStepToWorkflowJob(data, "linux-build-controls", workflowStepAddition{
					Name: "Require PR mutation gate success",
					Run:  `test "${{ needs.curator-mutation-pr.result }}" = success`,
				})
			},
		},
		{
			name: "renamed_bridge_run",
			plant: func(data []byte) ([]byte, error) {
				return addStepToWorkflowJob(data, "linux-build-controls", workflowStepAddition{
					Name: "Record shard outcome",
					Run:  `echo "${{ needs.curator-mutation-pr.result }}"`,
				})
			},
		},
		{
			name: "renamed_bridge_env",
			plant: func(data []byte) ([]byte, error) {
				return addStepToWorkflowJob(data, "linux-build-controls", workflowStepAddition{
					Name: "Record shard outcome",
					Env:  map[string]string{"MUTATION_RESULT": "${{ needs.curator-mutation-pr.result }}"},
				})
			},
		},
		{
			name: "dynamic_mutation_result_reference",
			plant: func(data []byte) ([]byte, error) {
				return addStepToWorkflowJob(data, "linux-build-controls", workflowStepAddition{
					Name: "Record shard outcome",
					Run:  `echo "${{ needs[vars.MUTATION_JOB].result }}"`,
				})
			},
		},
		{
			name: "renamed_mutation_result_reference",
			plant: func(data []byte) ([]byte, error) {
				mutant, err := addWorkflowJob(data, "deferred-suite", "Curator mutation controls", "go run ./tools/mutationinventory -mode inventory", "", "vars.ROSE_AIR_RUNNER == 'true' && github.event_name == 'push' && github.ref == 'refs/heads/main'")
				if err != nil {
					return nil, err
				}
				return addStepToWorkflowJob(mutant, "linux-build-controls", workflowStepAddition{
					Name: "Record shard outcome",
					Run:  `echo "${{ needs.deferred-suite.result }}"`,
				})
			},
		},
		{
			name: "restored_pr_test_bridge",
			plant: func(data []byte) ([]byte, error) {
				return addStepToWorkflowJob(data, "test", workflowStepAddition{
					Name: "Require PR mutation gate success",
					Run:  `test "${{ needs.curator-mutation-pr.result }}" = success`,
				})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := checkPullRequestDefersPostLandingTests(good); err != nil {
				t.Fatalf("unmodified workflow is not the passing control: %v", err)
			}
			mutant, err := test.plant(good)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkPullRequestDefersPostLandingTests(mutant); err == nil {
				t.Fatal("pull-request deferred-suite violation admitted")
			}
		})
	}
}

func TestAddStepToWorkflowJobRequiresExistingStepsSequence(t *testing.T) {
	good := workflowBytes(t)
	if err := checkPullRequestDefersPostLandingTests(good); err != nil {
		t.Fatalf("unmodified workflow is not the passing control: %v", err)
	}
	if _, err := addStepToWorkflowJob(good, "linux-build-controls", workflowStepAddition{Name: "control", Run: "true"}); err != nil {
		t.Fatalf("valid workflow helper use failed: %v", err)
	}
	for _, test := range []struct {
		name     string
		workflow string
		job      string
	}{
		{name: "missing_job", workflow: "jobs:\n  present:\n    steps: []\n", job: "missing"},
		{name: "missing_steps", workflow: "jobs:\n  present:\n    runs-on: ubuntu-latest\n", job: "present"},
		{name: "non_sequence_steps", workflow: "jobs:\n  present:\n    steps:\n      run: echo\n", job: "present"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := addStepToWorkflowJob([]byte(test.workflow), test.job, workflowStepAddition{Name: "control"}); err == nil {
				t.Fatal("helper accepted a missing job or invalid steps field")
			}
		})
	}
}
