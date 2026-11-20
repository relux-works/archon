package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/curatormutations"
	"gopkg.in/yaml.v3"
)

// Claim: inventory parsing keeps runnable Go test names and drops tool noise.
// Limit: this checks parser output only; it does not invoke Go or validate test behavior.
func TestParseInventoryKeepsRunnableGoTestNames(t *testing.T) {
	got := parseInventory("TestOne\nExampleOne\nFuzzOne\nBenchmarkOne\nnoise\n")
	want := []string{"ExampleOne", "FuzzOne", "TestOne"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("parsed inventory = %v, want %v", got, want)
	}
}

// Claim: the inventory command admits only the repository's supported CI presets.
// Limit: this does not prove a workflow invokes those presets on the intended runners.
func TestInventoryCommandProfileAllowsOnlySupportedCIPresets(t *testing.T) {
	// Claim: these rows cover accepted package, timeout, and race combinations.
	// Limit: they do not cover arbitrary runner environments or execute the commands.
	tests := []struct {
		name        string
		packagePath string
		timeout     string
		flags       []string
		wantPackage string
		wantTimeout string
		wantFlags   []string
	}{
		{
			name:        "repository path with default timeout",
			packagePath: curatorPackagePath,
			timeout:     inventoryTimeout,
			wantPackage: curatorPackagePath,
			wantTimeout: inventoryTimeout,
		},
		{
			name:        "module path with default timeout",
			packagePath: curatorPackageImportPath,
			timeout:     inventoryTimeout,
			wantPackage: curatorPackageImportPath,
			wantTimeout: inventoryTimeout,
		},
		{
			name:        "Windows timeout",
			packagePath: curatorPackagePath,
			timeout:     windowsInventoryTimeout,
			wantPackage: curatorPackagePath,
			wantTimeout: windowsInventoryTimeout,
		},
		{
			name:        "module path with Windows timeout",
			packagePath: curatorPackageImportPath,
			timeout:     windowsInventoryTimeout,
			wantPackage: curatorPackageImportPath,
			wantTimeout: windowsInventoryTimeout,
		},
		{
			name:        "race lane",
			packagePath: curatorPackagePath,
			timeout:     inventoryTimeout,
			flags:       []string{"-race"},
			wantPackage: curatorPackagePath,
			wantTimeout: inventoryTimeout,
			wantFlags:   []string{"-race"},
		},
		{
			name:        "module path race lane",
			packagePath: curatorPackageImportPath,
			timeout:     inventoryTimeout,
			flags:       []string{"-race"},
			wantPackage: curatorPackageImportPath,
			wantTimeout: inventoryTimeout,
			wantFlags:   []string{"-race"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile, err := newInventoryCommandProfile(test.packagePath, test.timeout, test.flags)
			if err != nil {
				t.Fatalf("supported CI invocation rejected: %v", err)
			}
			if profile.packagePath != test.wantPackage || profile.timeout != test.wantTimeout || !reflect.DeepEqual(profile.goTestFlags(), test.wantFlags) {
				t.Fatalf("profile = %#v, want package=%q timeout=%q flags=%v", profile, test.wantPackage, test.wantTimeout, test.wantFlags)
			}
			// Claim: each preset maps to the exact ordinary or canonical-mutation selector.
			// Limit: argument construction alone does not prove the selected tests pass.
			for _, selection := range []struct {
				value        inventorySelection
				wantSelector string
			}{
				{value: unfilteredInventory, wantSelector: "."},
				{value: mutationInventory, wantSelector: curatormutations.Selector()},
			} {
				cmd, err := inventoryCommand(profile, selection.value)
				if err != nil {
					t.Fatalf("supported inventory command rejected: %v", err)
				}
				wantArgs := []string{"go", "test", "-list", selection.wantSelector, "-timeout", test.wantTimeout}
				wantArgs = append(wantArgs, test.wantFlags...)
				wantArgs = append(wantArgs, test.wantPackage)
				if !reflect.DeepEqual(cmd.Args, wantArgs) {
					t.Fatalf("inventory command args = %q, want %q", cmd.Args, wantArgs)
				}
			}
		})
	}
}

