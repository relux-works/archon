package curatormutations

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestCanonicalInventoryAndSelector(t *testing.T) {
	want := []string{
		"TestCLIPathIsolationNarrowingMutant",
		"TestCommandHelpNarrowingMutants",
		"TestMuseCredentialPolicyNarrowingMutants",
		"TestMuseRootContextNarrowingMutants",
		"TestNativeSkillsTakeoverHelpNarrowingMutant",
		"TestNativeSkillsTakeoverRegistryNarrowingMutant",
		"TestWindowsGitBashSelectionNarrowingMutant",
	}
	if got := Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical inventory = %v, want %v", got, want)
	}
	if got := Selector(); got != "^(TestCLIPathIsolationNarrowingMutant|TestCommandHelpNarrowingMutants|TestMuseCredentialPolicyNarrowingMutants|TestMuseRootContextNarrowingMutants|TestNativeSkillsTakeoverHelpNarrowingMutant|TestNativeSkillsTakeoverRegistryNarrowingMutant|TestWindowsGitBashSelectionNarrowingMutant)$" {
		t.Fatalf("canonical selector drifted: %s", got)
	}
	// Callers cannot mutate the canonical list through a returned slice.
	names := Names()
	names[0] = "TestCallerMutation"
	if Names()[0] != want[0] {
		t.Fatal("Names exposed mutable canonical state")
	}
}

func TestSplitConservesInventoryAndRejectsUnclassifiedMutants(t *testing.T) {
	all := append(Names(), "TestOrdinary", "ExampleOrdinary")
	inventory, err := Split(all)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.All) != len(inventory.Pull)+len(inventory.Mutations) {
		t.Fatalf("inventory is not conserved: all=%d pull=%d mutation=%d", len(inventory.All), len(inventory.Pull), len(inventory.Mutations))
	}
	if len(inventory.Pull) != 2 || len(inventory.Mutations) != 7 {
		t.Fatalf("wrong partition: %+v", inventory)
	}
	selector := regexp.MustCompile(Selector())
	for _, name := range inventory.All {
		matched := selector.MatchString(name)
		if matched != contains(inventory.Mutations, name) {
			t.Fatalf("selector partition drift for %s: matched=%v", name, matched)
		}
	}
	if err := RequireComplete(inventory); err != nil {
		t.Fatal(err)
	}

	if _, err := Split(append(all, "TestFutureNarrowingMutant")); err == nil || !strings.Contains(err.Error(), "unclassified narrowing mutant") {
		t.Fatalf("unclassified mutant admitted: %v", err)
	}
	if _, err := Split(append(all, "TestOrdinary")); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate inventory name admitted: %v", err)
	}
	partial, err := Split([]string{"TestOrdinary", Names()[0]})
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireComplete(partial); err == nil {
		t.Fatal("incomplete post-landing inventory admitted")
	}
}

func TestSkipInvocationAndMissingSelectorNegativeControl(t *testing.T) {
	args := append([]string{"test", "-json", "-run", "^TestOrdinary$"}, SkipArgs()...)
	if err := ValidateInvocation(args); err != nil {
		t.Fatalf("canonical ordinary invocation rejected: %v", err)
	}
	// Removing only the selector pair from the ordinary invocation must make
	// the contract check fail, even though its -run selector is unchanged.
	withoutSkip := []string{"test", "-json", "-run", "^TestOrdinary$"}
	if err := ValidateInvocation(withoutSkip); err == nil {
		t.Fatal("ordinary invocation without the mutation skip was admitted")
	}
	selectorWithoutOneMutation := strings.Replace(Selector(), "|TestWindowsGitBashSelectionNarrowingMutant", "", 1)
	if err := ValidateSkipArgs([]string{"-skip", selectorWithoutOneMutation}); err == nil {
		t.Fatal("skip selector missing one mutation was admitted")
	}
	selectorWithOrdinaryTest := strings.TrimSuffix(Selector(), ")$") + "|TestOrdinary)$"
	if err := ValidateSkipArgs([]string{"-skip", selectorWithOrdinaryTest}); err == nil {
		t.Fatal("skip selector including an ordinary test was admitted")
	}
	if err := ValidateSkipArgs([]string{"-skip=" + Selector()}); err == nil {
		t.Fatal("combined -skip argument was admitted")
	}
	if err := ValidateSkipArgs([]string{"-skip"}); err == nil {
		t.Fatal("missing -skip value was admitted")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
