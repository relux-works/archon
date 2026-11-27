package harnessboundary

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/archon/internal/testgitenv"
	"gopkg.in/yaml.v3"
)

func TestMain(m *testing.M) {
	if os.Getenv("HARNESS_CI_TEST_HELPER") == "v1" {
		lane := "parity"
		for _, arg := range os.Args[1:] {
			if arg == "./internal/harnessboundary/cmd/harnessgate" {
				lane = "predecessor"
			}
			if arg == "./internal/harnessboundary" {
				lane = "boundary"
			}
		}
		recordedLane := lane
		if lane == "predecessor" {
			predecessor := os.Getenv("HARNESS_PREDECESSOR_SHA")
			if predecessor != os.Getenv("HARNESS_CI_TEST_EXPECT_PREDECESSOR") {
				os.Exit(70)
			}
			recordedLane += ":" + predecessor
		}
		file, err := os.OpenFile(os.Getenv("HARNESS_CI_TEST_CALLS"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
		if err != nil {
			os.Exit(71)
		}
		_, err = file.WriteString(recordedLane + "\n")
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			os.Exit(72)
		}
		if lane == "predecessor" && os.Getenv("GITHUB_EVENT_NAME") != "pull_request" && os.Getenv("GITHUB_EVENT_NAME") != "push" {
			os.Exit(RatchetMissingExit)
		}
		if lane == os.Getenv("HARNESS_CI_TEST_FAIL") {
			if lane == "predecessor" {
				os.Exit(23)
			}
			os.Exit(19)
		}
		os.Exit(0)
	}
	cleanup := testgitenv.Isolate()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// The actual CI step runs every command when an earlier lane is deliberately
// red. A fully green paired control succeeds; a missing dispatch or unrelated
// shell/setup error cannot satisfy the intended negative control.
func TestHarnessCILaneContinuation(t *testing.T) {
	const parityPattern = `^(TestHomeByteIdentity.*|TestAllAdaptersHaveBaselineCases|TestReplayFixtureSourcePins)$`
	payload, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct{ Name, Run, Shell string }
		}
	}
	if err := yaml.Unmarshal(payload, &workflow); err != nil {
		t.Fatal(err)
	}
	body := ""
	count := 0
	for _, step := range workflow.Jobs["test"].Steps {
		if step.Name == "Harness contract and parity gates" {
			body = step.Run
			count++
			if step.Shell != "bash" {
				t.Fatal("gate shell changed")
			}
		}
	}
	if count != 1 || !strings.Contains(body, "TestNoHarnessBranches") || !strings.Contains(body, parityPattern) {
		t.Fatal("complete harness CI gate missing")
	}
	requiredParityTests := []string{"TestHomeByteIdentity", "TestHomeByteIdentityMutationControl", "TestAllAdaptersHaveBaselineCases", "TestReplayFixtureSourcePins"}
	if !paritySelectorCovers(parityPattern, requiredParityTests) {
		t.Fatal("CI parity selector misses a required tagged test")
	}
	missingSourcePin := strings.Replace(parityPattern, "|TestReplayFixtureSourcePins", "", 1)
	if missingSourcePin == parityPattern || paritySelectorCovers(missingSourcePin, requiredParityTests) {
		t.Fatal("missing source-pin selector plant survived")
	}
	if strings.Contains(body, "${{") || !strings.Contains(body, "jq -r '.pull_request.base.sha // empty'") || !strings.Contains(body, "jq -r '.before // empty'") || !strings.Contains(body, `HARNESS_PREDECESSOR_SHA="$predecessor_sha" go run`) {
		t.Fatal("event predecessor must come from the runner payload and pass through the command environment")
	}
	bootstrapArgs := "--bootstrap-source " + baselineHead + " --bootstrap-frozen-digest " + frozenDigest
	if strings.Count(body, bootstrapArgs) != 1 || strings.Count(body, "--bootstrap-source ") != 1 || strings.Count(body, "--bootstrap-frozen-digest ") != 1 {
		t.Fatal("CI bootstrap must bind exactly the committed source and frozen exception digest")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal("CI Bash unavailable")
	}
	baseSHA, headSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
	runControl := func(t *testing.T, eventName, fail string, expectedExit int) {
		t.Helper()
		root := t.TempDir()
		bin := filepath.Join(root, "bin")
		if err := os.Mkdir(bin, 0o700); err != nil {
			t.Fatal(err)
		}
		name := "go"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(bin, name), binary, 0o700); err != nil {
			t.Fatal(err)
		}
		calls := filepath.Join(root, "calls")
		eventPath := filepath.Join(root, "event.json")
		event := map[string]any{"repository": map[string]string{"full_name": TargetRepository}}
		expectedPredecessor := ""
		ref := "refs/heads/main"
		switch eventName {
		case "pull_request":
			event["pull_request"] = map[string]any{"base": map[string]string{"sha": baseSHA}}
			expectedPredecessor = baseSHA
			ref = "refs/pull/1/merge"
		case "push":
			event["ref"], event["before"], event["after"] = ref, baseSHA, headSHA
			expectedPredecessor = baseSHA
		}
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(eventPath, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bash, "-e", "-c", body)
		cmd.Env = []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "HOME=" + root, "TMPDIR=" + root, "HARNESS_CI_TEST_HELPER=v1", "HARNESS_CI_TEST_CALLS=" + calls, "HARNESS_CI_TEST_FAIL=" + fail, "HARNESS_CI_TEST_EXPECT_PREDECESSOR=" + expectedPredecessor, "GITHUB_EVENT_NAME=" + eventName, "GITHUB_EVENT_PATH=" + eventPath, "GITHUB_REPOSITORY=" + TargetRepository, "GITHUB_REF=" + ref, "GITHUB_SHA=" + headSHA}
		if runtime.GOOS == "windows" {
			cmd.Env = append(cmd.Env, "SystemRoot="+os.Getenv("SystemRoot"))
		}
		err = cmd.Run()
		exit := 0
		if err != nil {
			var failure *exec.ExitError
			if !errors.As(err, &failure) {
				t.Fatal(err)
			}
			exit = failure.ExitCode()
		}
		if exit != expectedExit {
			t.Fatalf("CI outcome=%d want=%d", exit, expectedExit)
		}
		actual, err := os.ReadFile(calls)
		if err != nil {
			t.Fatal(err)
		}
		if string(actual) != "predecessor:"+expectedPredecessor+"\nboundary\nparity\n" {
			t.Fatalf("all selected lanes did not run in order: %q", actual)
		}
	}
	for _, eventName := range []string{"pull_request", "push"} {
		for _, fail := range []string{"", "predecessor", "boundary"} {
			want := 0
			if fail != "" {
				want = 1
			}
			if fail == "predecessor" {
				want = RatchetMissingExit
			}
			t.Run("event-"+eventName+"/first-lane-"+fail, func(t *testing.T) {
				runControl(t, eventName, fail, want)
			})
		}
	}
	t.Run("unsupported-event-has-no-predecessor", func(t *testing.T) { runControl(t, "workflow_dispatch", "", RatchetMissingExit) })
}

func paritySelectorCovers(pattern string, tests []string) bool {
	selected, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	for _, test := range tests {
		if !selected.MatchString(test) {
			return false
		}
	}
	return true
}
