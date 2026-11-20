package main

import (
	"fmt"
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
			// Claim: each preset maps to the ordinary, mutation, or post-landing selector.
			// Limit: argument construction alone does not prove the selected tests pass.
			for _, selection := range []struct {
				value        inventorySelection
				wantSelector string
			}{
				{value: unfilteredInventory, wantSelector: "."},
				{value: mutationInventory, wantSelector: curatormutations.Selector()},
				{value: postLandingInventory, wantSelector: curatormutations.PostLandingSelector()},
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

// Claim: the Mac race inventory preserves the PR/mutation/exhaustive partition and exact names.
// Limit: the fixtures do not run deferred tests or establish hosted-runner behavior.
func TestRaceMacInventoryUsesNamedPartitionInvariants(t *testing.T) {
	profile, err := newInventoryCommandProfile(curatorPackagePath, inventoryTimeout, []string{"-race"})
	if err != nil {
		t.Fatal(err)
	}
	mutationNames := curatormutations.Names()
	exhaustiveNames := curatormutations.ExhaustiveNames()
	ordinaryNames := []string{"TestOrdinaryOne", "TestOrdinaryTwo"}
	inventory := inventoryWithPartition(ordinaryNames, mutationNames, exhaustiveNames)
	if err := validateMacRaceInventory("darwin", profile, inventory); err != nil {
		t.Fatalf("complete Darwin race inventory rejected: %v", err)
	}
	if err := validateInventoryPartition(inventory); err != nil {
		t.Fatalf("complete inventory partition rejected: %v", err)
	}
	if err := curatormutations.RequirePostLandingComplete(inventory); err != nil {
		t.Fatalf("complete post-landing inventory rejected: %v", err)
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
		t.Fatal("an ordinary test missing from PR ∪ M ∪ exhaustive was admitted")
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

	missingMutation := inventoryWithPartition(ordinaryNames, mutationNames[:len(mutationNames)-1], exhaustiveNames)
	if err := validateInventoryPartition(missingMutation); err != nil {
		t.Fatalf("the missing-name fixture is not a conserved partition: %v", err)
	}
	if err := validateMacRaceInventory("darwin", profile, missingMutation); err == nil {
		t.Fatal("a missing canonical mutation name was admitted")
	}

	renamedMutationNames := append([]string(nil), mutationNames...)
	renamedMutationNames[0] = "TestRenamedMutation"
	renamedMutation := inventoryWithPartition(ordinaryNames, renamedMutationNames, exhaustiveNames)
	if err := validateInventoryPartition(renamedMutation); err != nil {
		t.Fatalf("the renamed-name fixture is not a conserved partition: %v", err)
	}
	if err := validateMacRaceInventory("darwin", profile, renamedMutation); err == nil {
		t.Fatal("a renamed mutation name was admitted")
	}
	missingExhaustive := inventoryWithPartition(ordinaryNames, mutationNames, nil)
	if err := validateInventoryPartition(missingExhaustive); err != nil {
		t.Fatalf("missing-exhaustive fixture is not a conserved partition: %v", err)
	}
	if err := validateMacRaceInventory("darwin", profile, missingExhaustive); err == nil {
		t.Fatal("post-landing inventory without the exhaustive product-behavior test was admitted")
	}

	if err := validateMacRaceInventory("linux", profile, missingMutation); err != nil {
		t.Fatalf("non-macOS invocation received the Mac mutation inventory policy: %v", err)
	}
	if _, err := newInventoryCommandProfile(curatorPackagePath, inventoryTimeout, []string{"-race", "-tags=curator_test_platform"}); err == nil {
		t.Fatal("caller-controlled race flags were admitted")
	}
}

func inventoryWithPartition(ordinary, mutations, exhaustive []string) curatormutations.Inventory {
	all := append(append(append([]string(nil), ordinary...), mutations...), exhaustive...)
	return curatormutations.Inventory{
		All:        all,
		Pull:       append([]string(nil), ordinary...),
		Mutations:  append([]string(nil), mutations...),
		Exhaustive: append([]string(nil), exhaustive...),
	}
}

// Claim: ordinary Go test invocations must carry the canonical post-landing skip.
// Limit: this does not prove the main-push workflow actually runs the skipped tests.
func TestOrdinaryGoTestInvocationRejectsMissingPostLandingSkip(t *testing.T) {
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
		t.Fatal("ordinary invocation without the post-landing selector was admitted")
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
	all := append(append(curatormutations.Names(), curatormutations.ExhaustiveNames()...), "TestOrdinary")
	inventory, err := curatormutations.Split(all)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyMutationSelection(inventory.Mutations, curatormutations.Names()); err != nil {
		t.Fatalf("canonical -run set rejected: %v", err)
	}
	postLanding := append(append([]string(nil), inventory.Mutations...), inventory.Exhaustive...)
	if err := verifyPostLandingSelection(postLanding, curatormutations.PostLandingNames()); err != nil {
		t.Fatalf("complete post-landing -run set rejected: %v", err)
	}
	missing := curatormutations.Names()[:len(curatormutations.Names())-1]
	if err := verifyMutationSelection(inventory.Mutations, missing); err == nil {
		t.Fatal("mutation selector missing a canonical test was admitted")
	}
	if err := verifyMutationSelection(inventory.Mutations, append(curatormutations.Names(), "TestOrdinary")); err == nil {
		t.Fatal("mutation selector including an ordinary test was admitted")
	}
	missingExhaustive := append([]string(nil), inventory.Mutations...)
	if err := verifyPostLandingSelection(postLanding, missingExhaustive); err == nil {
		t.Fatal("post-landing selector missing the exhaustive test was admitted")
	}
	if err := verifyPostLandingSelection(postLanding, append(postLanding, "TestOrdinary")); err == nil {
		t.Fatal("post-landing selector including an ordinary test was admitted")
	}
}

// Claim: every ordinary test-gate invocation explicitly skips deferred tests.
// Limit: this does not establish that the main-push lane runs them.
func TestEveryCITestGateInvocationSkipsDeferredSet(t *testing.T) {
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
		if !strings.Contains(block, `CI_SKIP_POST_LANDING_TESTS: "1"`) {
			t.Errorf("test-gate workflow step does not skip the post-landing set:\n%s", block)
		}
		if strings.Contains(block, "GO_TEST_FLAGS: -race -skip") || strings.Contains(block, "GO_TEST_FLAGS: '-race -skip") {
			t.Errorf("post-landing skip was folded into GO_TEST_FLAGS:\n%s", block)
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
	wantFlagLine := "-extra-go-test-flags " + safeExpansion + " -evidence \"$EVIDENCE/post-landing-inventory/$stage\")"
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

// Claim: the post-landing workflow inventories and runs every deferred test on main.
// Limit: this validates workflow text and YAML, not a completed hosted run.
func TestPostLandingWorkflowIsMainPushOnlyAndRunsCompleteDeferredSet(t *testing.T) {
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
	jobs, ok := parsed["jobs"].(map[string]any)
	if !ok {
		t.Fatal("post-landing workflow has no jobs mapping")
	}
	job, ok := jobs["curator-deferred-tests"].(map[string]any)
	if !ok || job["runs-on"] != "macos-latest" {
		t.Fatalf("post-landing job is missing or uses the wrong runner: %#v", job)
	}
	if _, ok := job["continue-on-error"]; ok {
		t.Fatal("post-landing test verdict is allowed to fail")
	}
	for _, required := range []string{
		"-mode inventory -package ./cmd/curator -timeout 60m",
		"-go-test-flags=-race -require-all",
		"selector=\"$(go run ./tools/mutationinventory -mode post-landing-selector)\"",
		"go test -race -count=1 -timeout=60m -v",
		"-run \"$selector\" ./cmd/curator",
		"go-test.txt",
		"name: post-landing-mutation-evidence",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("post-landing workflow is missing %q", required)
		}
	}
	if strings.Contains(workflow, "continue-on-error:") {
		t.Fatal("post-landing verdict is not required")
	}
}

func checkMacOSRaceCapacityStep(data []byte) error {
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return err
	}
	jobs, ok := parsed["jobs"].(map[string]any)
	if !ok {
		return fmt.Errorf("ci.yml has no jobs mapping")
	}
	race, ok := jobs["race"].(map[string]any)
	if !ok {
		return fmt.Errorf("ci.yml has no Race job")
	}
	steps, ok := race["steps"].([]any)
	if !ok {
		return fmt.Errorf("Race job has no steps")
	}
	found := 0
	for _, value := range steps {
		step, ok := value.(map[string]any)
		if !ok || step["name"] != "Record macOS race capacity" {
			continue
		}
		found++
		if step["if"] != "runner.os == 'macOS'" {
			return fmt.Errorf("capacity step is not limited to the macOS runner")
		}
		run, ok := step["run"].(string)
		if !ok {
			return fmt.Errorf("capacity step has no shell script")
		}
		for _, required := range []string{
			"fmt.Println(runtime.NumCPU())",
			"fmt.Println(runtime.GOMAXPROCS(0))",
			"GOTOOLCHAIN=local GOENV=off go run \"$RUNNER_TEMP/go-runner-capacity.go\"",
			"tee .temp/ci-evidence/race/runner-capacity.txt",
		} {
			if !strings.Contains(run, required) {
				return fmt.Errorf("capacity step is missing %q", required)
			}
		}
		if strings.Count(run, "fmt.Println(") != 2 || strings.Contains(run, "fmt.Printf(") {
			return fmt.Errorf("capacity step prints data beyond the two requested numeric values")
		}
	}
	if found != 1 {
		return fmt.Errorf("found %d macOS race capacity steps, want exactly one", found)
	}
	return nil
}

// Claim: the PR macOS Race job logs CPU count and effective GOMAXPROCS as two numeric lines.
// Limit: this checks workflow source, not the hosted runner's emitted values.
func TestMacOSRaceJobLogsRunnerCapacity(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkMacOSRaceCapacityStep(data); err != nil {
		t.Fatalf("valid macOS capacity step rejected: %v", err)
	}
	for name, plant := range map[string]func(string) string{
		"missing_cpu_count": func(workflow string) string {
			return strings.Replace(workflow, "fmt.Println(runtime.NumCPU())", "fmt.Println(runtime.NumGoroutine())", 1)
		},
		"missing_effective_gomaxprocs": func(workflow string) string {
			return strings.Replace(workflow, "fmt.Println(runtime.GOMAXPROCS(0))", "fmt.Println(runtime.NumCPU())", 1)
		},
		"wrong_runner_condition": func(workflow string) string {
			return strings.Replace(workflow,
				"- name: Record macOS race capacity\n        if: runner.os == 'macOS'",
				"- name: Record macOS race capacity\n        if: runner.os == 'Linux'", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			mutant := plant(string(data))
			if mutant == string(data) {
				t.Fatal("capacity-step plant not applied")
			}
			if err := checkMacOSRaceCapacityStep([]byte(mutant)); err == nil {
				t.Fatal("invalid macOS capacity step admitted")
			}
		})
	}
}

func checkPRDefersPostLandingTests(data []byte) error {
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return err
	}
	jobs, ok := parsed["jobs"].(map[string]any)
	if !ok {
		return fmt.Errorf("ci.yml has no jobs mapping")
	}
	if _, ok := jobs["curator-mutation-pr"]; ok {
		return fmt.Errorf("deferred mutation job runs in pull-request CI")
	}
	job, ok := jobs["test"].(map[string]any)
	if !ok {
		return fmt.Errorf("ci.yml has no test gate job")
	}
	needs, ok := job["needs"].([]any)
	if !ok || !reflect.DeepEqual(needs, []any{"windows-test-shards", "unix-test-shards", "store-v1-compatibility"}) {
		return fmt.Errorf("test gate has unexpected dependencies: %v", job["needs"])
	}
	steps, ok := job["steps"].([]any)
	if !ok {
		return fmt.Errorf("test gate has no steps")
	}
	for _, value := range steps {
		step, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if step["name"] == "Require PR mutation gate success" {
			return fmt.Errorf("test gate still requires a PR mutation job")
		}
	}
	return nil
}

// Claim: PR test gates defer the expensive suite, while the main-push workflow owns it.
// Limit: this checks static wiring; hosted execution and product behavior remain unverified.
func TestPullRequestWorkflowDefersMutationAndExhaustiveSuites(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "tools/mutationinventory") {
		t.Fatal("PR workflow invokes the deferred-suite inventory tool directly")
	}
	if err := checkPRDefersPostLandingTests(data); err != nil {
		t.Fatalf("valid PR deferral rejected: %v", err)
	}
	for name, mutate := range map[string]func(string) string{
		"restored_pr_mutation_job": func(workflow string) string {
			return strings.Replace(workflow, "  default-build-transport:", "  curator-mutation-pr:\n    runs-on: macos-latest\n  default-build-transport:", 1)
		},
		"restored_pr_mutation_dependency": func(workflow string) string {
			return strings.Replace(workflow,
				"needs: [windows-test-shards, unix-test-shards, store-v1-compatibility]",
				"needs: [windows-test-shards, unix-test-shards, store-v1-compatibility, curator-mutation-pr]", 1)
		},
		"restored_pr_mutation_result_bridge": func(workflow string) string {
			return strings.Replace(workflow,
				"      - name: Checkout authoritative protocol suite",
				"      - name: Require PR mutation gate success\n        run: test \"$CURATOR_MUTATION_RESULT\" = success\n      - name: Checkout authoritative protocol suite", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			mutant := mutate(string(data))
			if mutant == string(data) {
				t.Fatal("workflow plant not applied")
			}
			if err := checkPRDefersPostLandingTests([]byte(mutant)); err == nil {
				t.Fatal("pull-request deferral violation admitted")
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
			if strings.Contains(block, `-run "$`) || strings.Contains(block, `-run "${`) {
				t.Errorf("direct cmd/curator workflow test uses a dynamic selector (%s):\n%s", filepath.Base(path), block)
			}
			for _, name := range curatormutations.PostLandingNames() {
				if strings.Contains(block, name) {
					t.Errorf("direct workflow test selects a post-landing test (%s): %s", filepath.Base(path), name)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no direct cmd/curator workflow invocations were checked")
	}
}
