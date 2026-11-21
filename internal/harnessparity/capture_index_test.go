//go:build harness_parity

package harnessparity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const macCaptureSource = "805517ccc4bc77614e9ccb3e35e47876f054f629"
const macCaptureTree = "6a5cd21bbe028a278d5fcfc3f247434781674e99"
const macCaptureWorkflow = "56d60322f9f5d0258de12976e02ba39906e3cd256c96f1c86ca517fff5230625"
const linux07WorkflowHead = "81ee60f3a6897dd170d4065d414344b48805a51b"
const linux07WorkflowTree = "19dea7a4706f97b2bcde82e6798cdd12eb6c1506"
const linux07ArtifactSHA256 = "9525a6322a6e2fe20b06c75045f6de1ae5702a5b779d8311d8696219244aeab0"
const linux07SealSHA256 = "e861e89338861595484ba7310344f726060b13dd20c51ea5c0c896a5d556c831"
const linux07OutputsManifestSHA256 = "6cd55b28de2552e71878eb433cf70714c3e3e9e8f8c63fa7e93f6f90aefb346f"
const linux07RepeatManifestSHA256 = "e435fa496072dfca3133886693e776a9d2e4d2c4790e624db844b02d5d265da1"
const linux07RepeatResultSHA256 = "b8e48867c85314b5d34bb13065fe92716b636e30a5d07e7a2a43be8549ca6b2b"
const linux07ManifestSHA256 = "751a6d6adc9478570ad6ac154909eac127670bdfbcdfeadbf9e6020adf368b13"

var historicalLinuxCapturePolicy = capturePolicyPins{
	driver:   "4385a22ae940d8b144c1d848b1098b39af6cf441c72da6deca1ee3f4da2700f9",
	launcher: "ae6a3977913733def320f7d4975c68052659f72b3ac6c7ba68837964bbed1e8e",
	arch:     "amd64", goVersion: "go1.26.8", linuxDeclaration: "declared by run.sh, not attested",
}

var macCaptureDrivers = map[string]string{
	"testdata/originalcapture/capture_homes_test.go":     "35457d6728df2436292d85702621318d33b420b7d8aafd891cc96421be8d293b",
	"testdata/originalcapture/capture_launchers_test.go": "e202ab658c45b6ebaba731c2a72eda82de4cb94583b251d0f73054361e124518",
	"testdata/originalcapture/controls_test.go":          "2928b6a0001dccfd39f430caa93ced8528b23653f4b3542103639eef09b908e4",
	"testdata/originalcapture/launcher_fixture_test.go":  "deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983",
	"testdata/originalcapture/legacy_fixture_test.go":    "e9250a474e897a0d8aa9e0ee6f01afd0a42d8532ae6e8d812148810a4ffae911",
}

var linux07CaptureDrivers = map[string]string{
	"capture_homes_test.go":     "35457d6728df2436292d85702621318d33b420b7d8aafd891cc96421be8d293b",
	"capture_launchers_test.go": "e202ab658c45b6ebaba731c2a72eda82de4cb94583b251d0f73054361e124518",
	"controls_test.go":          "2928b6a0001dccfd39f430caa93ced8528b23653f4b3542103639eef09b908e4",
	"launcher_fixture_test.go":  "deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983",
	"legacy_fixture_test.go":    "e9250a474e897a0d8aa9e0ee6f01afd0a42d8532ae6e8d812148810a4ffae911",
}