// Claim: unsupported packages, timeouts, flags, and selector values are refused.
// Limit: this covers the local allowlist boundary, not every CLI failure mode.
func TestInventoryCommandProfileRejectsCallerControlledValues(t *testing.T) {
	// Claim: each row is outside the supported CI preset allowlist.
	// Limit: the table does not exhaust malformed command-line syntax.
	tests := []struct {
		name        string
		packagePath string
		timeout     string
		flags       []string
	}{
		{name: "unknown package", packagePath: "../../outside", timeout: inventoryTimeout},
		{name: "unknown timeout", packagePath: curatorPackagePath, timeout: "10s"},
		{name: "arbitrary flag", packagePath: curatorPackagePath, timeout: inventoryTimeout, flags: []string{"-coverprofile=/tmp/out"}},
		{name: "combined race flags", packagePath: curatorPackagePath, timeout: inventoryTimeout, flags: []string{"-race", "-count=1"}},
		{name: "race with Windows timeout", packagePath: curatorPackagePath, timeout: windowsInventoryTimeout, flags: []string{"-race"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newInventoryCommandProfile(test.packagePath, test.timeout, test.flags); err == nil {
				t.Fatalf("caller-controlled inventory values were admitted: package=%q timeout=%q flags=%v", test.packagePath, test.timeout, test.flags)
			}
		})
	}
	profile, err := newInventoryCommandProfile(curatorPackagePath, inventoryTimeout, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inventoryCommand(profile, inventorySelection(99)); err == nil {
		t.Fatal("unknown inventory selector was admitted")
	}
}

// Claim: the launcher version must match the version that built this inventory tool.
// Limit: matching version text does not authenticate the Go executable or its source.
func TestVerifyGoLauncherIdentityRequiresTheToolchainThatBuiltTheInventoryTool(t *testing.T) {
	if err := verifyGoLauncherIdentity("go version go1.25.5 darwin/arm64", "go1.25.5"); err != nil {
		t.Fatalf("matching launcher rejected: %v", err)
	}
	// Claim: a different or unrecognized launcher identity is rejected.
	// Limit: these examples do not test executable replacement after identity checking.
	for _, tc := range []struct {
		name    string
		version string
	}{
		{name: "different toolchain version", version: "go version go1.25.4 darwin/arm64"},
		{name: "unrecognized launcher output", version: "some other toolchain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := verifyGoLauncherIdentity(tc.version, "go1.25.5"); err == nil {
				t.Fatal("different Go launcher identity was admitted")
			}
		})
	}
}

