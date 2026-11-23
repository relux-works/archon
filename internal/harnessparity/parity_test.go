//go:build harness_parity

package harnessparity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/harnessregistry"
	"github.com/relux-works/curator/internal/testgitenv"
)

func TestMain(m *testing.M) {
	cleanup := testgitenv.Isolate()
	code := runParityProcess(m)
	cleanup()
	os.Exit(code)
}

type baselineManifest struct {
	SourceHead                  string            `json:"source_head"`
	Subject                     string            `json:"subject"`
	Platform                    string            `json:"platform"`
	CaptureDriverSHA256         string            `json:"capture_driver_sha256"`
	LauncherDriverSHA256        string            `json:"launcher_capture_driver_sha256"`
	Architecture                string            `json:"architecture"`
	GoVersion                   string            `json:"go_version"`
	CaptureSourceHead           string            `json:"capture_source_head,omitempty"`
	CaptureWorkflowHead         string            `json:"capture_workflow_head,omitempty"`
	CaptureWorkflowTree         string            `json:"capture_workflow_tree,omitempty"`
	CaptureArtifactSHA256       string            `json:"capture_artifact_sha256,omitempty"`
	CaptureSealSHA256           string            `json:"capture_seal_sha256,omitempty"`
	CaptureOutputsManifest      string            `json:"capture_outputs_manifest_sha256,omitempty"`
	CaptureOutputsManifestBytes []byte            `json:"capture_outputs_manifest_bytes,omitempty"`
	CaptureRepeatManifest       string            `json:"capture_repeat_manifest_sha256,omitempty"`
	CaptureRepeatManifestBytes  []byte            `json:"capture_repeat_manifest_bytes,omitempty"`
	CaptureRepeatRootBinding    string            `json:"capture_repeat_root_binding,omitempty"`
	CaptureRepeatResultSHA256   string            `json:"capture_repeat_result_sha256,omitempty"`
	CaptureSourceTree           string            `json:"capture_source_tree,omitempty"`
	CaptureWorkflowSHA256       string            `json:"capture_workflow_sha256,omitempty"`
	CaptureDriverPins           map[string]string `json:"capture_driver_pins,omitempty"`
	MetadataRole                string            `json:"metadata_role,omitempty"`
	CaptureCreationPolicy       string            `json:"capture_creation_policy,omitempty"`
	CaptureRunnerLabel          string            `json:"capture_runner_label,omitempty"`
	CaptureUmask                string            `json:"capture_umask,omitempty"`
	CaptureProbeDirectoryMode   string            `json:"capture_probe_directory_mode,omitempty"`
	CapturePayloadIdentity      string            `json:"capture_payload_identity,omitempty"`
	CaptureRedactedFields       int               `json:"capture_redacted_fields,omitempty"`
	CaptureRedactedValues       int               `json:"capture_redacted_values,omitempty"`
	Cases                       []struct {
		Name, SHA256 string
		Bytes        int
	} `json:"cases"`
	RepeatObservations []struct {
		Name, SHA256 string
		Bytes        int
	} `json:"repeat_observations,omitempty"`
	Corpora []struct {
		Path, SHA256 string
		Bytes        int
	} `json:"corpora"`
}

// Each pin seals a consumer index of an admitted original-source capture. It binds
// the complete provenance and all expected case/corpus hashes, not a render
// produced by the candidate. Missing platform evidence is an explicit failure.
var frozenManifestHashes = map[string]string{
	"linux":  "bc3cf3823d2b2308984d5663b7c9ae9017e5082ba42ba40ea766f3d78bda5848",
	"darwin": "f042708fa9ccff21771995ca73b5c8d11b04cb6f82c77e489c8e490c2871b976",
}

var frozenRefreshedManifestHashes = map[string]string{
	"linux":   "601d939606c9c6fb441ba0309b13f539e1d84d93e3314a3a7ffa4fccad22648b",
	"darwin":  "aa69ae09bbe74e3d5017e2ace0108f273902cdb3469de126a764926ebb7ada2c",
	"windows": "d62ca39126e6817897a27549b000820edf6ce7ebc3d18ed65d55c2d78fd5fa3b",
}

func verifyManifest(payload []byte, platform string) (baselineManifest, error) {
	path := filepath.Join("testdata", platform, "manifest.json")
	expected, known := frozenManifestHashes[platform]
	if !known {
		return baselineManifest{}, fmt.Errorf("%s: original_platform_manifest_missing", path)
	}
	if hashBytes(payload) != expected {
		return baselineManifest{}, fmt.Errorf("%s: frozen_manifest_mismatch", path)
	}
	var manifest baselineManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return baselineManifest{}, fmt.Errorf("%s: frozen_manifest_invalid", path)
	}
	if err := validateBaselineProvenance(manifest, platform); err != nil {
		return baselineManifest{}, err
	}
	return manifest, nil
}

