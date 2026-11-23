// Package main implements the capture-source command used by the native capture workflow.
package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	captureSourceRemote = "https://github.com/relux-works/curator.git"
	workflowArchivePath = ".github/workflows/original-capture-native.yml"
	fixtureArchiveRoot  = "testdata/originalcapture/"
	legacyRegularType   = byte(0) // Legacy tar TypeRegA spelling remains accepted.
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: capture-source <materialize|verify-tests> [flags]")
	}
	switch args[0] {
	case "materialize":
		return runMaterialize(args[1:])
	case "verify-tests":
		return runVerifyTests(args[1:])
	default:
		return fmt.Errorf("unknown capture-source command %q", args[0])
	}
}

func runMaterialize(args []string) error {
	flags := flag.NewFlagSet("materialize", flag.ContinueOnError)
	revision := flags.String("revision", "", "full source commit SHA")
	remote := flags.String("remote", "", "Git repository URL")
	destination := flags.String("destination", "", "ignored destination directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if !isFullCommitSHA(*revision) {
		return errors.New("revision must be a full 40-character lowercase commit SHA")
	}
	if strings.TrimSpace(*remote) == "" {
		return errors.New("remote is required")
	}
	if !isAllowedCaptureRemote(*remote) {
		return fmt.Errorf("remote must be the accepted capture source %q", captureSourceRemote)
	}
	if strings.TrimSpace(*destination) == "" {
		return errors.New("destination is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if err := materialize(ctx, *remote, *revision, *destination); err != nil {
		return err
	}
	fmt.Printf("capture source %s is available at %s\n", *revision, *destination)
	return nil
}

func materialize(ctx context.Context, remote, revision, destination string) (resultErr error) {
	if !isFullCommitSHA(revision) {
		return errors.New("revision must be a full 40-character lowercase commit SHA")
	}
	absoluteDestination, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	parent := filepath.Dir(absoluteDestination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create destination parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".capture-source-stage-")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(stage); cleanupErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove capture source staging directory: %w", cleanupErr))
		}
	}()

	bareRepo := filepath.Join(stage, "source.git")
	if _, err := runGit(ctx, "", "init", "--bare", "--quiet", bareRepo); err != nil {
		return err
	}
	if _, err := runGit(ctx, bareRepo, "fetch", "--quiet", "--depth=1", remote, revision); err != nil {
		return err
	}
	if err := verifyFetchedCommit(ctx, bareRepo, revision); err != nil {
		return err
	}

	extracted := filepath.Join(stage, "tree")
	if err := os.Mkdir(extracted, 0o755); err != nil {
		return fmt.Errorf("create extraction directory: %w", err)
	}
	if err := extractRequestedArchive(ctx, bareRepo, revision, extracted); err != nil {
		return err
	}
	if err := verifyFetchedCommit(ctx, bareRepo, revision); err != nil {
		return err
	}

	if _, err := os.Lstat(absoluteDestination); err == nil {
		if err := compareTrees(extracted, absoluteDestination); err != nil {
			return fmt.Errorf("existing capture source differs from fetched commit; preserved %s: %w", absoluteDestination, err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect destination: %w", err)
	}
	if err := os.Rename(extracted, absoluteDestination); err != nil {
		return fmt.Errorf("publish extracted capture source: %w", err)
	}
	return nil
}

func verifyFetchedCommit(ctx context.Context, bareRepo, expected string) error {
	got, err := runGit(ctx, bareRepo, "rev-parse", "FETCH_HEAD^{commit}")
	if err != nil {
		return err
	}
	if got != expected {
		return fmt.Errorf("fetched commit mismatch: got %s, want %s", got, expected)
	}
	return nil
}

func extractRequestedArchive(ctx context.Context, bareRepo, revision, destination string) error {
	if !filepath.IsAbs(bareRepo) || !isFullCommitSHA(revision) || !filepath.IsAbs(destination) {
		return errors.New("capture archive inputs are invalid")
	}
	args := []string{"archive", "--format=tar", revision, workflowArchivePath, fixtureArchiveRoot}
	// #nosec G204 -- git is fixed; revision is a full SHA and archive paths are constants.
	cmd := exec.CommandContext(ctx, "git", append([]string{"--git-dir=" + bareRepo}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open git archive output: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start git archive: %w", err)
	}
	workflowSeen, fixtureFiles, extractErr := extractTar(stdout, destination)
	if extractErr != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if extractErr != nil {
		return fmt.Errorf("extract selected git archive paths: %w", extractErr)
	}
	if waitErr != nil {
		return fmt.Errorf("git archive failed: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	if !workflowSeen {
		return fmt.Errorf("git archive did not contain %s", workflowArchivePath)
	}
	if fixtureFiles == 0 {
		return fmt.Errorf("git archive did not contain files below %s", fixtureArchiveRoot)
	}
	return nil
}

func extractTar(reader io.Reader, destination string) (bool, int, error) {
	archive := tar.NewReader(reader)
	seen := make(map[string]struct{})
	workflowSeen := false
	fixtureFiles := 0
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return workflowSeen, fixtureFiles, nil
		}
		if err != nil {
			return false, 0, fmt.Errorf("read archive entry: %w", err)
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			if header.Name != "pax_global_header" {
				return false, 0, fmt.Errorf("unexpected global archive header %q", header.Name)
			}
			continue
		}
		name, err := cleanArchiveName(header.Name)
		if err != nil {
			return false, 0, err
		}
		if _, duplicate := seen[name]; duplicate {
			return false, 0, fmt.Errorf("duplicate archive path %q", name)
		}
		seen[name] = struct{}{}
		fullPath := filepath.Join(destination, filepath.FromSlash(name))
		switch header.Typeflag {
		case tar.TypeDir:
			if !isAllowedArchiveDirectory(name) {
				return false, 0, fmt.Errorf("unexpected archive directory %q", name)
			}
			if err := os.MkdirAll(fullPath, 0o755); err != nil {
				return false, 0, fmt.Errorf("create archive directory %q: %w", name, err)
			}
		case tar.TypeReg, legacyRegularType:
			if !isAllowedArchiveFile(name) {
				return false, 0, fmt.Errorf("unexpected archive file %q", name)
			}
			if header.Size < 0 {
				return false, 0, fmt.Errorf("negative size for archive file %q", name)
			}
			if header.Mode < 0 || header.Mode > 0o777 {
				return false, 0, fmt.Errorf("unsupported archive file mode %d for %q", header.Mode, name)
			}
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return false, 0, fmt.Errorf("create parent for archive file %q: %w", name, err)
			}
			// #nosec G115 -- the signed archive mode was bounded to permission bits above.
			mode := fs.FileMode(header.Mode)
			if mode == 0 {
				mode = 0o644
			}
			file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode) // #nosec G304 -- cleanArchiveName and the positive archive path allowlist confine this to the fresh extraction root.
			if err != nil {
				return false, 0, fmt.Errorf("create archive file %q: %w", name, err)
			}
			_, copyErr := io.CopyN(file, archive, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return false, 0, fmt.Errorf("write archive file %q: %w", name, copyErr)
			}
			if closeErr != nil {
				return false, 0, fmt.Errorf("close archive file %q: %w", name, closeErr)
			}
			if err := os.Chmod(fullPath, mode); err != nil {
				return false, 0, fmt.Errorf("set archive file mode %q: %w", name, err)
			}
			if name == workflowArchivePath {
				workflowSeen = true
			}
			if strings.HasPrefix(name, fixtureArchiveRoot) {
				fixtureFiles++
			}
		default:
			return false, 0, fmt.Errorf("unsupported archive entry type %d for %q", header.Typeflag, name)
		}
	}
}

