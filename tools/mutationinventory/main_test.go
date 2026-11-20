package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
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
	for _, test := range []struct {
		name  string
		plant func([]byte) ([]byte, error)
	}{
		{
			name: "missing_cpu_count",
			plant: func(workflow []byte) ([]byte, error) {
				mutant := strings.Replace(string(workflow), "fmt.Println(runtime.NumCPU())", "fmt.Println(runtime.NumGoroutine())", 1)
				if mutant == string(workflow) {
					return nil, fmt.Errorf("CPU count statement not found")
				}
				return []byte(mutant), nil
			},
		},
		{
			name: "missing_effective_gomaxprocs",
			plant: func(workflow []byte) ([]byte, error) {
				mutant := strings.Replace(string(workflow), "fmt.Println(runtime.GOMAXPROCS(0))", "fmt.Println(runtime.NumCPU())", 1)
				if mutant == string(workflow) {
					return nil, fmt.Errorf("GOMAXPROCS statement not found")
				}
				return []byte(mutant), nil
			},
		},
		{
			name: "wrong_runner_condition",
			plant: func(workflow []byte) ([]byte, error) {
				return setWorkflowStepCondition(workflow, "race", "Record macOS race capacity", "runner.os == 'Linux'")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutant, err := test.plant(data)
			if err != nil {
				t.Fatalf("capacity-step plant not applied: %v", err)
			}
			if err := checkMacOSRaceCapacityStep(mutant); err == nil {
				t.Fatal("invalid macOS capacity step admitted")
			}
		})
	}
	t.Run("wrong_runner_condition_on_crlf", func(t *testing.T) {
		crlf := []byte(strings.ReplaceAll(normalizeLineEndings(data), "\n", "\r\n"))
		if err := checkMacOSRaceCapacityStep(crlf); err != nil {
			t.Fatalf("valid CRLF workflow rejected: %v", err)
		}
		mutant, err := setWorkflowStepCondition(crlf, "race", "Record macOS race capacity", "runner.os == 'Linux'")
		if err != nil {
			t.Fatalf("CRLF condition plant failed: %v", err)
		}
		condition, err := workflowStepConditionValue(mutant, "race", "Record macOS race capacity")
		if err != nil {
			t.Fatal(err)
		}
		if condition != "runner.os == 'Linux'" {
			t.Fatalf("CRLF plant condition = %q, want Linux", condition)
		}
		if err := checkMacOSRaceCapacityStep(mutant); err == nil {
			t.Fatal("wrong runner condition admitted after CRLF plant")
		}
	})
}

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

func checkPRDefersPostLandingTests(data []byte) error {
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

func setWorkflowStepCondition(data []byte, jobID, stepName, condition string) ([]byte, error) {
	document, root, err := parseWorkflowDocument(data)
	if err != nil {
		return nil, err
	}
	job := workflowMappingValue(workflowMappingValue(root, "jobs"), jobID)
	if job == nil {
		return nil, fmt.Errorf("workflow job %q is missing", jobID)
	}
	steps := workflowMappingValue(job, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("workflow job %q has no steps sequence", jobID)
	}
	found := false
	for _, step := range steps.Content {
		name := workflowMappingValue(step, "name")
		if name == nil || name.Value != stepName {
			continue
		}
		if found {
			return nil, fmt.Errorf("workflow job %q has multiple steps named %q", jobID, stepName)
		}
		workflowSetMappingValue(step, "if", workflowStringNode(condition))
		found = true
	}
	if !found {
		return nil, fmt.Errorf("workflow job %q has no step named %q", jobID, stepName)
	}
	return yaml.Marshal(document)
}

func workflowStepConditionValue(data []byte, jobID, stepName string) (string, error) {
	_, root, err := parseWorkflowDocument(data)
	if err != nil {
		return "", err
	}
	job := workflowMappingValue(workflowMappingValue(root, "jobs"), jobID)
	if job == nil {
		return "", fmt.Errorf("workflow job %q is missing", jobID)
	}
	steps := workflowMappingValue(job, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return "", fmt.Errorf("workflow job %q has no steps sequence", jobID)
	}
	for _, step := range steps.Content {
		name := workflowMappingValue(step, "name")
		if name == nil || name.Value != stepName {
			continue
		}
		condition := workflowMappingValue(step, "if")
		if condition == nil || condition.Kind != yaml.ScalarNode {
			return "", fmt.Errorf("workflow job %q step %q has no scalar if condition", jobID, stepName)
		}
		return condition.Value, nil
	}
	return "", fmt.Errorf("workflow job %q has no step named %q", jobID, stepName)
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

// Claim: PR test gates defer the expensive suite, while the main-push workflow owns it.
// Limit: this checks static wiring; hosted execution and product behavior remain unverified.
func TestPullRequestWorkflowDefersMutationAndExhaustiveSuites(t *testing.T) {
	good := mutationInventoryWorkflowBytes(t)
	if strings.Contains(string(good), "tools/mutationinventory") {
		t.Fatal("PR workflow invokes the deferred-suite inventory tool directly")
	}
	if err := checkPRDefersPostLandingTests(good); err != nil {
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
			if err := checkPRDefersPostLandingTests(good); err != nil {
				t.Fatalf("unmodified workflow is not the passing control: %v", err)
			}
			mutant, err := test.plant(good)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkPRDefersPostLandingTests(mutant); err == nil {
				t.Fatal("pull-request deferred-suite violation admitted")
			}
		})
	}
}

func TestAddStepToWorkflowJobRequiresExistingStepsSequence(t *testing.T) {
	good := mutationInventoryWorkflowBytes(t)
	if err := checkPRDefersPostLandingTests(good); err != nil {
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

func mutationInventoryWorkflowBytes(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	return data
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
