package harnessboundary

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func ratchetFixture() (ExceptionSet, ExceptionSet, Exception) {
	a := Exception{"internal/shared/policy.go", "codex_cli", strings.Repeat("a", 64)}
	b := Exception{"internal/shared/policy.go", "muse", strings.Repeat("b", 64)}
	frozen := ExceptionSet{SourceHead: OriginalSourceHead, Exceptions: []Exception{a, b}}
	return frozen, ExceptionSet{SourceHead: OriginalSourceHead, Exceptions: []Exception{a}}, b
}

func requireRatchetCode(t *testing.T, err error, code string) {
	t.Helper()
	var failure *RatchetError
	if !errors.As(err, &failure) || failure.Code() != code {
		t.Fatalf("typed refusal: got=%v want=%s", err, code)
	}
}

// Each refusal is paired with accepted unchanged/shrinking controls. Checking
// the serialized inputs after every verdict proves the rule does not modify
// source allowances, including the historical restoration regression.
func testHistoricalRatchet(t *testing.T) {
	frozen, shrunk, b := ratchetFixture()
	previous := VerifiedPredecessor{sha: strings.Repeat("1", 40), digest: strings.Repeat("2", 64), exists: true, active: shrunk, verified: true}
	for _, row := range []struct {
		name      string
		previous  VerifiedPredecessor
		active    ExceptionSet
		bootstrap BootstrapBinding
		code      string
	}{
		{"unchanged", previous, shrunk, BootstrapBinding{}, ""},
		{"further-shrink", previous, ExceptionSet{SourceHead: OriginalSourceHead}, BootstrapBinding{}, ""},
		{"restored-B", previous, frozen, BootstrapBinding{}, RatchetGrowth},
		{"bootstrap-prior-present", previous, shrunk, BootstrapBinding{OriginalSourceHead, FrozenExceptionDigest}, RatchetBootstrapForbidden},
		{"missing-proof", VerifiedPredecessor{}, shrunk, BootstrapBinding{}, RatchetMissingInput},
		{"absent-without-bootstrap", VerifiedPredecessor{sha: previous.sha, digest: EmptyPayloadDigest, verified: true}, shrunk, BootstrapBinding{}, RatchetMissingInput},
		{"proven-first-bootstrap", VerifiedPredecessor{sha: previous.sha, digest: EmptyPayloadDigest, verified: true}, frozen, BootstrapBinding{OriginalSourceHead, FrozenExceptionDigest}, ""},
		{"bootstrap-binding-mismatch", VerifiedPredecessor{sha: previous.sha, digest: EmptyPayloadDigest, verified: true}, frozen, BootstrapBinding{OriginalSourceHead, strings.Repeat("3", 64)}, RatchetBindingMismatch},
	} {
		t.Run(row.name, func(t *testing.T) {
			before, err := json.Marshal([]ExceptionSet{frozen, row.previous.active, row.active})
			if err != nil {
				t.Fatal(err)
			}
			err = CheckRatchet(frozen, row.active, row.previous, row.bootstrap)
			if row.code == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				requireRatchetCode(t, err, row.code)
				if row.name == "restored-B" {
					var failure *RatchetError
					if !errors.As(err, &failure) || failure.exception == nil || *failure.exception != b {
						t.Fatal("restoration did not identify B")
					}
				}
			}
			after, err := json.Marshal([]ExceptionSet{frozen, row.previous.active, row.active})
			if err != nil || string(after) != string(before) {
				t.Fatal("ratchet mutated input state")
			}
		})
	}
}

// Only target event fields select CI predecessors. A claimed head, zero/missing
// before, wrong repository/ref or unsupported event fails before materialization.
func testTargetEventPredecessor(t *testing.T) {
	base, head := strings.Repeat("1", 40), strings.Repeat("2", 40)
	event := targetEvent{Repository: eventRepository{FullName: TargetRepository}, Ref: "refs/heads/main", Before: base, After: head}
	event.PullRequest.Base.SHA, event.PullRequest.Head.SHA = base, head
	event.MergeGroup.BaseSHA, event.MergeGroup.HeadSHA = base, head
	for _, name := range []string{"pull_request", "merge_group", "push"} {
		before := event
		actual, err := selectTargetPredecessor(name, TargetRepository, "refs/heads/main", base, event)
		if err != nil || actual != base {
			t.Fatalf("valid %s refused: %v", name, err)
		}
		_, err = selectTargetPredecessor(name, TargetRepository, "refs/heads/main", head, event)
		requireRatchetCode(t, err, RatchetMissingInput)
		if !reflect.DeepEqual(before, event) {
			t.Fatal("target event mutated")
		}
	}
	for _, before := range []string{"", strings.Repeat("0", 40)} {
		bad := event
		bad.Before = before
		snapshot := bad
		_, err := selectTargetPredecessor("push", TargetRepository, "refs/heads/main", before, bad)
		requireRatchetCode(t, err, RatchetMissingInput)
		if !reflect.DeepEqual(snapshot, bad) {
			t.Fatal("missing/zero predecessor changed event state")
		}
	}
	_, err := selectTargetPredecessor("push", TargetRepository, "refs/heads/topic", base, event)
	requireRatchetCode(t, err, RatchetMissingInput)
	_, err = selectTargetPredecessor("workflow_dispatch", TargetRepository, "refs/heads/main", base, event)
	requireRatchetCode(t, err, RatchetMissingInput)
}

func testPredecessorContract(t *testing.T) {
	t.Run("pure-history", testHistoricalRatchet)
	t.Run("target-event", testTargetEventPredecessor)
	t.Run("materialization", testPredecessorMaterialization)
	t.Run("CLI-inputs", testRatchetCLIInputs)
	t.Run("verified-archive", testVerifiedArchivePredecessor)
	t.Run("read-confinement", testPredecessorReadConfinement)
	t.Run("diagnostics", testRatchetDiagnostics)
	t.Run("argument-refusals", testRatchetArgumentRefusals)
}