// The consumer index describes the unchanged implementation separately from
// its capture source and drivers. The embedded producer manifest retains its
// original wire bytes; it is never reencoded as a consumer baseline manifest.
func validateBaselineProvenance(m baselineManifest, platform string) error {
	if m.SourceHead != originalSource || m.Subject != "unchanged legacy implementation" || m.Platform != platform {
		return fmt.Errorf("%s", provenanceFailure)
	}
	pins, known := admittedCapturePolicies[platform]
	if platform == "linux" {
		pins, known = historicalLinuxCapturePolicy, true
	}
	if !known || m.Architecture != pins.arch || m.GoVersion != pins.goVersion || m.CaptureDriverSHA256 != pins.driver || m.LauncherDriverSHA256 != pins.launcher {
		return fmt.Errorf("%s", provenanceFailure)
	}
	if platform != "darwin" {
		return nil
	}
	if code := validateCapturePayloadIdentity(m.CapturePayloadIdentity, m.CaptureRedactedFields, m.CaptureRedactedValues); code != "" {
		return fmt.Errorf("%s", code)
	}
	if m.MetadataRole != "consumer provenance index for immutable producer capture" || m.CaptureSourceHead != macCaptureSource || m.CaptureSourceTree != macCaptureTree || m.CaptureWorkflowSHA256 != macCaptureWorkflow || !reflect.DeepEqual(m.CaptureDriverPins, macCaptureDrivers) || m.CaptureCreationPolicy != pins.nativeDeclaration {
		return fmt.Errorf("%s", provenanceFailure)
	}
	if len(m.CaptureOutputsManifestBytes) == 0 || hashBytes(m.CaptureOutputsManifestBytes) != m.CaptureOutputsManifest {
		return fmt.Errorf("capture_outputs_manifest_mismatch")
	}
	var producer struct {
		Files []struct {
			Path   string `json:"path"`
			Bytes  int    `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if json.Unmarshal(m.CaptureOutputsManifestBytes, &producer) != nil || len(producer.Files) != expectedBaselineCases || len(m.Cases) != expectedBaselineCases {
		return fmt.Errorf("capture_outputs_manifest_invalid")
	}
	for i, entry := range producer.Files {
		c := m.Cases[i]
		if entry.Path != c.Name+".json" || entry.Path != filepath.Base(entry.Path) || entry.Bytes != c.Bytes || entry.SHA256 != c.SHA256 || i > 0 && producer.Files[i-1].Path >= entry.Path {
			return fmt.Errorf("capture_outputs_index_mismatch")
		}
	}
	return nil
}

func validateLinux07Provenance(m baselineManifest) error {
	pins := admittedCapturePolicies["linux"]
	if m.SourceHead != originalSource || m.Subject != "unchanged legacy implementation" || m.Platform != "linux" ||
		m.CaptureDriverSHA256 != pins.driver || m.LauncherDriverSHA256 != pins.launcher ||
		m.Architecture != pins.arch || m.GoVersion != pins.goVersion ||
		m.CaptureSourceHead != macCaptureSource || m.CaptureSourceTree != macCaptureTree ||
		m.CaptureWorkflowHead != linux07WorkflowHead || m.CaptureWorkflowTree != linux07WorkflowTree ||
		m.CaptureArtifactSHA256 != linux07ArtifactSHA256 || m.CaptureSealSHA256 != linux07SealSHA256 ||
		!reflect.DeepEqual(m.CaptureDriverPins, linux07CaptureDrivers) || m.CaptureCreationPolicy != pins.nativeDeclaration ||
		m.MetadataRole != "consumer provenance index for immutable producer capture" {
		return fmt.Errorf("%s", provenanceFailure)
	}
	if code := validateCapturePayloadIdentity(m.CapturePayloadIdentity, m.CaptureRedactedFields, m.CaptureRedactedValues); code != "" {
		return fmt.Errorf("%s", code)
	}
	if len(m.CaptureOutputsManifestBytes) == 0 || m.CaptureOutputsManifest != linux07OutputsManifestSHA256 || hashBytes(m.CaptureOutputsManifestBytes) != m.CaptureOutputsManifest {
		return fmt.Errorf("capture_outputs_manifest_mismatch")
	}
	if m.CaptureRepeatRootBinding != "t.TempDir -> filepath.EvalSymlinks" || m.CaptureRepeatResultSHA256 != linux07RepeatResultSHA256 {
		return fmt.Errorf("%s", provenanceFailure)
	}
	if len(m.CaptureRepeatManifestBytes) == 0 || m.CaptureRepeatManifest != linux07RepeatManifestSHA256 || hashBytes(m.CaptureRepeatManifestBytes) != m.CaptureRepeatManifest {
		return fmt.Errorf("capture_repeat_manifest_mismatch")
	}
	var producer struct {
		Files []struct {
			Path   string `json:"path"`
			Bytes  int    `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if json.Unmarshal(m.CaptureOutputsManifestBytes, &producer) != nil || len(producer.Files) != expectedBaselineCases || len(m.Cases) != expectedBaselineCases {
		return fmt.Errorf("capture_outputs_manifest_invalid")
	}
	for i, entry := range producer.Files {
		c := m.Cases[i]
		if entry.Path != c.Name+".json" || entry.Path != filepath.Base(entry.Path) || entry.Bytes != c.Bytes || entry.SHA256 != c.SHA256 || i > 0 && producer.Files[i-1].Path >= entry.Path {
			return fmt.Errorf("capture_outputs_index_mismatch")
		}
	}
	var repeatProducer struct {
		Files []struct {
			Path   string `json:"path"`
			Bytes  int    `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if json.Unmarshal(m.CaptureRepeatManifestBytes, &repeatProducer) != nil || len(repeatProducer.Files) != 2 || len(m.RepeatObservations) != 2 {
		return fmt.Errorf("capture_repeat_manifest_invalid")
	}
	for i, entry := range repeatProducer.Files {
		o := m.RepeatObservations[i]
		if entry.Path != o.Name || entry.Path != filepath.Base(entry.Path) || entry.Bytes != o.Bytes || entry.SHA256 != o.SHA256 || i > 0 && repeatProducer.Files[i-1].Path >= entry.Path {
			return fmt.Errorf("capture_repeat_index_mismatch")
		}
	}
	return nil
}

func verifyLinux07Manifest(payload []byte) (baselineManifest, error) {
	path := filepath.Join("testdata", "linux", "capture-07", "manifest.json")
	if hashBytes(payload) != linux07ManifestSHA256 {
		return baselineManifest{}, fmt.Errorf("%s: frozen_manifest_mismatch", path)
	}
	var manifest baselineManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return baselineManifest{}, fmt.Errorf("%s: frozen_manifest_invalid", path)
	}
	if err := validateLinux07Provenance(manifest); err != nil {
		return baselineManifest{}, err
	}
	return manifest, nil
}

// The new Linux capture is a separate immutable comparator. The original
// historical Linux record remains pinned under testdata/linux/manifest.json.
func TestLinux07CaptureIndexRefusals(t *testing.T) {
	path := filepath.Join("testdata", "linux", "capture-07", "manifest.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := verifyLinux07Manifest(payload)
	if err != nil || len(original.Cases) != expectedBaselineCases {
		t.Fatalf("Linux 07 capture not admitted: %v", err)
	}
	for _, row := range []struct {
		name, code string
		mutate     func(*baselineManifest)
	}{
		{"source-tree", provenanceFailure, func(m *baselineManifest) { m.CaptureSourceTree = "" }},
		{"workflow-head", provenanceFailure, func(m *baselineManifest) { m.CaptureWorkflowHead = "" }},
		{"artifact", provenanceFailure, func(m *baselineManifest) { m.CaptureArtifactSHA256 = "" }},
		{"driver-pins", provenanceFailure, func(m *baselineManifest) { m.CaptureDriverPins = nil }},
		{"identity", provenanceFailure, func(m *baselineManifest) { m.CapturePayloadIdentity = "" }},
		{"transformed-output", transformedCaptureFailure, func(m *baselineManifest) {
			m.CapturePayloadIdentity = "transformed by capture sanitizer"
			m.CaptureRedactedFields = 1
		}},
		{"producer-bytes", "capture_outputs_manifest_mismatch", func(m *baselineManifest) {
			m.CaptureOutputsManifestBytes = append(bytes.Clone(m.CaptureOutputsManifestBytes), ' ')
		}},
		{"repeat-producer-bytes", "capture_repeat_manifest_mismatch", func(m *baselineManifest) {
			m.CaptureRepeatManifestBytes = append(bytes.Clone(m.CaptureRepeatManifestBytes), ' ')
		}},
		{"repeat-root-binding", provenanceFailure, func(m *baselineManifest) { m.CaptureRepeatRootBinding = "" }},
		{"repeat-index", "capture_repeat_index_mismatch", func(m *baselineManifest) {
			m.RepeatObservations = append(m.RepeatObservations[:0:0], m.RepeatObservations...)
			m.RepeatObservations[0].Bytes++
		}},
		{"case-index", "capture_outputs_index_mismatch", func(m *baselineManifest) {
			m.Cases = append(m.Cases[:0:0], m.Cases...)
			m.Cases[0].Bytes++
		}},
	} {
		t.Run(row.name, func(t *testing.T) {
			altered := original
			row.mutate(&altered)
			if err := validateLinux07Provenance(altered); err == nil || err.Error() != row.code {
				t.Fatalf("wrong rejection: %v", err)
			}
		})
	}
	altered := append(bytes.Clone(payload), ' ')
	if _, err := verifyLinux07Manifest(altered); err == nil || err.Error() != path+": frozen_manifest_mismatch" {
		t.Fatalf("corrupt Linux 07 index verdict: %v", err)
	}
}

// Both repeat records retain their exact bytes, bind distinct disposable roots
// without publishing the paths, and report the same result digest.
func TestLinux07RepeatObservations(t *testing.T) {
	manifestPath := filepath.Join("testdata", "linux", "capture-07", "manifest.json")
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := verifyLinux07Manifest(payload)
	if err != nil {
		t.Fatal(err)
	}
	type observation struct {
		FixtureRootBinding     string `json:"fixture_root_binding"`
		FixtureRootOrdinal     int    `json:"fixture_root_ordinal"`
		FixtureRootPathOmitted bool   `json:"fixture_root_path_omitted"`
		DistinctRootsInMemory  bool   `json:"distinct_roots_in_memory"`
		ResultSHA256           string `json:"result_sha256"`
	}
	for index, expected := range manifest.RepeatObservations {
		path := filepath.Join("testdata", "linux", "capture-07", "repeat-observations", expected.Name)
		data, err := os.ReadFile(path)
		if err != nil || len(data) != expected.Bytes || hashBytes(data) != expected.SHA256 {
			t.Fatalf("repeat observation changed: %s", expected.Name)
		}
		var got observation
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("repeat observation invalid: %s", expected.Name)
		}
		if got.FixtureRootBinding != manifest.CaptureRepeatRootBinding || got.FixtureRootOrdinal != index+1 || !got.FixtureRootPathOmitted || !got.DistinctRootsInMemory || got.ResultSHA256 != manifest.CaptureRepeatResultSHA256 {
			t.Fatalf("repeat root or result contract changed: %s", expected.Name)
		}
	}
}

// Historical Linux data stays immutable and distinct from the newer capture.
func TestHistoricalLinuxCaptureRemainsPinned(t *testing.T) {
	manifestPath := filepath.Join("testdata", "linux", "manifest.json")
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := verifyManifest(payload, "linux")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Cases {
		casePath := filepath.Join("testdata", "linux", "cases", entry.Name+".json")
		data, err := os.ReadFile(casePath)
		if err != nil || len(data) != entry.Bytes || hashBytes(data) != entry.SHA256 {
			t.Fatalf("historical Linux case changed: %s", entry.Name)
		}
	}
}

// The sealed Mac index passes; changed source/platform/driver declarations and
// producer-manifest bytes refuse without changing the legacy invocation count.
// Manifest-seal corruption and missing Windows exercise the public verifier.
func TestCaptureIndexRefusals(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("testdata", "darwin", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	original, err := verifyManifest(payload, "darwin")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name, code string
		mutate     func(*baselineManifest)
	}{
		{"implementation-source", provenanceFailure, func(m *baselineManifest) { m.SourceHead = macCaptureSource }},
		{"capture-source", provenanceFailure, func(m *baselineManifest) { m.CaptureSourceHead = originalSource }},
		{"capture-tree", provenanceFailure, func(m *baselineManifest) { m.CaptureSourceTree = "" }},
		{"platform", provenanceFailure, func(m *baselineManifest) { m.Platform = "linux" }},
		{"architecture", provenanceFailure, func(m *baselineManifest) { m.Architecture = "amd64" }},
		{"toolchain", provenanceFailure, func(m *baselineManifest) { m.GoVersion = "go1.26.8" }},
		{"workflow", provenanceFailure, func(m *baselineManifest) { m.CaptureWorkflowSHA256 = "" }},
		{"driver-pins", provenanceFailure, func(m *baselineManifest) { m.CaptureDriverPins = nil }},
		{"identity", provenanceFailure, func(m *baselineManifest) { m.CapturePayloadIdentity = "" }},
		{"derived04", transformedCaptureFailure, func(m *baselineManifest) {
			m.CapturePayloadIdentity = "transformed by capture sanitizer"
			m.CaptureRedactedFields = 6
			m.CaptureRedactedValues = 1
		}},
		{"producer-bytes", "capture_outputs_manifest_mismatch", func(m *baselineManifest) {
			m.CaptureOutputsManifestBytes = append(bytes.Clone(m.CaptureOutputsManifestBytes), ' ')
		}},
		{"producer-seal", "capture_outputs_manifest_mismatch", func(m *baselineManifest) { m.CaptureOutputsManifest = "" }},
		{"case-index", "capture_outputs_index_mismatch", func(m *baselineManifest) { m.Cases = append(m.Cases[:0:0], m.Cases...); m.Cases[0].Bytes++ }},
	} {
		t.Run(row.name, func(t *testing.T) {
			before := processProvenance.Invocations
			altered := original
			row.mutate(&altered)
			err := validateBaselineProvenance(altered, "darwin")
			if err == nil || err.Error() != row.code || processProvenance.Invocations != before {
				t.Fatalf("wrong rejection or API effect: %v", err)
			}
		})
	}
	altered := append(bytes.Clone(payload), ' ')
	if m, err := verifyManifest(altered, "darwin"); err == nil || err.Error() != filepath.Join("testdata", "darwin", "manifest.json")+": frozen_manifest_mismatch" || !reflect.DeepEqual(m, baselineManifest{}) {
		t.Fatalf("corrupt index verdict: %v", err)
	}
	if m, err := verifyManifest(payload, "windows"); err == nil || err.Error() != filepath.Join("testdata", "windows", "manifest.json")+": original_platform_manifest_missing" || !reflect.DeepEqual(m, baselineManifest{}) {
		t.Fatalf("missing Windows verdict: %v", err)
	}
}

// All 47 immutable Mac capsules reach the existing legacy APIs and whole-result
// comparator on this host. This cross-host replay is not native Mac execution.
func TestMacOriginalReplay(t *testing.T) {
	for _, c := range allCases() {
		t.Run(c.Name, func(t *testing.T) {
			expected := loadPlatformBaseline(t, c, "darwin")
			before := processProvenance.Invocations
			actual := countedCase(t, c)
			if processProvenance.Invocations != before+1 || !actual.Executed {
				t.Fatal("legacy API did not execute")
			}
			if err := compareResult(expected, actual); err != nil {
				t.Fatal(err)
			}
		})
	}
}
