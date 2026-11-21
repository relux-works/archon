package harnessboundary

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxPredecessorBytes = 8 << 20

type boundedGitOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedGitOutput) Write(data []byte) (int, error) {
	remaining := maxPredecessorBytes - b.Len()
	if len(data) > remaining {
		if remaining > 0 {
			_, _ = b.Buffer.Write(data[:remaining])
		}
		b.overflow = true
		return len(data), nil
	}
	return b.Buffer.Write(data)
}

type predecessorGit struct{ root string }

func (g predecessorGit) run(ctx context.Context, args ...string) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, "git", args...) // #nosec G204 -- private callers use fixed Git operations and validated SHA/path arguments.
	cmd.Dir = g.root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.WaitDelay = 100 * time.Millisecond
	var out boundedGitOutput
	cmd.Stdout = &out
	// Git diagnostics may contain account-specific URLs. Never expose them.
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil || out.overflow {
		return nil, &RatchetError{code: RatchetMissingInput, operation: args[0]}
	}
	return out.Bytes(), nil
}

func boundedFile(path string) ([]byte, error) {
	return boundedRootFile(filepath.Dir(path), filepath.Base(path))
}

func boundedRootFile(root, path string) ([]byte, error) {
	directory, err := os.OpenRoot(root)
	if err != nil {
		return nil, ratchetError(RatchetMissingInput)
	}
	defer func() { _ = directory.Close() }()
	f, err := directory.Open(path)
	if err != nil {
		return nil, ratchetError(RatchetMissingInput)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, ratchetError(RatchetMissingInput)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxPredecessorBytes+1))
	if err != nil || len(data) > maxPredecessorBytes {
		return nil, ratchetError(RatchetMissingInput)
	}
	return data, nil
}

func decodeBoundedJSON(data []byte, destination any) error {
	if len(data) > maxPredecessorBytes {
		return ratchetError(RatchetBindingMismatch)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(destination) != nil {
		return ratchetError(RatchetBindingMismatch)
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return ratchetError(RatchetBindingMismatch)
	}
	return nil
}

func readCurrentExceptionSets(root string) (ExceptionSet, ExceptionSet, error) {
	frozenBytes, err := boundedRootFile(root, FrozenExceptionPath)
	if err != nil {
		return ExceptionSet{}, ExceptionSet{}, err
	}
	if digest(frozenBytes) != FrozenExceptionDigest {
		return ExceptionSet{}, ExceptionSet{}, ratchetError(RatchetBindingMismatch)
	}
	activeBytes, err := boundedRootFile(root, ActiveExceptionPath)
	if err != nil {
		return ExceptionSet{}, ExceptionSet{}, err
	}
	var frozen, active ExceptionSet
	if err = decodeBoundedJSON(frozenBytes, &frozen); err != nil {
		return frozen, active, err
	}
	if err = decodeBoundedJSON(activeBytes, &active); err != nil {
		return frozen, active, err
	}
	return frozen, active, nil
}

func materializePredecessor(ctx context.Context, root, sha, expectedDigest, expectedHead string, freshDefault bool) (VerifiedPredecessor, error) {
	if !validHex(sha, 40) || expectedDigest != "" && !validHex(expectedDigest, 64) {
		return VerifiedPredecessor{}, ratchetError(RatchetMissingInput)
	}
	g := predecessorGit{root}
	if freshDefault {
		if expectedDigest == "" {
			return VerifiedPredecessor{}, ratchetError(RatchetMissingInput)
		}
		observation, err := g.run(ctx, "ls-remote", "--exit-code", "origin", "refs/heads/main")
		if err != nil {
			return VerifiedPredecessor{}, err
		}
		fields := strings.Fields(string(observation))
		if len(fields) != 2 || fields[0] != sha || fields[1] != "refs/heads/main" {
			return VerifiedPredecessor{}, ratchetError(RatchetBindingMismatch)
		}
	}
	headBytes, err := g.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return VerifiedPredecessor{}, err
	}
	head := strings.TrimSpace(string(headBytes))
	if !validHex(head, 40) || expectedHead != "" && (head != expectedHead || sha == head) {
		return VerifiedPredecessor{}, ratchetError(RatchetMissingInput)
	}
	if _, err = g.run(ctx, "fetch", "--no-tags", "--depth=1", "origin", sha); err != nil {
		return VerifiedPredecessor{}, err
	}
	// checkout's default shallow history cannot establish ancestry. Obtain the
	// actual checked-out head's history, never another predecessor or branch.
	shallow, err := g.run(ctx, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return VerifiedPredecessor{}, err
	}
	if strings.TrimSpace(string(shallow)) == "true" {
		if _, err = g.run(ctx, "fetch", "--no-tags", "--unshallow", "origin", head); err != nil {
			return VerifiedPredecessor{}, err
		}
	}
	if _, err = g.run(ctx, "merge-base", "--is-ancestor", sha, head); err != nil {
		return VerifiedPredecessor{}, err
	}
	if _, err = g.run(ctx, "cat-file", "-e", sha+"^{commit}"); err != nil {
		return VerifiedPredecessor{}, err
	}
	listing, err := g.run(ctx, "ls-tree", "-z", sha, "--", ActiveExceptionPath)
	if err != nil {
		return VerifiedPredecessor{}, err
	}
	previous := VerifiedPredecessor{sha: sha, verified: true}
	var payload []byte
	if len(listing) != 0 {
		if bytes.Count(listing, []byte{0}) != 1 {
			return VerifiedPredecessor{}, ratchetError(RatchetBindingMismatch)
		}
		row := strings.SplitN(strings.TrimSuffix(string(listing), "\x00"), "\t", 2)
		if len(row) != 2 || row[1] != ActiveExceptionPath {
			return VerifiedPredecessor{}, ratchetError(RatchetBindingMismatch)
		}
		metadata := strings.Fields(row[0])
		if len(metadata) != 3 || metadata[0] != "100644" || metadata[1] != "blob" {
			return VerifiedPredecessor{}, ratchetError(RatchetBindingMismatch)
		}
		payload, err = g.run(ctx, "show", sha+":"+ActiveExceptionPath)
		if err != nil {
			return VerifiedPredecessor{}, err
		}
		previous.exists = true
		if err = decodeBoundedJSON(payload, &previous.active); err != nil {
			return VerifiedPredecessor{}, err
		}
	}
	previous.digest = digest(payload)
	if expectedDigest != "" && previous.digest != expectedDigest {
		return VerifiedPredecessor{}, ratchetError(RatchetBindingMismatch)
	}
	return previous, nil
}

