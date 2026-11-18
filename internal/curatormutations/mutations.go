// Package curatormutations defines the tests withheld from pull-request CI and
// run by the post-landing mutation workflow.
package curatormutations

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var mutationTests = []string{
	"TestCommandHelpNarrowingMutants",
	"TestMuseCredentialPolicyNarrowingMutants",
	"TestMuseRootContextNarrowingMutants",
	"TestNativeSkillsTakeoverHelpNarrowingMutant",
	"TestNativeSkillsTakeoverRegistryNarrowingMutant",
	"TestCLIPathIsolationNarrowingMutant",
	"TestWindowsGitBashSelectionNarrowingMutant",
}

// Names returns a copy of the exact mutation-test inventory.
func Names() []string {
	names := append([]string(nil), mutationTests...)
	sort.Strings(names)
	return names
}

// Selector is the exact top-level Go test selector for the mutation suite.
func Selector() string {
	names := Names()
	for i, name := range names {
		names[i] = regexp.QuoteMeta(name)
	}
	return "^(" + strings.Join(names, "|") + ")$"
}

// Inventory partitions one package's unfiltered test names into the tests
// admitted to pull requests and the exact post-landing mutation set.
type Inventory struct {
	All       []string
	Pull      []string
	Mutations []string
}

// Split rejects duplicate names and any newly added mutant-style test that is
// not explicitly admitted to the canonical post-landing set.
func Split(names []string) (Inventory, error) {
	known := make(map[string]bool, len(mutationTests))
	for _, name := range mutationTests {
		known[name] = true
	}

	seen := make(map[string]bool, len(names))
	result := Inventory{All: append([]string(nil), names...)}
	for _, name := range names {
		if name == "" || seen[name] {
			return Inventory{}, fmt.Errorf("empty or duplicate test name %q", name)
		}
		seen[name] = true
		if known[name] {
			result.Mutations = append(result.Mutations, name)
			continue
		}
		if strings.Contains(name, "NarrowingMutant") {
			return Inventory{}, fmt.Errorf("unclassified narrowing mutant test %q", name)
		}
		result.Pull = append(result.Pull, name)
	}
	sort.Strings(result.All)
	sort.Strings(result.Pull)
	sort.Strings(result.Mutations)
	return result, nil
}

// RequireComplete checks that the current platform exposes all seven
// canonical mutation tests. The main post-landing macOS lane uses this guard.
func RequireComplete(inventory Inventory) error {
	if len(inventory.Mutations) != len(mutationTests) {
		return fmt.Errorf("mutation inventory has %d tests, want %d", len(inventory.Mutations), len(mutationTests))
	}
	got := make(map[string]bool, len(inventory.Mutations))
	for _, name := range inventory.Mutations {
		got[name] = true
	}
	for _, name := range mutationTests {
		if !got[name] {
			return fmt.Errorf("mutation inventory is missing %s", name)
		}
	}
	return nil
}

// SkipArgs returns the Go testing flags as a separate flag and value.
func SkipArgs() []string {
	return []string{"-skip", Selector()}
}

// ValidateSkipArgs requires exactly one standalone -skip with the canonical
// selector. It deliberately rejects -skip=<selector> so callers keep the flag
// separate as required by the CI contract.
func ValidateSkipArgs(args []string) error {
	count := 0
	waitingForSelector := false
	for _, arg := range args {
		if waitingForSelector {
			if arg != Selector() {
				return fmt.Errorf("-skip must be followed by the canonical mutation selector")
			}
			waitingForSelector = false
			continue
		}
		if strings.HasPrefix(arg, "-skip=") {
			return fmt.Errorf("mutation selector must use a separate -skip argument")
		}
		if arg != "-skip" {
			continue
		}
		count++
		waitingForSelector = true
	}
	if waitingForSelector {
		return fmt.Errorf("-skip must be followed by the canonical mutation selector")
	}
	if count != 1 {
		return fmt.Errorf("want exactly one canonical -skip pair, got %d", count)
	}
	return nil
}

// ValidateInvocation checks the Go test portion of a command vector that may
// begin with env assignments. It is used immediately before test-gate runs a
// full package invocation.
func ValidateInvocation(args []string) error {
	for i, arg := range args {
		if arg == "test" {
			return ValidateSkipArgs(args[i+1:])
		}
	}
	return fmt.Errorf("command does not contain a go test invocation")
}