// Claim: the Mac race inventory preserves the ordinary/mutation partition and exact names.
// Limit: the fixtures do not run the mutation tests or establish hosted-runner behavior.
func TestRaceMacInventoryUsesNamedPartitionInvariants(t *testing.T) {
	profile, err := newInventoryCommandProfile(curatorPackagePath, inventoryTimeout, []string{"-race"})
	if err != nil {
		t.Fatal(err)
	}
	mutationNames := curatormutations.Names()
	ordinaryNames := []string{"TestOrdinaryOne", "TestOrdinaryTwo"}
	inventory := inventoryWithPartition(ordinaryNames, mutationNames)
	if err := validateMacRaceInventory("darwin", profile, inventory); err != nil {
		t.Fatalf("complete Darwin race inventory rejected: %v", err)
	}
	if err := validateInventoryPartition(inventory); err != nil {
		t.Fatalf("complete inventory partition rejected: %v", err)
	}

	addedOrdinary := append(append([]string(nil), inventory.All...), "TestAddedOrdinary")
	withAddedOrdinary, err := curatormutations.Split(addedOrdinary)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMacRaceInventory("darwin", profile, withAddedOrdinary); err != nil {
		t.Fatalf("an added ordinary test was rejected: %v", err)
	}

	lostOrdinary := inventory
	lostOrdinary.Pull = append([]string(nil), inventory.Pull[:1]...)
	if err := validateMacRaceInventory("darwin", profile, lostOrdinary); err == nil {
		t.Fatal("an ordinary test missing from PR ∪ M was admitted")
	}

	duplicateAcross := inventory
	duplicateAcross.Mutations = append(append([]string(nil), inventory.Mutations...), inventory.Pull[0])
	if err := validateMacRaceInventory("darwin", profile, duplicateAcross); err == nil {
		t.Fatal("a test duplicated across PR and M was admitted")
	}

	eighthMutation := inventory
	eighthMutation.All = append(append([]string(nil), inventory.All...), "TestUnexpectedAdditionalMutation")
	eighthMutation.Mutations = append(append([]string(nil), inventory.Mutations...), "TestUnexpectedAdditionalMutation")
	if err := validateInventoryPartition(eighthMutation); err != nil {
		t.Fatalf("the eighth-name fixture is not a conserved partition: %v", err)
	}
	if err := validateMacRaceInventory("darwin", profile, eighthMutation); err == nil {
		t.Fatal("an eighth mutation name was admitted")
	}

	missingMutation := inventoryWithPartition(ordinaryNames, mutationNames[:len(mutationNames)-1])
	if err := validateInventoryPartition(missingMutation); err != nil {
		t.Fatalf("the missing-name fixture is not a conserved partition: %v", err)
	}
	if err := validateMacRaceInventory("darwin", profile, missingMutation); err == nil {
		t.Fatal("a missing canonical mutation name was admitted")
	}

	renamedMutationNames := append([]string(nil), mutationNames...)
	renamedMutationNames[0] = "TestRenamedMutation"
	renamedMutation := inventoryWithPartition(ordinaryNames, renamedMutationNames)
	if err := validateInventoryPartition(renamedMutation); err != nil {
		t.Fatalf("the renamed-name fixture is not a conserved partition: %v", err)
	}
	if err := validateMacRaceInventory("darwin", profile, renamedMutation); err == nil {
		t.Fatal("a renamed mutation name was admitted")
	}

	if err := validateMacRaceInventory("linux", profile, missingMutation); err != nil {
		t.Fatalf("non-macOS invocation received the Mac mutation inventory policy: %v", err)
	}
	if _, err := newInventoryCommandProfile(curatorPackagePath, inventoryTimeout, []string{"-race", "-tags=curator_test_platform"}); err == nil {
		t.Fatal("caller-controlled race flags were admitted")
	}
}

func inventoryWithPartition(ordinary, mutations []string) curatormutations.Inventory {
	all := append(append([]string(nil), ordinary...), mutations...)
	return curatormutations.Inventory{
		All:       all,
		Pull:      append([]string(nil), ordinary...),
		Mutations: append([]string(nil), mutations...),
	}
}

// Claim: ordinary Go test invocations must carry the canonical mutation skip.
// Limit: this does not prove a separate PR job actually runs the skipped tests.
func TestOrdinaryGoTestInvocationRejectsMissingMutationSkip(t *testing.T) {
	base := []string{"-mode", "verify-invocation", "--", "env", "CURATOR_CONFORMANCE_ROOT=/tmp/root", "go", "test", "-json", "-run", "^TestOrdinary$"}
	withSkip := append(append([]string(nil), base...), curatormutations.SkipArgs()...)
	withSkip = append(withSkip, "./cmd/curator")
	if err := run(withSkip); err != nil {
		t.Fatalf("ordinary invocation with canonical skip rejected: %v", err)
	}
	// This is the ordinary-lane negative control: remove just the skip pair
	// from the same invocation while leaving its package and run selector intact.
	withoutSkip := removePair(withSkip, "-skip")
	if err := run(withoutSkip); err == nil {
		t.Fatal("ordinary invocation without the mutation selector was admitted")
	}
}

func removePair(args []string, flag string) []string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return append(append([]string(nil), args[:i]...), args[i+2:]...)
		}
	}
	return append([]string(nil), args...)
}