// archiveDescriptor is admitted only with independently supplied SHA, payload
// digest and descriptor-byte digest. Absence is explicit, never an empty file.
type archiveDescriptor struct {
	Version int    `json:"version"`
	SHA     string `json:"predecessor_sha"`
	Digest  string `json:"predecessor_digest"`
	Exists  bool   `json:"active_exists"`
	Active  []byte `json:"active_payload"`
}

func readArchivePredecessor(path, descriptorDigest, sha, expectedDigest string) (VerifiedPredecessor, error) {
	if path == "" || !validHex(descriptorDigest, 64) || !validHex(sha, 40) || !validHex(expectedDigest, 64) {
		return VerifiedPredecessor{}, ratchetError(RatchetMissingInput)
	}
	data, err := boundedFile(path)
	if err != nil {
		return VerifiedPredecessor{}, err
	}
	if digest(data) != descriptorDigest {
		return VerifiedPredecessor{}, ratchetError(RatchetBindingMismatch)
	}
	var descriptor archiveDescriptor
	if err = decodeBoundedJSON(data, &descriptor); err != nil {
		return VerifiedPredecessor{}, err
	}
	if descriptor.Version != 1 || descriptor.SHA != sha || descriptor.Digest != expectedDigest {
		return VerifiedPredecessor{}, ratchetError(RatchetBindingMismatch)
	}
	previous := VerifiedPredecessor{sha: sha, digest: expectedDigest, exists: descriptor.Exists, verified: true}
	if descriptor.Exists {
		if digest(descriptor.Active) != expectedDigest {
			return VerifiedPredecessor{}, ratchetError(RatchetBindingMismatch)
		}
		if err = decodeBoundedJSON(descriptor.Active, &previous.active); err != nil {
			return VerifiedPredecessor{}, err
		}
	} else if len(descriptor.Active) != 0 || expectedDigest != EmptyPayloadDigest {
		return VerifiedPredecessor{}, ratchetError(RatchetBindingMismatch)
	}
	return previous, nil
}
