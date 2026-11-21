package harnessboundary

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/testgitenv"
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
		file, err := os.OpenFile(os.Getenv("HARNESS_CI_TEST_CALLS"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
		if err != nil {
			os.Exit(71)
		}
		_, err = file.WriteString(lane + "\n")
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			os.Exit(72)
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
	if count != 1 || !strings.Contains(body, "TestNoHarnessBranches") || !strings.Contains(body, "TestHomeByteIdentityMutationControl") {
		t.Fatal("complete harness CI gate missing")
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
	for _, fail := range []string{"", "predecessor", "boundary"} {
		t.Run("first-lane-"+fail, func(t *testing.T) {
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
			cmd := exec.Command(bash, "-e", "-c", body)
			cmd.Env = []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "HOME=" + root, "TMPDIR=" + root, "HARNESS_CI_TEST_HELPER=v1", "HARNESS_CI_TEST_CALLS=" + calls, "HARNESS_CI_TEST_FAIL=" + fail}
			if runtime.GOOS == "windows" {
				cmd.Env = append(cmd.Env, "SystemRoot="+os.Getenv("SystemRoot"))
			}
			err := cmd.Run()
			exit := 0
			if err != nil {
				var failure *exec.ExitError
				if !errors.As(err, &failure) {
					t.Fatal(err)
				}
				exit = failure.ExitCode()
			}
			want := 0
			if fail != "" {
				want = 1
			}
			if fail == "predecessor" {
				want = RatchetMissingExit
			}
			if exit != want {
				t.Fatalf("CI outcome=%d want=%d", exit, want)
			}
			actual, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != "predecessor\nboundary\nparity\n" {
				t.Fatalf("all selected lanes did not run in order: %q", actual)
			}
		})
	}
}
