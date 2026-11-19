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

func TestParseInventoryKeepsRunnableGoTestNames(t *testing.T) {
	got := parseInventory("TestOne\nExampleOne\nFuzzOne\nBenchmarkOne\nnoise\n")
	want := []string{"ExampleOne", "FuzzOne", "TestOne"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("parsed inventory = %v, want %v", got, want)
	}
}

func TestInventoryCommandProfileAllowsOnlySupportedCIPresets(t *testing.T) {
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

func TestInventoryCommandProfileRejectsCallerControlledValues(t *testing.T) {
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

func TestVerifyGoLauncherIdentityRequiresTheToolchainThatBuiltTheInventoryTool(t *testing.T) {
	if err := verifyGoLauncherIdentity("go version go1.25.5 darwin/arm64", "go1.25.5"); err != nil {
		t.Fatalf("matching launcher rejected: %v", err)
	}
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

func TestNormalizeLineEndingsAcceptsLFAndCRLF(t *testing.T) {
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
	if !strings.Contains(workflow, "on:\n  push:\n    branches: [main]") {
		t.Fatal("post-landing workflow is not restricted to pushes on main")
	}
	for _, forbidden := range []string{"pull_request:", "workflow_dispatch:", "schedule:"} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("post-landing workflow includes a forbidden trigger %q", forbidden)
		}
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