func verifyRefreshedManifest(payload []byte, platform string) (baselineManifest, error) {
	path := filepath.Join("testdata", platform, "main-267bbf47", "manifest.json")
	expected, known := frozenRefreshedManifestHashes[platform]
	if !known {
		return baselineManifest{}, fmt.Errorf("%s: refreshed_platform_manifest_missing", path)
	}
	if hashBytes(payload) != expected {
		return baselineManifest{}, fmt.Errorf("%s: frozen_manifest_mismatch", path)
	}
	var manifest baselineManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return baselineManifest{}, fmt.Errorf("%s: frozen_manifest_invalid", path)
	}
	if err := validateRefreshedBaselineProvenance(manifest, platform); err != nil {
		return baselineManifest{}, err
	}
	return manifest, nil
}

func loadBaseline(t *testing.T, c fixtureCase) fixtureResult {
	t.Helper()
	return loadPlatformBaseline(t, c, runtime.GOOS)
}

func loadPlatformBaseline(t *testing.T, c fixtureCase, platform string) fixtureResult {
	t.Helper()
	base := filepath.Join("testdata", platform, "main-267bbf47")
	payload, err := os.ReadFile(filepath.Join(base, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest baselineManifest
	manifest, err = verifyRefreshedManifest(payload, platform)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SourceHead != refreshedSource || manifest.Subject != "unchanged legacy implementation" || manifest.CaptureDriverSHA256 == "" {
		t.Fatal("baseline provenance missing or substituted")
	}
	if manifest.Platform != platform {
		t.Fatal("baseline platform mismatch")
	}
	if len(manifest.Corpora) != 3 {
		t.Fatal("existing baseline corpora missing")
	}
	for _, corpus := range manifest.Corpora {
		bytes, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(corpus.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if len(bytes) != corpus.Bytes || hashBytes(bytes) != corpus.SHA256 {
			t.Fatalf("existing baseline corpus changed: %s", corpus.Path)
		}
	}
	var expectedHash string
	expectedSize := 0
	for _, entry := range manifest.Cases {
		if entry.Name == c.Name {
			if expectedHash != "" {
				t.Fatal("duplicate frozen case")
			}
			expectedHash = entry.SHA256
			expectedSize = entry.Bytes
		}
	}
	if expectedHash == "" {
		t.Fatalf("no frozen original case %s", c.Name)
	}
	bytes, err := os.ReadFile(filepath.Join(base, "cases", c.Name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if hashBytes(bytes) != expectedHash || len(bytes) != expectedSize {
		t.Fatalf("frozen case bytes changed: %s", c.Name)
	}
	var result fixtureResult
	if err := json.Unmarshal(bytes, &result); err != nil {
		t.Fatal(err)
	}
	if result.Case != c || !result.Executed {
		t.Fatal("baseline case not executed or mismatched")
	}
	for _, node := range result.Tree {
		if node.Kind == "file" && hashBytes(node.Bytes) != node.SHA256 || node.Kind == "link" && hashBytes([]byte(node.Link)) != node.SHA256 {
			t.Fatalf("baseline digest disagrees with bytes: %s", node.Path)
		}
	}
	return result
}

// Every full tree, output mode, link, notice and refusal is compared against a
// frozen original case. Only the explicit disposable-root parameter varies.
func TestHomeByteIdentity(t *testing.T) {
	t.Run("creation-policy-controls", testCreationPolicyControls)
	t.Run("provenance-controls", testCreationProvenanceControls)
	t.Run("native-creation-contract", testNativeCreationContract)
	t.Run("frozen-manifest-control", testFrozenManifestControl)
	for _, c := range allCases() {
		t.Run(c.Name, func(t *testing.T) {
			expected := laneBaseline(t, c)
			actual := countedCase(t, c)
			if err := compareResult(expected, actual); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The checked-in platform manifest passes; one changed provenance byte, still
// valid JSON, is refused at that path. An absent platform cannot reuse the
// Linux evidence. Neither control writes or updates an expected fixture.
func testFrozenManifestControl(t *testing.T) {
	path := filepath.Join("testdata", runtime.GOOS, "manifest.json")
	verify := verifyManifest
	missingPlatformCode := "original_platform_manifest_missing"
	if runtime.GOOS == "windows" {
		path = filepath.Join("testdata", "windows", "main-267bbf47", "manifest.json")
		verify = verifyRefreshedManifest
		missingPlatformCode = "refreshed_platform_manifest_missing"
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verify(payload, runtime.GOOS); err != nil {
		t.Fatal(err)
	}
	marker := []byte(`"launcher_capture_driver_sha256": "`)
	index := bytes.Index(payload, marker)
	if index < 0 || index+len(marker) >= len(payload) {
		t.Fatal("original launcher provenance witness missing")
	}
	altered := bytes.Clone(payload)
	index += len(marker)
	if altered[index] == 'a' {
		altered[index] = 'b'
	} else {
		altered[index] = 'a'
	}
	if !json.Valid(altered) || bytes.Equal(payload, altered) {
		t.Fatal("provenance mutation was not applied")
	}
	if _, err := verify(altered, runtime.GOOS); err == nil || err.Error() != path+": frozen_manifest_mismatch" {
		t.Fatalf("provenance mutation verdict: %v", err)
	}
	if _, err := verify(payload, "unrecorded-platform"); err == nil || !strings.Contains(err.Error(), missingPlatformCode) {
		t.Fatalf("missing platform verdict: %v", err)
	}
}

// Coverage is based on actual successful executions, never registry metadata or
// subtest names. An absent, skipped, refused-positive or unregistered fixture
// prevents the adapter coverage claim.
func TestAllAdaptersHaveBaselineCases(t *testing.T) {
	registered := map[string]bool{}
	for _, id := range harnessregistry.IDs() {
		registered[string(id)] = true
	}
	seen := map[string]bool{}
	for _, c := range allCases() {
		if !registered[c.ID] {
			t.Errorf("unregistered fixture %s", c.Name)
			continue
		}
		passed := t.Run(c.Name, func(t *testing.T) {
			defer func() {
				if t.Skipped() {
					t.Error("skipped baseline case cannot count as coverage")
				}
			}()
			expected := laneBaseline(t, c)
			actual := countedCase(t, c)
			if err := compareResult(expected, actual); err != nil {
				t.Fatal(err)
			}
			if c.State == "empty" && c.Operation == "managed" {
				if actual.Refusal != "" || len(actual.Tree) == 0 || !containsNode(actual, ".json") {
					t.Fatal("full managed baseline did not render")
				}
				seen[c.ID] = true
			}
			if c.Operation == "project-skills" {
				if actual.Refusal != "" || !containsNode(actual, "SKILL.md") {
					t.Fatal("skill projection did not execute")
				}
				seen[c.ID] = true
			}
			if c.Operation == "native-skills" {
				if actual.Refusal != "" || actual.Notice != "native discovery: no project mirror" || !onlyRoot(actual.Tree) {
					t.Fatal("native no-mirror case did not execute")
				}
				seen[c.ID] = true
			}
		})
		if !passed {
			delete(seen, c.ID)
		}
	}
	for id := range registered {
		if !seen[id] {
			t.Errorf("missing executed original baseline for %s", id)
		}
	}
}

// The same comparator accepts the original rendered candidate and rejects a
// single changed byte in the fixed Claude MCP file, naming path and both hashes.
func TestHomeByteIdentityMutationControl(t *testing.T) {
	t.Run("launcher-projection", testLauncherMutation)
	c := fixtureCase{Name: "claude_code-mcp", ID: "claude_code", Operation: "managed", State: "user-settings"}
	expected := laneBaseline(t, c)
	actual := countedCase(t, c)
	if err := compareResult(expected, actual); err != nil {
		t.Fatal(err)
	}
	if actual.Refusal != "" {
		t.Fatal("MCP positive case refused")
	}
	altered := actual
	altered.Rendered = append([]treeNode(nil), actual.Rendered...)
	var path, want, got string
	for i, node := range altered.Rendered {
		if node.Path == ".agent-context/mcp/claude_code.json" && node.Kind == "file" && len(node.Bytes) > 0 {
			path = node.Path
			want = node.SHA256
			altered.Rendered[i].Bytes = append([]byte(nil), node.Bytes...)
			altered.Rendered[i].Bytes[0] ^= 1
			got = hashBytes(altered.Rendered[i].Bytes)
			altered.Rendered[i].SHA256 = got
			break
		}
	}
	if path == "" {
		t.Fatal("MCP mutation was not applied")
	}
	err := compareResult(expected, altered)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), got) || want == got {
		t.Fatalf("mutation verdict: %v", err)
	}
}

// A launcher projection passes the same whole-tree oracle before a one-byte
// change is rejected with its exact path and differing digests.
func testLauncherMutation(t *testing.T) {
	c := fixtureCase{Name: "claude-launcher-typed-projection", ID: "claude_code", Operation: "launcher", State: "typed-projection"}
	expected := laneBaseline(t, c)
	actual := countedCase(t, c)
	if err := compareResult(expected, actual); err != nil {
		t.Fatal(err)
	}
	altered := actual
	altered.Tree = append([]treeNode(nil), actual.Tree...)
	var path, want, got string
	for i, node := range altered.Tree {
		if node.Path == "projection.json" && node.Kind == "file" && len(node.Bytes) > 0 {
			path = node.Path
			want = node.SHA256
			altered.Tree[i].Bytes = append([]byte(nil), node.Bytes...)
			altered.Tree[i].Bytes[0] ^= 1
			got = hashBytes(altered.Tree[i].Bytes)
			altered.Tree[i].SHA256 = got
			break
		}
	}
	if path == "" {
		t.Fatal("launcher mutation not applied")
	}
	err := compareResult(expected, altered)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), got) || want == got {
		t.Fatalf("launcher mutation verdict: %v", err)
	}
}
