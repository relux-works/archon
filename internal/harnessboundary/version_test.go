package harnessboundary

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/relux-works/archon/internal/testtoolchain"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func registryControlReceipt(payload []byte) error {
	runs, passes := 0, 0
	leaves := map[string]int{}
	for _, line := range bytes.Split(payload, []byte("\n")) {
		var event struct{ Action, Test string }
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		if strings.HasPrefix(event.Test, "TestAPIVersionRefusal/") {
			if event.Action == "skip" || event.Action == "fail" {
				return fmt.Errorf("registry leaf control did not pass")
			}
			if event.Action == "pass" {
				leaves[event.Test]++
			}
		}
		if event.Test == "TestAPIVersionRefusal" {
			switch event.Action {
			case "run":
				runs++
			case "pass":
				passes++
			case "skip", "fail":
				return fmt.Errorf("registry contract control did not pass")
			}
		}
	}
	if runs != 1 || passes != 1 {
		return fmt.Errorf("registry contract witness missing")
	}
	for _, name := range []string{"registered-v2", "reported-v2", "correct-v1", "wrong-id"} {
		if leaves["TestAPIVersionRefusal/"+name] != 1 {
			return fmt.Errorf("registry leaf witness missing")
		}
	}
	return nil
}

// The exact production verifier is private. Its registry-package spies prove
// pre-constructor/pre-operation refusal without exposing a runtime constructor
// API; this required boundary entry point checks the actual child PASS receipt.
func TestAPIVersionRefusal(t *testing.T) {
	cmd := exec.Command(testtoolchain.Go(t), "test", "-count=1", "-json", "./internal/harnessregistry", "-run", "^TestAPIVersionRefusal$")
	cmd.Dir = filepath.Join("..", "..")
	testtoolchain.InheritHostGOROOTLock(t, cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("registry version control failed: %v\n%s", err, output)
	}
	if err := registryControlReceipt(output); err != nil {
		t.Fatal(err)
	}
	positive := []byte("{\"Action\":\"run\",\"Test\":\"TestAPIVersionRefusal\"}\n{\"Action\":\"pass\",\"Test\":\"TestAPIVersionRefusal\"}\n")
	for _, name := range []string{"registered-v2", "reported-v2", "correct-v1", "wrong-id"} {
		event, _ := json.Marshal(map[string]string{"Action": "pass", "Test": "TestAPIVersionRefusal/" + name})
		positive = append(positive, append(event, '\n')...)
	}
	if err := registryControlReceipt(positive); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{nil, []byte("not JSON"), []byte("{\"Action\":\"skip\",\"Test\":\"TestAPIVersionRefusal\"}\n"), []byte("{\"Action\":\"pass\",\"Test\":\"UnrelatedSetup\"}\n")} {
		if err := registryControlReceipt(bad); err == nil {
			t.Fatal("missing/skipped/unrelated registry witness accepted")
		}
	}
}
