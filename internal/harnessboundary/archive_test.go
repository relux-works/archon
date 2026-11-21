package harnessboundary

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Archive data preserves the predecessor's exact byte spelling (including its
// final LF), and its independently supplied descriptor/payload pins must agree.
// A valid descriptor pairs every integrity refusal; no branch/network fallback
// or source modification is permitted after a refusal.
func testVerifiedArchivePredecessor(t *testing.T) {
	f := makePredecessorFixture(t)
	payload := append(append([]byte{}, f.active...), '\n')
	descriptor := archiveDescriptor{Version: 1, SHA: f.prior, Digest: digest(payload), Exists: true, Active: payload}
	data, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "predecessor.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	previous, err := readArchivePredecessor(path, digest(data), f.prior, digest(payload))
	if err != nil || !previous.verified || !previous.exists {
		t.Fatalf("valid archive refused: %v", err)
	}
	for _, row := range []struct{ name, descriptorHash, sha, payloadHash string }{
		{"descriptor-digest", strings.Repeat("3", 64), f.prior, digest(payload)},
		{"payload-digest", digest(data), f.prior, strings.Repeat("4", 64)},
		{"commit-binding", digest(data), strings.Repeat("5", 40), digest(payload)},
	} {
		t.Run(row.name, func(t *testing.T) {
			before := sourceState(t, f.root)
			_, err := readArchivePredecessor(path, row.descriptorHash, row.sha, row.payloadHash)
			requireRatchetCode(t, err, RatchetBindingMismatch)
			if sourceState(t, f.root) != before {
				t.Fatal("archive refusal changed source")
			}
		})
	}
	var out, diagnostics bytes.Buffer
	args := []string{"--predecessor-descriptor", path, "--descriptor-digest", digest(data), "--predecessor-sha", f.prior, "--predecessor-digest", digest(payload)}
	if RunRatchetCLI(context.Background(), args, f.root, func(string) string { return "" }, &out, &diagnostics) != 0 {
		t.Fatalf("archive CLI refused: %s", diagnostics.String())
	}
	before := sourceState(t, f.root)
	out.Reset()
	diagnostics.Reset()
	code := RunRatchetCLI(context.Background(), args, f.root, func(k string) string {
		if k == "GITHUB_ACTIONS" {
			return "true"
		}
		return ""
	}, &out, &diagnostics)
	if code != RatchetMissingExit || strings.Contains(out.String(), "=accepted") {
		t.Fatal("CI accepted archive/head-controlled predecessor route")
	}
	if sourceState(t, f.root) != before {
		t.Fatal("CI archive refusal changed source")
	}
}

// Source reads stay below the admitted root. The outside fixture is harmless,
// but a symlink to it must be refused without changing either fixture tree.
func testPredecessorReadConfinement(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	fixtureWrite(t, root, "regular", []byte("fixture\n"))
	if data, err := boundedRootFile(root, "regular"); err != nil || string(data) != "fixture\n" {
		t.Fatal("regular positive control refused")
	}
	fixtureWrite(t, outside, "payload", []byte("outside fixture\n"))
	if err := os.Symlink(filepath.Join(outside, "payload"), filepath.Join(root, "escape")); err != nil {
		t.Fatal("isolated symlink control could not run")
	}
	_, err := boundedRootFile(root, "escape")
	requireRatchetCode(t, err, RatchetMissingInput)
	data, err := os.ReadFile(filepath.Join(outside, "payload"))
	if err != nil || string(data) != "outside fixture\n" {
		t.Fatal("outside fixture changed")
	}
}

// A restored duplicate consumes the predecessor's allowance one-to-one. The
// diagnostic identifies the new physical occurrence, not the retained one.
func testRatchetDiagnostics(t *testing.T) {
	root := t.TempDir()
	const rel = "internal/shared/policy.go"
	const line = "if selected==\"codex_cli\" {return true}"
	fixtureWrite(t, root, rel, []byte("package shared\nfunc choose(selected string) bool {\n"+line+"\n"+line+"\nreturn false\n}\n"))
	vocabulary, err := json.Marshal(map[string][]string{"codex_cli": {"codex_cli"}})
	if err != nil {
		t.Fatal(err)
	}
	fixtureWrite(t, root, "internal/harnessboundary/testdata/vocabulary.json", vocabulary)
	a := Exception{rel, "codex_cli", digest([]byte(line))}
	prior := ExceptionSet{SourceHead: OriginalSourceHead, Exceptions: []Exception{a}}
	frozen := ExceptionSet{SourceHead: OriginalSourceHead, Exceptions: []Exception{a, a}}
	previous := VerifiedPredecessor{sha: strings.Repeat("1", 40), digest: strings.Repeat("2", 64), exists: true, active: prior, verified: true}
	if err = CheckRatchet(frozen, prior, previous, BootstrapBinding{}); err != nil {
		t.Fatal("valid retained occurrence refused")
	}
	err = CheckRatchet(frozen, frozen, previous, BootstrapBinding{})
	requireRatchetCode(t, err, RatchetGrowth)
	failure, ok := err.(*RatchetError)
	if !ok {
		t.Fatal("typed growth missing")
	}
	var output bytes.Buffer
	ratchetDiagnostic(root, failure, &output)
	if !strings.Contains(output.String(), rel+":4:") || strings.Contains(output.String(), rel+":3:") {
		t.Fatal("diagnostic did not identify the restored occurrence")
	}
}

// No-input and malformed-argument paths fail before source/Git access and never
// echo argument values. Help is a non-operational positive control, not an
// accepted ratchet result.
func testRatchetArgumentRefusals(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{nil, {"--unrecognized=do-not-echo"}, {"--predecessor-sha=do-not-echo"}} {
		var out, diagnostics bytes.Buffer
		code := RunRatchetCLI(context.Background(), args, root, func(string) string { return "" }, &out, &diagnostics)
		if code != RatchetMissingExit || strings.Contains(out.String()+diagnostics.String(), "do-not-echo") || !strings.Contains(diagnostics.String(), RatchetMissingInput) {
			t.Fatal("argument refusal leaked values or accepted missing input")
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatal("missing-input path wrote state")
		}
	}
	var out, diagnostics bytes.Buffer
	if code := RunRatchetCLI(context.Background(), []string{"--help"}, root, func(string) string { return "" }, &out, &diagnostics); code != 0 || out.Len() == 0 || strings.Contains(out.String(), "=accepted") {
		t.Fatal("help positive control failed")
	}
}
