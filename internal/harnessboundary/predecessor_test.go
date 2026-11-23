package harnessboundary

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureGitCommand(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "user.name=Fixture Contributor", "-c", "user.email=fixture@example.invalid"}, args...)...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated Git fixture failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func fixtureWrite(t *testing.T, root, path string, data []byte) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0600); err != nil {
		t.Fatal(err)
	}
}

type predecessorFixture struct {
	root, absent, prior, head, unrelated string
	active                               []byte
}

func makePredecessorFixture(t *testing.T) predecessorFixture {
	t.Helper()
	origin := t.TempDir()
	fixtureGitCommand(t, origin, "init", "-q", "-b", "main")
	fixtureWrite(t, origin, "marker", []byte("base\n"))
	fixtureGitCommand(t, origin, "add", "--", "marker")
	fixtureGitCommand(t, origin, "commit", "-qm", "fixture base")
	absent := fixtureGitCommand(t, origin, "rev-parse", "HEAD")
	frozenBytes, err := os.ReadFile(filepath.Join("testdata", "frozen-exceptions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var frozen ExceptionSet
	if err = json.Unmarshal(frozenBytes, &frozen); err != nil {
		t.Fatal(err)
	}
	active, err := json.Marshal(ExceptionSet{SourceHead: OriginalSourceHead, Exceptions: frozen.Exceptions[:1]})
	if err != nil {
		t.Fatal(err)
	}
	fixtureWrite(t, origin, ActiveExceptionPath, active)
	fixtureGitCommand(t, origin, "add", "--", ActiveExceptionPath)
	fixtureGitCommand(t, origin, "commit", "-qm", "fixture predecessor")
	prior := fixtureGitCommand(t, origin, "rev-parse", "HEAD")
	fixtureWrite(t, origin, "marker", []byte("head\n"))
	fixtureGitCommand(t, origin, "add", "--", "marker")
	fixtureGitCommand(t, origin, "commit", "-qm", "fixture head")
	head := fixtureGitCommand(t, origin, "rev-parse", "HEAD")
	tree := fixtureGitCommand(t, origin, "rev-parse", "HEAD^{tree}")
	// Synthetic fixture objects are confined to this temporary Git repository;
	// no publication commit, signing key, auth channel or external remote is used.
	unrelated := fixtureGitCommand(t, origin, "commit-tree", tree, "-m", "fixture unrelated")
	fixtureGitCommand(t, origin, "update-ref", "refs/heads/unrelated", unrelated)
	root := filepath.Join(t.TempDir(), "checkout")
	fixtureGitCommand(t, origin, "clone", "--quiet", "--no-local", "--depth=1", origin, root)
	fixtureWrite(t, root, FrozenExceptionPath, frozenBytes)
	return predecessorFixture{root, absent, prior, head, unrelated, active}
}

func sourceState(t *testing.T, root string) string {
	t.Helper()
	var state bytes.Buffer
	for _, path := range []string{ActiveExceptionPath, FrozenExceptionPath, "marker"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		state.WriteString(path + ":" + digest(data) + "\n")
	}
	return state.String()
}

// Real isolated Git objects establish ancestry, file presence and exact bytes.
// Every refusal preserves current source/frozen/active content; permitted fetch
// object-cache changes are not source writes. A valid shallow checkout is the
// paired positive control for force/unreachable/digest refusals.
func testPredecessorMaterialization(t *testing.T) {
	f := makePredecessorFixture(t)
	for _, row := range []struct{ name, sha, hash, code string }{
		{"valid-shallow", f.prior, digest(f.active), ""},
		{"wrong-digest", f.prior, strings.Repeat("9", 64), RatchetBindingMismatch},
		{"force-unrelated-before", f.unrelated, digest(f.active), RatchetMissingInput},
		{"unreachable-before", strings.Repeat("7", 40), digest(f.active), RatchetMissingInput},
		{"zero-before", strings.Repeat("0", 40), digest(f.active), RatchetMissingInput},
		{"proven-absence", f.absent, EmptyPayloadDigest, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			before := sourceState(t, f.root)
			previous, err := materializePredecessor(context.Background(), f.root, row.sha, row.hash, f.head, false)
			if row.code == "" {
				if err != nil || !previous.verified || previous.digest != row.hash || previous.exists != (row.sha != f.absent) {
					t.Fatalf("valid predecessor: %v", err)
				}
			} else {
				requireRatchetCode(t, err, row.code)
			}
			if sourceState(t, f.root) != before {
				t.Fatal("refusal/materialization changed source state")
			}
		})
	}
}

// The CLI's real local lane binds a fresh origin/main observation. Missing
// flags, stale SHA, digest mismatch and forbidden bootstrap have distinct
// negative witnesses, with accepted local and explicit first-bootstrap peers.
func testRatchetCLIInputs(t *testing.T) {
	f := makePredecessorFixture(t)
	baseArgs := []string{"--predecessor-sha", f.head, "--predecessor-digest", digest(f.active)}
	emptyEnv := func(string) string { return "" }
	for _, row := range []struct {
		name string
		args []string
		want int
		code string
	}{
		{"valid-local", baseArgs, 0, ""},
		{"no-predecessor-flags", nil, RatchetMissingExit, RatchetMissingInput},
		{"missing-digest", []string{"--predecessor-sha", f.head}, RatchetMissingExit, RatchetMissingInput},
		{"stale-default", []string{"--predecessor-sha", f.prior, "--predecessor-digest", digest(f.active)}, 24, RatchetBindingMismatch},
		{"bootstrap-prior-present", append(append([]string{}, baseArgs...), "--bootstrap", "--bootstrap-source", OriginalSourceHead, "--bootstrap-frozen-digest", FrozenExceptionDigest), 24, RatchetBootstrapForbidden},
	} {
		t.Run(row.name, func(t *testing.T) {
			before := sourceState(t, f.root)
			var out, diagnostics bytes.Buffer
			code := RunRatchetCLI(context.Background(), row.args, f.root, emptyEnv, &out, &diagnostics)
			if code != row.want || row.code != "" && !strings.Contains(diagnostics.String(), row.code) {
				t.Fatalf("CLI code=%d diagnostics=%s", code, diagnostics.String())
			}
			if code != 0 && strings.Contains(out.String(), "=accepted") {
				t.Fatal("refusal emitted acceptance")
			}
			if sourceState(t, f.root) != before {
				t.Fatal("CLI refusal changed source")
			}
		})
	}
	for _, row := range []struct {
		name, claimed, base, bootstrapSource, bootstrapDigest string
		want                                                  int
		refusal                                               string
	}{
		{"CI-valid-prior", f.prior, f.prior, "", "", 0, ""},
		{"CI-head-source", f.head, f.prior, "", "", RatchetMissingExit, RatchetMissingInput},
		{"CI-explicit-first-bootstrap", f.absent, f.absent, "", "", 0, ""},
		{"CI-stale-first-bootstrap-tuple", f.absent, f.absent, "643e727edd2db21307ca6d8c0332935e33d50ac8", "d400920c0956af1ad90b9e964828c6c9dea2fbe8786b3064d133d825e945f0e9", 24, RatchetBindingMismatch},
	} {
		t.Run(row.name, func(t *testing.T) {
			event := targetEvent{Repository: eventRepository{FullName: TargetRepository}}
			event.PullRequest.Base.SHA, event.PullRequest.Head.SHA = row.base, f.head
			data, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "event.json")
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			env := map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": "pull_request", "GITHUB_REPOSITORY": TargetRepository, "GITHUB_REF": "refs/pull/1/merge", "GITHUB_SHA": f.head, "GITHUB_EVENT_PATH": path, "HARNESS_PREDECESSOR_SHA": row.claimed}
			before := sourceState(t, f.root)
			var out, diagnostics bytes.Buffer
			bootstrapSource, bootstrapDigest := row.bootstrapSource, row.bootstrapDigest
			if bootstrapSource == "" {
				bootstrapSource = OriginalSourceHead
			}
			if bootstrapDigest == "" {
				bootstrapDigest = FrozenExceptionDigest
			}
			code := RunRatchetCLI(context.Background(), []string{"--ci", "--bootstrap-source", bootstrapSource, "--bootstrap-frozen-digest", bootstrapDigest}, f.root, func(k string) string { return env[k] }, &out, &diagnostics)
			if code != row.want {
				t.Fatalf("CI code=%d diagnostics=%s", code, diagnostics.String())
			}
			if row.refusal != "" && !strings.Contains(diagnostics.String(), row.refusal) {
				t.Fatalf("CI refusal did not name %s: %s", row.refusal, diagnostics.String())
			}
			if code != 0 && strings.Contains(out.String(), "=accepted") {
				t.Fatal("CI refusal emitted acceptance")
			}
			if sourceState(t, f.root) != before {
				t.Fatal("CI changed source state")
			}
		})
	}
}
