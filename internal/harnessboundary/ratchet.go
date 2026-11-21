package harnessboundary

import "strings"

// These pins identify the immutable original source and its exception bytes.
const (
	OriginalSourceHead    = "643e727edd2db21307ca6d8c0332935e33d50ac8"
	FrozenExceptionDigest = "d400920c0956af1ad90b9e964828c6c9dea2fbe8786b3064d133d825e945f0e9"
	EmptyPayloadDigest    = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	TargetRepository      = "relux-works/curator"
	ActiveExceptionPath   = "internal/harnessboundary/testdata/exceptions.json"
	FrozenExceptionPath   = "internal/harnessboundary/testdata/frozen-exceptions.json"
)

// Ratchet refusal codes distinguish unavailable evidence from policy growth.
const (
	RatchetMissingInput       = "harness_predecessor_input_missing"
	RatchetBindingMismatch    = "harness_predecessor_binding_mismatch"
	RatchetBootstrapForbidden = "harness_predecessor_bootstrap_forbidden"
	RatchetGrowth             = "harness_exception_restored_or_added"
)

// RatchetError never renders event payloads, command errors or input values.
type RatchetError struct {
	code       string
	exception  *Exception
	allowances ExceptionSet
	operation  string
}

// Error returns the stable refusal code.
func (e *RatchetError) Error() string { return e.code }

// Code returns the stable refusal code.
func (e *RatchetError) Code() string { return e.code }

func ratchetError(code string) error { return &RatchetError{code: code} }

// VerifiedPredecessor is produced only by bound Git or archive materialization.
// Its private fields prevent callers from declaring an unchecked file absent.
type VerifiedPredecessor struct {
	sha, digest string
	exists      bool
	active      ExceptionSet
	verified    bool
}

// BootstrapBinding explicitly authorizes only the original first introduction.
type BootstrapBinding struct {
	SourceHead   string
	FrozenDigest string
}

func validHex(value string, length int) bool {
	if len(value) != length || value == strings.Repeat("0", length) {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func firstExceptionGrowth(before, after ExceptionSet) *Exception {
	counts := map[string]int{}
	for _, e := range before.Exceptions {
		counts[key(e)]++
	}
	for _, e := range after.Exceptions {
		if counts[key(e)] == 0 {
			candidate := e
			return &candidate
		}
		counts[key(e)]--
	}
	return nil
}

// CheckRatchet preserves both the original ceiling and historical monotonicity.
// Materialization establishes the predecessor's provenance before this pure
// comparison; an absent file is never inferred from an empty or failed read.
func CheckRatchet(frozen, active ExceptionSet, previous VerifiedPredecessor, bootstrap BootstrapBinding) error {
	if !previous.verified || !validHex(previous.sha, 40) || !validHex(previous.digest, 64) {
		return ratchetError(RatchetMissingInput)
	}
	if frozen.SourceHead != OriginalSourceHead || active.SourceHead != OriginalSourceHead {
		return ratchetError(RatchetBindingMismatch)
	}
	if growth := firstExceptionGrowth(frozen, active); growth != nil {
		return &RatchetError{code: RatchetGrowth, exception: growth, allowances: frozen}
	}
	requestedBootstrap := bootstrap.SourceHead != "" || bootstrap.FrozenDigest != ""
	if previous.exists {
		if requestedBootstrap {
			return ratchetError(RatchetBootstrapForbidden)
		}
		if previous.active.SourceHead != OriginalSourceHead || firstExceptionGrowth(frozen, previous.active) != nil {
			return ratchetError(RatchetBindingMismatch)
		}
		if growth := firstExceptionGrowth(previous.active, active); growth != nil {
			return &RatchetError{code: RatchetGrowth, exception: growth, allowances: previous.active}
		}
		return nil
	}
	if !requestedBootstrap || previous.digest != EmptyPayloadDigest {
		return ratchetError(RatchetMissingInput)
	}
	if bootstrap.SourceHead != OriginalSourceHead || bootstrap.FrozenDigest != FrozenExceptionDigest {
		return ratchetError(RatchetBindingMismatch)
	}
	return nil
}

type eventRepository struct {
	FullName string `json:"full_name"`
}
type eventCommit struct {
	SHA string `json:"sha"`
}
type targetEvent struct {
	Repository  eventRepository `json:"repository"`
	Ref         string          `json:"ref"`
	Before      string          `json:"before"`
	After       string          `json:"after"`
	PullRequest struct {
		Base eventCommit `json:"base"`
		Head eventCommit `json:"head"`
	} `json:"pull_request"`
	MergeGroup struct {
		BaseSHA string `json:"base_sha"`
		HeadSHA string `json:"head_sha"`
	} `json:"merge_group"`
}

func selectTargetPredecessor(name, repository, ref, claimed string, event targetEvent) (string, error) {
	if repository != TargetRepository || event.Repository.FullName != TargetRepository || !validHex(claimed, 40) {
		return "", ratchetError(RatchetMissingInput)
	}
	var selected string
	switch name {
	case "pull_request":
		selected = event.PullRequest.Base.SHA
	case "merge_group":
		selected = event.MergeGroup.BaseSHA
	case "push":
		if ref != "refs/heads/main" || event.Ref != ref {
			return "", ratchetError(RatchetMissingInput)
		}
		selected = event.Before
	default:
		return "", ratchetError(RatchetMissingInput)
	}
	if !validHex(selected, 40) || claimed != selected {
		return "", ratchetError(RatchetMissingInput)
	}
	return selected, nil
}