func cleanArchiveName(raw string) (string, error) {
	if raw == "" || strings.Contains(raw, "\\") || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("unsafe archive path %q", raw)
	}
	name := strings.TrimSuffix(raw, "/")
	if name == "" || path.Clean(name) != name {
		return "", fmt.Errorf("unsafe archive path %q", raw)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe archive path %q", raw)
		}
	}
	return name, nil
}

func isAllowedArchiveDirectory(name string) bool {
	switch name {
	case ".github", ".github/workflows", "testdata", "testdata/originalcapture":
		return true
	default:
		return strings.HasPrefix(name, "testdata/originalcapture/")
	}
}

func isAllowedArchiveFile(name string) bool {
	if _, err := cleanArchiveName(name); err != nil {
		return false
	}
	return name == workflowArchivePath || strings.HasPrefix(name, fixtureArchiveRoot)
}

func compareTrees(expected, actual string) error {
	expectedEntries, err := snapshotTree(expected)
	if err != nil {
		return fmt.Errorf("read fetched tree: %w", err)
	}
	actualEntries, err := snapshotTree(actual)
	if err != nil {
		return fmt.Errorf("read existing tree: %w", err)
	}
	paths := make([]string, 0, len(expectedEntries)+len(actualEntries))
	seen := make(map[string]struct{}, len(expectedEntries)+len(actualEntries))
	for name := range expectedEntries {
		seen[name] = struct{}{}
	}
	for name := range actualEntries {
		seen[name] = struct{}{}
	}
	for name := range seen {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	for _, name := range paths {
		want, wantOK := expectedEntries[name]
		got, gotOK := actualEntries[name]
		if !wantOK || !gotOK {
			return fmt.Errorf("path set differs at %s", name)
		}
		if want.isDir != got.isDir || want.mode != got.mode || !bytes.Equal(want.data, got.data) {
			return fmt.Errorf("content or mode differs at %s", name)
		}
	}
	return nil
}

type treeEntry struct {
	isDir bool
	mode  fs.FileMode
	data  []byte
}

func snapshotTree(root string) (map[string]treeEntry, error) {
	entries := make(map[string]treeEntry)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == root {
			if !entry.IsDir() {
				return fmt.Errorf("tree root is not a directory")
			}
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		info, err := os.Lstat(name)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlink in tree at %s", filepath.ToSlash(relative))
		}
		mode := info.Mode().Perm()
		item := treeEntry{isDir: info.IsDir(), mode: mode}
		if !item.isDir {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported file type in tree at %s", filepath.ToSlash(relative))
			}
			item.data, err = os.ReadFile(name) // #nosec G304 -- name is a regular entry from the non-following walk rooted at the selected tree; symlinks are rejected above.
			if err != nil {
				return err
			}
		}
		entries[filepath.ToSlash(relative)] = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func runGit(ctx context.Context, bareRepo string, args ...string) (string, error) {
	if !allowedGitInvocation(bareRepo, args) {
		return "", errors.New("git invocation is not allowed")
	}
	argv := make([]string, 0, len(args)+1)
	if bareRepo != "" {
		argv = append(argv, "--git-dir="+bareRepo)
	}
	argv = append(argv, args...)
	// #nosec G204 -- allowedGitInvocation admits only fixed Git operations and validated repository, remote, and revision inputs.
	cmd := exec.CommandContext(ctx, "git", argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func allowedGitInvocation(bareRepo string, args []string) bool {
	if len(args) == 4 && bareRepo == "" && args[0] == "init" && args[1] == "--bare" && args[2] == "--quiet" {
		return filepath.IsAbs(args[3])
	}
	if len(args) == 5 && filepath.IsAbs(bareRepo) && args[0] == "fetch" && args[1] == "--quiet" && args[2] == "--depth=1" {
		return (isAllowedCaptureRemote(args[3]) || filepath.IsAbs(args[3])) && isFullCommitSHA(args[4])
	}
	return len(args) == 2 && filepath.IsAbs(bareRepo) && args[0] == "rev-parse" && args[1] == "FETCH_HEAD^{commit}"
}

func isFullCommitSHA(value string) bool {
	if len(value) != 40 || strings.ToLower(value) != value {
		return false
	}
	for _, char := range value {
		decimal := char >= '0' && char <= '9'
		lowerHex := char >= 'a' && char <= 'f'
		if !decimal && !lowerHex {
			return false
		}
	}
	return true
}

func isAllowedCaptureRemote(remote string) bool {
	return remote == captureSourceRemote
}

type testEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

func runVerifyTests(args []string) error {
	flags := flag.NewFlagSet("verify-tests", flag.ContinueOnError)
	goBinary := flags.String("go", "go", "Go executable")
	packagePath := flags.String("package", "./internal/harnessparity/", "package passed to go test")
	var required stringList
	flags.Var(&required, "required", "test name that must pass; may be repeated")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if len(required) == 0 {
		return errors.New("at least one --required test name is needed")
	}
	if !isAllowedVerifyPackage(*packagePath) {
		return errors.New("package must be the native capture parity test package")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	return verifyGoTests(ctx, *goBinary, *packagePath, required, os.Stdout)
}

type stringList []string

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("required test name cannot be empty")
	}
	*values = append(*values, value)
	return nil
}

func verifyGoTests(ctx context.Context, goBinary, packagePath string, required []string, output io.Writer) error {
	if !isAllowedGoExecutable(goBinary) {
		return errors.New("go executable must be go or an absolute path")
	}
	if !isAllowedVerifyPackage(packagePath) {
		return errors.New("package must be the native capture parity test package")
	}
	if len(required) == 0 {
		return errors.New("at least one required test name is needed")
	}
	// #nosec G204 -- executable and package are allowlisted; all other arguments are fixed, and no shell is invoked.
	cmd := exec.CommandContext(ctx, goBinary, "test", "-json", "-count=1", packagePath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open go test output: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start go test: %w", err)
	}
	verifyErr := verifyTestEvents(stdout, required, output)
	if verifyErr != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if verifyErr != nil {
		return verifyErr
	}
	if waitErr != nil {
		return fmt.Errorf("go test failed: %w", waitErr)
	}
	return nil
}

func isAllowedGoExecutable(value string) bool {
	return value == "go"
}

func isAllowedVerifyPackage(value string) bool {
	return value == "./internal/harnessparity" || value == "./internal/harnessparity/"
}

func verifyTestEvents(reader io.Reader, required []string, output io.Writer) error {
	wanted := make(map[string]struct{}, len(required))
	for _, name := range required {
		if strings.TrimSpace(name) == "" {
			return errors.New("required test name cannot be empty")
		}
		if _, duplicate := wanted[name]; duplicate {
			return fmt.Errorf("duplicate required test name %q", name)
		}
		wanted[name] = struct{}{}
	}
	states := make(map[string]string, len(wanted))
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if _, err := fmt.Fprintln(output, string(line)); err != nil {
			return fmt.Errorf("write go test output: %w", err)
		}
		var event testEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return fmt.Errorf("parse go test JSON event: %w", err)
		}
		if _, required := wanted[event.Test]; required && (event.Action == "pass" || event.Action == "skip" || event.Action == "fail") {
			states[event.Test] = event.Action
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read go test output: %w", err)
	}
	missing := make([]string, 0)
	for name := range wanted {
		if states[name] != "pass" {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		return fmt.Errorf("required tests did not pass: %s", strings.Join(missing, ", "))
	}
	return nil
}