// Claim: a mutation selector must contain exactly the canonical mutation inventory.
// Limit: this does not prove workflow wiring or the behavior of selected tests.
func TestMutationRunInventoryMustMatchCanonicalPartition(t *testing.T) {
	all := append(curatormutations.Names(), "TestOrdinary")
	inventory, err := curatormutations.Split(all)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyMutationSelection(inventory.Mutations, curatormutations.Names()); err != nil {
		t.Fatalf("canonical -run set rejected: %v", err)
	}
	missing := curatormutations.Names()[:len(curatormutations.Names())-1]
	if err := verifyMutationSelection(inventory.Mutations, missing); err == nil {
		t.Fatal("mutation selector missing a canonical test was admitted")
	}
	if err := verifyMutationSelection(inventory.Mutations, append(curatormutations.Names(), "TestOrdinary")); err == nil {
		t.Fatal("mutation selector including an ordinary test was admitted")
	}
}

// Claim: every ordinary test-gate invocation explicitly opts out of mutation tests.
// Limit: this does not establish that the dedicated PR mutation job runs them.
func TestEveryCITestGateInvocationOptsOutOfMutationSuite(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil || len(parsed) == 0 {
		t.Fatalf("ci.yml is not valid YAML: %v", err)
	}
	var blocks []string
	var current strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "      - ") {
			if current.Len() != 0 {
				blocks = append(blocks, current.String())
				current.Reset()
			}
		}
		if current.Len() != 0 || strings.HasPrefix(line, "      - ") {
			current.WriteString(line)
			current.WriteByte('\n')
		}
	}
	if current.Len() != 0 {
		blocks = append(blocks, current.String())
	}

	count := 0
	for _, block := range blocks {
		if !strings.Contains(block, ".github/ci/test-gate.sh") {
			continue
		}
		count++
		if !strings.Contains(block, `CI_SKIP_MUTATION_TESTS: "1"`) {
			t.Errorf("test-gate workflow step does not opt out of mutation tests:\n%s", block)
		}
		if strings.Contains(block, "GO_TEST_FLAGS: -race -skip") || strings.Contains(block, "GO_TEST_FLAGS: '-race -skip") {
			t.Errorf("mutation skip was folded into GO_TEST_FLAGS:\n%s", block)
		}
	}
	if count != 5 {
		t.Fatalf("found %d test-gate steps, want five", count)
	}
}

// Claim: the test-gate compatibility flag expression remains one safe Bash argument.
// Limit: this checks the expression and local Bash behavior, not a complete CI run.
func TestTestGateStoreCompatFlagsExpansionIsNounsetSafe(t *testing.T) {
	const safeExpansion = `"${STORE_COMPAT_FLAGS[*]+"${STORE_COMPAT_FLAGS[*]}"}"`

	data, err := os.ReadFile("../../.github/ci/test-gate.sh")
	if err != nil {
		t.Fatal(err)
	}
	var flagLines []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, "-extra-go-test-flags") {
			continue
		}
		flagLines = append(flagLines, trimmed)
	}
	wantFlagLine := "-extra-go-test-flags " + safeExpansion + " -evidence \"$EVIDENCE/mutation-inventory/$stage\")"
	if len(flagLines) != 1 || flagLines[0] != wantFlagLine {
		t.Fatalf("active test-gate store flags argument lines = %q, want exactly %q", flagLines, wantFlagLine)
	}

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("bash is unavailable for the expansion behavior check: %v", err)
	}

	// Claim: empty and populated compatibility values retain the expected argument shape.
	// Limit: the examples do not cover every Bash release or environment configuration.
	for _, tc := range []struct {
		name       string
		assignment string
		want       string
	}{
		{
			name:       "empty compatibility flags retain the required empty argument",
			assignment: "STORE_COMPAT_FLAGS=()",
			want:       "count=4\nflag=<>\n",
		},
		{
			name:       "populated compatibility flags remain one argument",
			assignment: "STORE_COMPAT_FLAGS=(-race -count=1)",
			want:       "count=4\nflag=<-race -count=1>\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := "set -u\n" + tc.assignment + "\nargs=(-extra-go-test-flags " + safeExpansion + " -evidence evidence)\nprintf 'count=%d\\nflag=<%s>\\n' \"${#args[@]}\" \"${args[1]}\"\n"
			got, err := exec.Command(bash, "-c", script).CombinedOutput()
			if err != nil {
				t.Fatalf("nounset-safe expansion failed: %v\n%s", err, got)
			}
			if string(got) != tc.want {
				t.Fatalf("argument shape = %q, want %q", got, tc.want)
			}
		})
	}
	// The source check and argument-shape cases above cover the contract. Bash
	// releases differ on whether the legacy empty-array form fails under nounset.
}

