package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializeFetchesOnlyRequestedPathsAndIsIdempotent(t *testing.T) {
	remote, revision := newCaptureSourceRemote(t)
	destination := filepath.Join(t.TempDir(), "capture-source")
	if err := materialize(context.Background(), remote, revision, destination); err != nil {
		t.Fatalf("materialize source: %v", err)
	}
	assertFileContents(t, filepath.Join(destination, workflowArchivePath), "workflow source\n")
	assertFileContents(t, filepath.Join(destination, "testdata", "originalcapture", "capture_test.go"), "fixture source\n")
	if _, err := os.Stat(filepath.Join(destination, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("unrequested README was materialized: %v", err)
	}
	if err := materialize(context.Background(), remote, revision, destination); err != nil {
		t.Fatalf("repeat materialization: %v", err)
	}
}

func TestMaterializePreservesDifferentExistingDestination(t *testing.T) {
	remote, revision := newCaptureSourceRemote(t)
	destination := filepath.Join(t.TempDir(), "capture-source")
	if err := materialize(context.Background(), remote, revision, destination); err != nil {
		t.Fatalf("materialize source: %v", err)
	}
	changed := []byte("locally changed workflow\n")
	if err := os.WriteFile(filepath.Join(destination, workflowArchivePath), changed, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := materialize(context.Background(), remote, revision, destination); err == nil {
		t.Fatal("different existing destination was accepted")
	}
	got, err := os.ReadFile(filepath.Join(destination, workflowArchivePath))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, changed) {
		t.Fatalf("existing destination changed after refusal: %q", got)
	}
}

func TestMaterializeRequiresAFullCommitSHA(t *testing.T) {
	err := materialize(context.Background(), "unused", "44ff5d16", filepath.Join(t.TempDir(), "capture-source"))
	if err == nil || !strings.Contains(err.Error(), "full 40-character") {
		t.Fatalf("short revision error = %v", err)
	}
}

func TestMaterializeRemoteAllowlist(t *testing.T) {
	for _, test := range []struct {
		name   string
		remote string
		valid  bool
	}{
		{name: "accepted HTTPS repository", remote: captureSourceRemote, valid: true},
		{name: "different repository", remote: "https://github.com/other/curator.git"},
		{name: "lookalike host", remote: "https://github.com.attacker.invalid/relux-works/curator.git"},
		{name: "SSH form", remote: "git@github.com:relux-works/curator.git"},
		{name: "local file remote", remote: "file:///tmp/curator.git"},
		{name: "local path remote", remote: "/tmp/curator.git"},
		{name: "case variant", remote: "https://GITHUB.com/relux-works/curator.git"},
		{name: "trailing slash variant", remote: "https://github.com/relux-works/curator.git/"},
	} {
		t.Run(test.name, func(t *testing.T) {
			valid := isAllowedCaptureRemote(test.remote)
			if valid != test.valid {
				t.Fatalf("remote %q valid = %t, want %t", test.remote, valid, test.valid)
			}
		})
	}
}

func TestMaterializeCommandRejectsUnapprovedRemoteBeforeFetch(t *testing.T) {
	remote, revision := newCaptureSourceRemote(t)
	destination := filepath.Join(t.TempDir(), "capture-source")
	err := runMaterialize([]string{
		"--remote", remote,
		"--revision", revision,
		"--destination", destination,
	})
	if err == nil || !strings.Contains(err.Error(), "accepted capture source") {
		t.Fatalf("unapproved remote error = %v", err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination was created before rejecting the remote: %v", err)
	}
}

func TestArchivePathsUseAnExplicitAllowlist(t *testing.T) {
	for _, valid := range []string{
		workflowArchivePath,
		"testdata/originalcapture/capture_test.go",
	} {
		if !isAllowedArchiveFile(valid) {
			t.Errorf("expected archive file %q to be allowed", valid)
		}
	}
	for _, invalid := range []string{
		"Makefile",
		"testdata/other/fixture.go",
		"testdata/originalcapture/../outside.go",
	} {
		if isAllowedArchiveFile(invalid) {
			t.Errorf("unexpected archive file %q was allowed", invalid)
		}
	}
	for _, invalid := range []string{"../outside", "/absolute/path", `testdata\outside`} {
		if _, err := cleanArchiveName(invalid); err == nil {
			t.Errorf("unsafe archive path %q was accepted", invalid)
		}
	}
}

func TestAllowedGitInvocationHasBoundedShapes(t *testing.T) {
	root := t.TempDir()
	bare := filepath.Join(root, "source.git")
	remote := filepath.Join(root, "remote.git")
	revision := strings.Repeat("a", 40)
	for _, row := range []struct {
		name, bare string
		args       []string
	}{
		{"init", "", []string{"init", "--bare", "--quiet", bare}},
		{"accepted-remote-fetch", bare, []string{"fetch", "--quiet", "--depth=1", captureSourceRemote, revision}},
		{"local-fixture-fetch", bare, []string{"fetch", "--quiet", "--depth=1", remote, revision}},
		{"fetch-head-verification", bare, []string{"rev-parse", "FETCH_HEAD^{commit}"}},
	} {
		t.Run(row.name, func(t *testing.T) {
			if !allowedGitInvocation(row.bare, row.args) {
				t.Fatal("supported Git invocation refused")
			}
		})
	}
	for _, row := range []struct {
		name, bare string
		args       []string
	}{
		{"extra-fetch-option", bare, []string{"fetch", "--quiet", "--depth=1", "--upload-pack=sh", captureSourceRemote, revision}},
		{"unapproved-remote", bare, []string{"fetch", "--quiet", "--depth=1", "https://example.invalid/repo.git", revision}},
		{"short-revision", bare, []string{"fetch", "--quiet", "--depth=1", captureSourceRemote, "deadbeef"}},
		{"unknown-subcommand", bare, []string{"config", "--global", "core.editor=sh"}},
		{"relative-bare-repository", "relative.git", []string{"rev-parse", "FETCH_HEAD^{commit}"}},
	} {
		t.Run(row.name, func(t *testing.T) {
			if allowedGitInvocation(row.bare, row.args) {
				t.Fatal("unlisted Git invocation accepted")
			}
		})
	}
}

func TestVerifyTestCommandInputsUsePositiveAllowlists(t *testing.T) {
	for _, executable := range []string{"go"} {
		if !isAllowedGoExecutable(executable) {
			t.Errorf("supported Go executable %q refused", executable)
		}
	}
	for _, executable := range []string{"", "./go", filepath.Join(t.TempDir(), "go"), "go;touch marker", "go test"} {
		if isAllowedGoExecutable(executable) {
			t.Errorf("unlisted Go executable %q accepted", executable)
		}
	}
	for _, packagePath := range []string{"./internal/harnessparity", "./internal/harnessparity/"} {
		if !isAllowedVerifyPackage(packagePath) {
			t.Errorf("supported package %q refused", packagePath)
		}
	}
	for _, packagePath := range []string{"", "-exec=sh", "./internal/other", "./internal/harnessparity/;touch marker"} {
		if isAllowedVerifyPackage(packagePath) {
			t.Errorf("unlisted package %q accepted", packagePath)
		}
	}
}

func TestExtractTarPreservesRegularTypeFormsAndRejectsModeOverflow(t *testing.T) {
	for _, row := range []struct {
		name     string
		typeflag byte
		mode     int64
		wantErr  bool
	}{
		{"ustar-regular", tar.TypeReg, 0o644, false},
		{"legacy-nul-regular-default-mode", legacyRegularType, 0, false},
		{"file-type-bits-outside-permissions", tar.TypeReg, 0o100644, true},
		{"negative-mode", tar.TypeReg, -1, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := tar.NewWriter(&archive)
			data := []byte("workflow source\n")
			if err := writer.WriteHeader(&tar.Header{Name: workflowArchivePath, Typeflag: row.typeflag, Mode: row.mode, Size: int64(len(data))}); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Write(data); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			destination := t.TempDir()
			workflowSeen, _, err := extractTar(bytes.NewReader(archive.Bytes()), destination)
			if row.wantErr {
				if err == nil || !strings.Contains(err.Error(), "unsupported archive file mode") {
					t.Fatalf("invalid archive mode verdict = %v", err)
				}
				if _, statErr := os.Lstat(filepath.Join(destination, workflowArchivePath)); !os.IsNotExist(statErr) {
					t.Fatalf("invalid mode published workflow file: %v", statErr)
				}
				return
			}
			if err != nil || !workflowSeen {
				t.Fatalf("supported regular archive form refused: workflow=%t err=%v", workflowSeen, err)
			}
		})
	}
}

func TestVerifyTestEventsRequiresEveryNamedTestToPass(t *testing.T) {
	required := []string{"TestUploadBound", "TestMissingBindingFailsClosed"}
	var stream strings.Builder
	for _, event := range []testEvent{
		{Action: "run", Test: "TestUploadBound"},
		{Action: "pass", Test: "TestUploadBound"},
		{Action: "pass", Test: "TestMissingBindingFailsClosed"},
	} {
		line, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		stream.Write(line)
		stream.WriteByte('\n')
	}
	var output bytes.Buffer
	if err := verifyTestEvents(strings.NewReader(stream.String()), required, &output); err != nil {
		t.Fatalf("passing events rejected: %v", err)
	}
	if output.Len() == 0 {
		t.Fatal("go test output was not forwarded")
	}

	for name, action := range map[string]string{
		"missing": "",
		"skipped": "skip",
		"failed":  "fail",
	} {
		t.Run(name, func(t *testing.T) {
			var events strings.Builder
			for _, requiredName := range required {
				currentAction := "pass"
				if requiredName == "TestUploadBound" {
					currentAction = action
				}
				if currentAction == "" {
					continue
				}
				line, err := json.Marshal(testEvent{Action: currentAction, Test: requiredName})
				if err != nil {
					t.Fatal(err)
				}
				events.Write(line)
				events.WriteByte('\n')
			}
			if err := verifyTestEvents(strings.NewReader(events.String()), required, &bytes.Buffer{}); err == nil {
				t.Fatalf("%s required test was accepted", name)
			}
		})
	}
}

func TestVerifyTestEventsRejectsDuplicateRequirements(t *testing.T) {
	err := verifyTestEvents(strings.NewReader(""), []string{"TestA", "TestA"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate requirement error = %v", err)
	}
}

func newCaptureSourceRemote(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(work, filepath.Dir(workflowArchivePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, "testdata", "originalcapture"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(work, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(workflowArchivePath, "workflow source\n")
	write(filepath.Join("testdata", "originalcapture", "capture_test.go"), "fixture source\n")
	write("README.md", "must not be archived\n")
	runTestGit(t, work, "init", "--quiet")
	runTestGit(t, work, "add", "--", ".")
	runTestGit(t, work, "-c", "user.name=Capture Source Test", "-c", "user.email=capture-source@example.invalid", "commit", "--quiet", "-m", "source")
	revision := runTestGit(t, work, "rev-parse", "HEAD")
	remote := filepath.Join(root, "remote.git")
	runTestGit(t, root, "clone", "--bare", "--quiet", work, remote)
	return remote, revision
}

func runTestGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
