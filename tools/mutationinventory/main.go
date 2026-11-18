// Command mutationinventory exposes the canonical mutation selector and
// records conservation evidence for unfiltered cmd/curator test inventories.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/relux-works/curator/internal/curatormutations"
)

type inventoryReport struct {
	Commit           string   `json:"commit"`
	GOOS             string   `json:"goos"`
	GOARCH           string   `json:"goarch"`
	GoVersion        string   `json:"go_version"`
	GOFLAGSHash      string   `json:"goflags_sha256"`
	Package          string   `json:"package"`
	Timeout          string   `json:"timeout"`
	GoTestFlags      []string `json:"go_test_flags"`
	MutationSelector string   `json:"mutation_selector"`
	Unfiltered       []string `json:"unfiltered"`
	PullRequest      []string `json:"pull_request"`
	Mutations        []string `json:"mutations"`
}

const expectedMacRacePullTests = 531
const expectedMacRaceMutationTests = 7

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mutationinventory:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("mutationinventory", flag.ContinueOnError)
	mode := fs.String("mode", "", "selector, inventory, or verify-invocation")
	goBin := fs.String("go", "go", "Go launcher")
	packagePath := fs.String("package", "./cmd/curator", "package to inventory")
	timeout := fs.String("timeout", "60m", "go test package timeout")
	goTestFlags := fs.String("go-test-flags", "", "additional go test flags, separated by spaces")
	extraGoTestFlags := fs.String("extra-go-test-flags", "", "additional flags supplied by the gate, separated by spaces")
	evidence := fs.String("evidence", "", "directory for the inventory report")
	requireAll := fs.Bool("require-all", false, "require all seven mutation tests")
	if err := fs.Parse(args); err != nil {
		return err
	}
	remaining := fs.Args()

	switch *mode {
	case "selector":
		if len(remaining) != 0 {
			return errors.New("selector mode takes no arguments")
		}
		fmt.Println(curatormutations.Selector())
		return nil
	case "verify-invocation":
		if err := curatormutations.ValidateInvocation(remaining); err != nil {
			return err
		}
		return nil
	case "inventory":
		if len(remaining) != 0 {
			return errors.New("inventory mode takes no arguments")
		}
		if *packagePath == "" || *timeout == "" || *evidence == "" {
			return errors.New("inventory mode requires package, timeout, and evidence")
		}
		flags := append(strings.Fields(*extraGoTestFlags), strings.Fields(*goTestFlags)...)
		return writeInventory(*goBin, *packagePath, *timeout, flags, *evidence, *requireAll)
	default:
		return fmt.Errorf("unknown mode %q", *mode)
	}
}

func writeInventory(goBin, packagePath, timeout string, flags []string, evidence string, requireAll bool) error {
	all, err := listInventory(goBin, packagePath, timeout, flags, ".")
	if err != nil {
		return fmt.Errorf("unfiltered inventory: %w", err)
	}
	if len(all) == 0 {
		return fmt.Errorf("empty unfiltered test inventory for %s", packagePath)
	}
	inventory, err := curatormutations.Split(all)
	if err != nil {
		return err
	}
	selectedMutations, err := listInventory(goBin, packagePath, timeout, flags, curatormutations.Selector())
	if err != nil {
		return fmt.Errorf("mutation -run inventory: %w", err)
	}
	if err := verifyMutationSelection(inventory.Mutations, selectedMutations); err != nil {
		return err
	}
	if len(inventory.All) != len(inventory.Pull)+len(inventory.Mutations) {
		return fmt.Errorf("inventory conservation failed: all=%d pull=%d mutation=%d", len(inventory.All), len(inventory.Pull), len(inventory.Mutations))
	}
	if requireAll {
		if err := curatormutations.RequireComplete(inventory); err != nil {
			return err
		}
	}

	goVersion, err := commandOutput(goBin, "version")
	if err != nil {
		return err
	}
	commit, err := commandOutput("git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	goEnv, err := commandOutput(goBin, "env", "GOOS", "GOARCH")
	if err != nil {
		return err
	}
	platform := strings.Fields(goEnv)
	if len(platform) != 2 {
		return fmt.Errorf("unexpected go env platform output %q", goEnv)
	}
	if platform[0] == "" {
		platform[0] = runtime.GOOS
	}
	if platform[1] == "" {
		platform[1] = runtime.GOARCH
	}
	if err := validateExpectedMacRaceCounts(platform[0], packagePath, flags, inventory); err != nil {
		return err
	}
	effectiveGoFlags, err := commandOutput(goBin, "env", "GOFLAGS")
	if err != nil {
		return err
	}
	report := inventoryReport{
		Commit:           strings.TrimSpace(commit),
		GOOS:             platform[0],
		GOARCH:           platform[1],
		GoVersion:        strings.TrimSpace(goVersion),
		GOFLAGSHash:      fmt.Sprintf("%x", sha256.Sum256([]byte(effectiveGoFlags))),
		Package:          packagePath,
		Timeout:          timeout,
		GoTestFlags:      flags,
		MutationSelector: curatormutations.Selector(),
		Unfiltered:       inventory.All,
		PullRequest:      inventory.Pull,
		Mutations:        inventory.Mutations,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(evidence, 0755); err != nil {
		return err
	}
	path := filepath.Join(evidence, "mutation-inventory.json")
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("inventory %s: unfiltered=%d PR=%d M=%d evidence=%s\n", packagePath, len(inventory.All), len(inventory.Pull), len(inventory.Mutations), path)
	return nil
}

func listInventory(goBin, packagePath, timeout string, flags []string, selector string) ([]string, error) {
	args := []string{"test", "-list", selector, "-timeout", timeout}
	args = append(args, flags...)
	args = append(args, packagePath)
	cmd := exec.Command(goBin, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("command %q failed: %w", append([]string{goBin}, args...), err)
	}
	return parseInventory(stdout.String()), nil
}

func verifyMutationSelection(expected, selected []string) error {
	expected = append([]string(nil), expected...)
	selected = append([]string(nil), selected...)
	sort.Strings(expected)
	sort.Strings(selected)
	if !reflect.DeepEqual(expected, selected) {
		return fmt.Errorf("mutation selector inventory mismatch: unfiltered partition=%v selected by -run=%v", expected, selected)
	}
	return nil
}

func validateExpectedMacRaceCounts(goos, packagePath string, flags []string, inventory curatormutations.Inventory) error {
	curatorPackage := packagePath == "./cmd/curator" || strings.HasSuffix(packagePath, "/cmd/curator")
	if goos != "darwin" || !curatorPackage || len(flags) != 1 || flags[0] != "-race" {
		return nil
	}
	if len(inventory.Pull) != expectedMacRacePullTests || len(inventory.Mutations) != expectedMacRaceMutationTests {
		return fmt.Errorf("Race macOS inventory counts are PR=%d/M=%d, want PR=%d/M=%d", len(inventory.Pull), len(inventory.Mutations), expectedMacRacePullTests, expectedMacRaceMutationTests)
	}
	return nil
}

func parseInventory(output string) []string {
	var names []string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Test") || strings.HasPrefix(line, "Example") || strings.HasPrefix(line, "Fuzz") {
			names = append(names, line)
		}
	}
	sort.Strings(names)
	return names
}

func commandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