func normalizeLineEndings(data []byte) string {
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// Claim: LF and CRLF workflow text normalize to the same LF form.
// Limit: this does not parse YAML or check workflow semantics.
func TestNormalizeLineEndingsAcceptsLFAndCRLF(t *testing.T) {
	// Claim: both line-ending forms are accepted by the normalizer.
	// Limit: the table covers only these two byte sequences.
	for _, tc := range []struct {
		name string
		data string
	}{
		{name: "LF", data: "on:\n  push:\n"},
		{name: "CRLF", data: "on:\r\n  push:\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeLineEndings([]byte(tc.data)); got != "on:\n  push:\n" {
				t.Fatalf("normalized workflow = %q, want LF form", got)
			}
		})
	}
}

func mainPushOnlyTrigger(workflow map[string]any) bool {
	triggers, ok := workflow["on"].(map[string]any)
	if !ok || len(triggers) != 1 {
		return false
	}
	push, ok := triggers["push"].(map[string]any)
	if !ok || len(push) != 1 {
		return false
	}
	branches, ok := push["branches"].([]any)
	return ok && len(branches) == 1 && branches[0] == "main"
}

// Claim: only a push to main satisfies the post-landing trigger predicate.
// Limit: this does not validate jobs or triggers outside the modeled mapping.
func TestMainPushOnlyTriggerRejectsOtherEventsAndBranches(t *testing.T) {
	// Claim: the table covers main push, another branch, an extra event, and malformed input.
	// Limit: it does not enumerate every valid GitHub Actions trigger form.
	for _, tc := range []struct {
		name     string
		workflow map[string]any
		want     bool
	}{
		{
			name: "main push only",
			workflow: map[string]any{"on": map[string]any{
				"push": map[string]any{"branches": []any{"main"}},
			}},
			want: true,
		},
		{
			name: "other branch",
			workflow: map[string]any{"on": map[string]any{
				"push": map[string]any{"branches": []any{"release"}},
			}},
		},
		{
			name: "additional event",
			workflow: map[string]any{"on": map[string]any{
				"push":         map[string]any{"branches": []any{"main"}},
				"pull_request": map[string]any{},
			}},
		},
		{
			name:     "trigger text is not a mapping",
			workflow: map[string]any{"on": "push to main"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mainPushOnlyTrigger(tc.workflow); got != tc.want {
				t.Fatalf("main-push-only trigger = %t, want %t", got, tc.want)
			}
		})
	}
}

// Claim: the post-landing workflow is main-push-only and selects the canonical mutation set.
// Limit: this validates workflow text and YAML, not a completed hosted run.
func TestPostLandingWorkflowIsMainPushOnlyAndRunsTheCanonicalSet(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/post-landing-mutation-tests.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := normalizeLineEndings(data)
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil || len(parsed) == 0 {
		t.Fatalf("post-landing workflow is not valid YAML: %v", err)
	}
	if !mainPushOnlyTrigger(parsed) {
		t.Fatal("post-landing workflow must trigger only on pushes to main")
	}
	for _, required := range []string{
		"runs-on: macos-latest",
		"-mode inventory -package ./cmd/curator -timeout 60m",
		"-go-test-flags=-race -require-all",
		"selector=\"$(go run ./tools/mutationinventory -mode selector)\"",
		"go test -race -count=1 -timeout=60m -v",
		"-run \"$selector\" ./cmd/curator",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("post-landing workflow is missing %q", required)
		}
	}
	if strings.Contains(workflow, "continue-on-error:") {
		t.Fatal("post-landing mutation verdict is not required")
	}
}

// Claim: every PR OS gate inventories and runs all seven canonical mutation tests,
// and omitting one selected name is rejected by the same inventory comparator.
// Limit: this checks workflow wiring and selector comparison, not hosted execution
// or the behavioral strength of each mutation test.
func TestPullRequestMutationWorkflowRunsCanonicalSetOnEveryPriorOS(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil || len(parsed) == 0 {
		t.Fatalf("ci.yml is not valid YAML: %v", err)
	}
	jobs, ok := parsed["jobs"].(map[string]any)
	if !ok {
		t.Fatal("ci.yml has no jobs mapping")
	}
	triggers, ok := parsed["on"].(map[string]any)
	if !ok {
		t.Fatal("ci.yml has no event mapping")
	}
	if _, ok := triggers["pull_request"]; !ok {
		t.Fatal("ci.yml does not trigger pull-request validation")
	}
	job, ok := jobs["curator-mutation-pr"].(map[string]any)
	if !ok {
		t.Fatal("ci.yml has no curator-mutation-pr job")
	}
	if job["if"] != "${{ github.event_name == 'pull_request' }}" {
		t.Fatalf("PR mutation job condition = %v", job["if"])
	}
	if job["runs-on"] != "${{ matrix.os }}" {
		t.Fatalf("PR mutation job runner = %v, want matrix.os", job["runs-on"])
	}
	if _, ok := job["continue-on-error"]; ok {
		t.Fatal("PR mutation result is allowed to fail")
	}
	testJob, ok := jobs["test"].(map[string]any)
	if !ok {
		t.Fatal("ci.yml has no test gate job")
	}
	needs, ok := testJob["needs"].([]any)
	if !ok {
		t.Fatal("test gate has no needs list")
	}
	dependsOnMutations := false
	for _, dependency := range needs {
		if dependency == "curator-mutation-pr" {
			dependsOnMutations = true
		}
	}
	if !dependsOnMutations {
		t.Fatal("existing test gate does not depend on curator-mutation-pr")
	}
	testSteps, ok := testJob["steps"].([]any)
	if !ok {
		t.Fatal("test gate has no steps")
	}
	bridgeFound := false
	for _, item := range testSteps {
		step, ok := item.(map[string]any)
		if !ok || step["name"] != "Require PR mutation gate success" {
			continue
		}
		bridgeFound = true
		if step["if"] != "always()" {
			t.Errorf("PR mutation bridge condition = %v, want always()", step["if"])
		}
		env, ok := step["env"].(map[string]any)
		if !ok || env["CURATOR_MUTATION_RESULT"] != "${{ needs.curator-mutation-pr.result }}" {
			t.Errorf("PR mutation bridge does not read the dependency result: %v", step["env"])
		}
		run, ok := step["run"].(string)
		if !ok || !strings.Contains(run, `if [[ "$GITHUB_EVENT_NAME" == "pull_request" ]]`) ||
			!strings.Contains(run, `test "$CURATOR_MUTATION_RESULT" = success`) {
			t.Errorf("PR mutation bridge does not fail closed on pull requests: %v", step["run"])
		}
	}
	if !bridgeFound {
		t.Fatal("existing test gate has no PR mutation result check")
	}

	strategy, ok := job["strategy"].(map[string]any)
	if !ok {
		t.Fatal("PR mutation job has no strategy mapping")
	}
	matrix, ok := strategy["matrix"].(map[string]any)
	if !ok {
		t.Fatal("PR mutation job has no matrix mapping")
	}
	included, ok := matrix["include"].([]any)
	if !ok {
		t.Fatal("PR mutation job matrix has no include rows")
	}
	gotMatrix := make(map[string]string, len(included))
	for _, item := range included {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("invalid PR mutation matrix row: %#v", item)
		}
		osName, osOK := row["os"].(string)
		timeout, timeoutOK := row["timeout"].(string)
		if !osOK || !timeoutOK {
			t.Fatalf("PR mutation matrix row lacks OS or timeout: %#v", row)
		}
		if _, exists := gotMatrix[osName]; exists {
			t.Fatalf("duplicate PR mutation OS %q", osName)
		}
		gotMatrix[osName] = timeout
	}
	// Claim: the ordinary PR gate restores the mutation lane to every prior OS.
	// Limit: this compares configured runner labels, not the hosted images themselves.
	wantMatrix := map[string]string{
		"ubuntu-latest":  "60m",
		"macos-latest":   "60m",
		"windows-latest": "120m",
	}
	if !reflect.DeepEqual(gotMatrix, wantMatrix) {
		t.Fatalf("PR mutation matrix = %v, want %v", gotMatrix, wantMatrix)
	}

	steps, ok := job["steps"].([]any)
	if !ok {
		t.Fatal("PR mutation job has no steps")
	}
	stepRun := func(wantName string) string {
		t.Helper()
		for _, item := range steps {
			step, ok := item.(map[string]any)
			if !ok || step["name"] != wantName {
				continue
			}
			run, ok := step["run"].(string)
			if !ok {
				t.Fatalf("step %q has no run script", wantName)
			}
			return run
		}
		t.Fatalf("PR mutation job has no %q step", wantName)
		return ""
	}
	inventoryRun := stepRun("Inventory complete pre-merge mutation suite")
	for _, required := range []string{
		"bash .github/ci/isolated-test.sh go run ./tools/mutationinventory",
		"-mode inventory -package ./cmd/curator -timeout ${{ matrix.timeout }}",
		"-require-all -evidence .temp/ci-evidence/pr-mutation",
	} {
		if !strings.Contains(inventoryRun, required) {
			t.Errorf("PR mutation inventory step is missing %q", required)
		}
	}
	testRun := stepRun("Run all seven pre-merge mutation tests")
	for _, required := range []string{
		"selector=\"$(go run ./tools/mutationinventory -mode selector)\"",
		"bash .github/ci/isolated-test.sh go test -count=1",
		"-timeout=${{ matrix.timeout }} -v -run \"$selector\" ./cmd/curator",
	} {
		if !strings.Contains(testRun, required) {
			t.Errorf("PR mutation test step is missing %q", required)
		}
	}

	// Claim: the complete canonical selector passes, while removing one name fails.
	// Limit: this exercises the comparator with names; it does not edit or run CI YAML.
	// This negative control models deleting one test from the PR gate's selected set.
	names := curatormutations.Names()
	if len(names) != 7 {
		t.Fatalf("canonical PR mutation selector has %d names, want seven", len(names))
	}
	// Claim: these selector cases cover the full gate and a one-name omission.
	// Limit: other malformed selector forms are covered by the general inventory tests.
	for _, tc := range []struct {
		name     string
		selected []string
		wantErr  bool
	}{
		{name: "all seven in PR gate", selected: names},
		{name: "one removed from PR gate", selected: names[:len(names)-1], wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyMutationSelection(names, tc.selected)
			if (err != nil) != tc.wantErr {
				t.Fatalf("verifyMutationSelection error = %v, wantErr=%t", err, tc.wantErr)
			}
		})
	}
}

// Claim: other workflow cmd/curator invocations remain narrowly selected.
// Limit: this line-oriented scan does not execute workflows or parse expressions.
func TestOtherWorkflowCmdCuratorInvocationsAreNarrow(t *testing.T) {
	files, err := filepath.Glob("../../.github/workflows/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, path := range files {
		if strings.HasSuffix(path, "post-landing-mutation-tests.yml") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var blocks []string
		var current strings.Builder
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "      - ") {
				if current.Len() != 0 {
					blocks = append(blocks, current.String())
					current.Reset()
				}
			}
			if current.Len() != 0 || strings.HasPrefix(line, "      - ") {
				current.WriteString(line)
				current.WriteByte('\n')
			}
		}
		if current.Len() != 0 {
			blocks = append(blocks, current.String())
		}
		for _, block := range blocks {
			if !strings.Contains(block, "go test") || !strings.Contains(block, "cmd/curator") || strings.Contains(block, "test-gate.sh") {
				continue
			}
			checked++
			if !strings.Contains(block, "-run") {
				t.Errorf("direct cmd/curator workflow test is not narrowly selected (%s):\n%s", filepath.Base(path), block)
			}
			for _, name := range curatormutations.Names() {
				if strings.Contains(block, name) {
					t.Errorf("direct workflow test selects a post-landing mutation test (%s): %s", filepath.Base(path), name)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no direct cmd/curator workflow invocations were checked")
	}
}
