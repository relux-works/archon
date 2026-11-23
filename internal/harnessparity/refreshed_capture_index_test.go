//go:build harness_parity

package harnessparity

import (
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const refreshedCaptureSourceHead = "44ff5d16c7dc6d2382981b83e14b0efd9ffddb85"
const refreshedCaptureSourceTree = "d72b3900459be99d1b407ef74c4cb3b9db942097"
const refreshedCaptureWorkflowSHA256 = "948e0cfe0ea0b5f6434b26a9ed64e5c1a6ebd7b9e1acb7035902adee29af1b1a"
const refreshedCaptureRepeatRootBinding = "t.TempDir -> filepath.EvalSymlinks"
const windowsC3SHA256 = "72e13b1d3f05f5b354cc582e063738ca340b430ab8429d283df609ad1d2a20b0"

var refreshedCaptureArtifactSHA256 = map[string]string{
	"linux":   "bc2367c9d77da74f5dad48d107519042cfbd8829a6167197ee34cc459c78029e",
	"darwin":  "8e55fbb5849f76d1cc277002bc3a8395d922b9042e5ee83cba903d0850983ace",
	"windows": "2b3427c41ffa245be17cd88eba23f813c874f54fee044dba5f1e3d029135ad6c",
}

var refreshedCaptureSealSHA256 = map[string]string{
	"linux":   "bfd31772f93127efac7d8a689469cf6effe0fac6c43fd6fa8f57783c7978f56d",
	"darwin":  "269654f39eac1bad06c253e916bbab5b6d4301a3d4238351901e4b76b0270187",
	"windows": "16cd0f3f48f2e0ef9ed78982745c6b23a40fc1ad8cc46cf4f629cc11aec94956",
}

var refreshedCaptureOutputsManifestSHA256 = map[string]string{
	"linux":   "5a3c534a7c2ace839ccf939ce795f6a25a94622cdd5c8cecd2a2226dc2c8d1a8",
	"darwin":  "612a02d752d4e8232ef0c483707b6b75eee64639d662403d4f86eecd10c7ec71",
	"windows": "c7d23d51b4773d20df6b5227ac5c9ff236e7a0f37f0787ea711f1b4205d97f65",
}

var refreshedCaptureRepeatManifestSHA256 = map[string]string{
	"linux":   "7b5ff8a45e7cd2cb05bd4c524c3975ffcd5b6626f52273f733ba994825893d6c",
	"darwin":  "40d635560094090587939014376ccd0fe13833ab555e2ff74f9bc15715f9bca0",
	"windows": "9cadc6f24a23439c8d05fe498768a2d8bffffc73b212206c5661dbdba7aea8c2",
}

var refreshedCaptureRepeatResultSHA256 = map[string]string{
	"linux":   "f95f37ae3e32b9fea3f6120392c1f7135476fce53e986785ca675467754bf894",
	"darwin":  "d10a834b09e4fe67fb0389d46e49b4f3e20d69ebe6170e9eb96fb21f2e66c71c",
	"windows": "505e1a6c39342b8ee5fbc47c989a7921a1ca346688c3bc5fb432e5dcf0ac061d",
}

var refreshedCaptureDriverPins = map[string]string{
	"testdata/originalcapture/ancestor_projection_test.go": "03e153275c7ca9a7a83ade0705ae9d4aac8e4ee3d30a36e381b8f97dbaf1851f",
	"testdata/originalcapture/capture_homes_test.go":       "35457d6728df2436292d85702621318d33b420b7d8aafd891cc96421be8d293b",
	"testdata/originalcapture/capture_launchers_test.go":   "e202ab658c45b6ebaba731c2a72eda82de4cb94583b251d0f73054361e124518",
	"testdata/originalcapture/controls_test.go":            "2928b6a0001dccfd39f430caa93ced8528b23653f4b3542103639eef09b908e4",
	"testdata/originalcapture/launcher_fixture_test.go":    "deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983",
	"testdata/originalcapture/legacy_fixture_test.go":      "b96b012122f3ecb70537e2f64ca5fefbbdf6225123ce11f3ff7d6fc038c3a6eb",
}

func validateRefreshedBaselineProvenance(m baselineManifest, platform string) error {
	pins, known := refreshedCapturePolicies[platform]
	if !known || m.SourceHead != refreshedSource || m.Subject != "unchanged legacy implementation" || m.Platform != platform ||
		m.Architecture != pins.arch || m.GoVersion != pins.goVersion || m.CaptureDriverSHA256 != pins.driver ||
		m.LauncherDriverSHA256 != pins.launcher || m.CaptureSourceHead != refreshedCaptureSourceHead ||
		m.CaptureSourceTree != refreshedCaptureSourceTree || m.CaptureWorkflowHead != refreshedCaptureSourceHead ||
		m.CaptureWorkflowTree != refreshedCaptureSourceTree || m.CaptureWorkflowSHA256 != refreshedCaptureWorkflowSHA256 ||
		m.CaptureArtifactSHA256 != refreshedCaptureArtifactSHA256[platform] || m.CaptureSealSHA256 != refreshedCaptureSealSHA256[platform] ||
		m.CaptureCreationPolicy != pins.nativeDeclaration || m.CaptureRunnerLabel != pins.runnerLabel ||
		m.CaptureUmask != pins.umask || m.CaptureProbeDirectoryMode != pins.probeDirectoryMode ||
		m.MetadataRole != "consumer provenance index for immutable producer capture" ||
		!reflect.DeepEqual(m.CaptureDriverPins, refreshedCaptureDriverPins) {
		return fmt.Errorf("%s", provenanceFailure)
	}
	if code := validateCapturePayloadIdentity(m.CapturePayloadIdentity, m.CaptureRedactedFields, m.CaptureRedactedValues); code != "" {
		return fmt.Errorf("%s", code)
	}
	if len(m.CaptureOutputsManifestBytes) == 0 || m.CaptureOutputsManifest != refreshedCaptureOutputsManifestSHA256[platform] || hashBytes(m.CaptureOutputsManifestBytes) != m.CaptureOutputsManifest {
		return fmt.Errorf("capture_outputs_manifest_mismatch")
	}
	if len(m.CaptureRepeatManifestBytes) == 0 || m.CaptureRepeatManifest != refreshedCaptureRepeatManifestSHA256[platform] || hashBytes(m.CaptureRepeatManifestBytes) != m.CaptureRepeatManifest {
		return fmt.Errorf("capture_repeat_manifest_mismatch")
	}
	if m.CaptureRepeatRootBinding != refreshedCaptureRepeatRootBinding || m.CaptureRepeatResultSHA256 != refreshedCaptureRepeatResultSHA256[platform] {
		return fmt.Errorf("%s", provenanceFailure)
	}
	if len(m.Cases) != expectedBaselineCases || len(m.RepeatObservations) != 2 || len(m.Corpora) != 3 {
		return fmt.Errorf("%s", provenanceFailure)
	}
	type fileRecord struct {
		Path   string `json:"path"`
		Bytes  int    `json:"bytes"`
		SHA256 string `json:"sha256"`
	}
	var producer struct {
		Schema     string       `json:"schema"`
		LegacyBase string       `json:"legacy_base"`
		Files      []fileRecord `json:"files"`
	}
	if json.Unmarshal(m.CaptureOutputsManifestBytes, &producer) != nil || producer.Schema != "capture-manifest/v2" || producer.LegacyBase != refreshedSource || len(producer.Files) != expectedBaselineCases {
		return fmt.Errorf("capture_outputs_manifest_invalid")
	}
	for i, entry := range producer.Files {
		c := m.Cases[i]
		if entry.Path != c.Name+".json" || entry.Path != filepath.Base(entry.Path) || entry.Bytes != c.Bytes || entry.SHA256 != c.SHA256 || i > 0 && producer.Files[i-1].Path >= entry.Path {
			return fmt.Errorf("capture_outputs_index_mismatch")
		}
	}
	var repeatProducer struct {
		Schema     string       `json:"schema"`
		LegacyBase string       `json:"legacy_base"`
		Files      []fileRecord `json:"files"`
	}
	if json.Unmarshal(m.CaptureRepeatManifestBytes, &repeatProducer) != nil || repeatProducer.Schema != "capture-manifest/v2" || repeatProducer.LegacyBase != refreshedSource || len(repeatProducer.Files) != 2 {
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

func TestRefreshedCaptureIndexRefusals(t *testing.T) {
	path := filepath.Join("testdata", "darwin", "main-267bbf47", "manifest.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := verifyRefreshedManifest(payload, "darwin")
	if err != nil {
		t.Fatalf("refreshed capture not admitted: %v", err)
	}
	for _, row := range []struct {
		name, code string
		mutate     func(*baselineManifest)
	}{
		{"source-head", provenanceFailure, func(m *baselineManifest) { m.SourceHead = originalSource }},
		{"capture-tree", provenanceFailure, func(m *baselineManifest) { m.CaptureSourceTree = "" }},
		{"workflow", provenanceFailure, func(m *baselineManifest) { m.CaptureWorkflowSHA256 = "" }},
		{"artifact", provenanceFailure, func(m *baselineManifest) { m.CaptureArtifactSHA256 = "" }},
		{"seal", provenanceFailure, func(m *baselineManifest) { m.CaptureSealSHA256 = "" }},
		{"driver-pins", provenanceFailure, func(m *baselineManifest) { m.CaptureDriverPins = nil }},
		{"projector-pin", provenanceFailure, func(m *baselineManifest) {
			m.CaptureDriverPins = map[string]string{"ancestor_projection_test.go": "wrong"}
		}},
		{"identity", provenanceFailure, func(m *baselineManifest) { m.CapturePayloadIdentity = "" }},
		{"transformed-output", transformedCaptureFailure, func(m *baselineManifest) {
			m.CapturePayloadIdentity = "transformed by capture sanitizer"
			m.CaptureRedactedFields = 1
		}},
		{"producer-bytes", "capture_outputs_manifest_mismatch", func(m *baselineManifest) {
			m.CaptureOutputsManifestBytes = append(bytes.Clone(m.CaptureOutputsManifestBytes), ' ')
		}},
		{"case-index", "capture_outputs_index_mismatch", func(m *baselineManifest) { m.Cases = append(m.Cases[:0:0], m.Cases...); m.Cases[0].Bytes++ }},
		{"repeat-producer-bytes", "capture_repeat_manifest_mismatch", func(m *baselineManifest) {
			m.CaptureRepeatManifestBytes = append(bytes.Clone(m.CaptureRepeatManifestBytes), ' ')
		}},
		{"repeat-root-binding", provenanceFailure, func(m *baselineManifest) { m.CaptureRepeatRootBinding = "" }},
		{"repeat-index", "capture_repeat_index_mismatch", func(m *baselineManifest) {
			m.RepeatObservations = append(m.RepeatObservations[:0:0], m.RepeatObservations...)
			m.RepeatObservations[0].Bytes++
		}},
	} {
		t.Run(row.name, func(t *testing.T) {
			altered := original
			row.mutate(&altered)
			if err := validateRefreshedBaselineProvenance(altered, "darwin"); err == nil || err.Error() != row.code {
				t.Fatalf("wrong rejection: %v", err)
			}
		})
	}
	altered := append(bytes.Clone(payload), ' ')
	if _, err := verifyRefreshedManifest(altered, "darwin"); err == nil || err.Error() != path+": frozen_manifest_mismatch" {
		t.Fatalf("corrupt refreshed index verdict: %v", err)
	}

	windowsPath := filepath.Join("testdata", "windows", "main-267bbf47", "manifest.json")
	windowsPayload, err := os.ReadFile(windowsPath)
	if err != nil {
		t.Fatal(err)
	}
	windowsOriginal, err := verifyRefreshedManifest(windowsPayload, "windows")
	if err != nil {
		t.Fatalf("Windows refreshed capture not admitted: %v", err)
	}
	if _, err := verifyRefreshedManifest(append(bytes.Clone(windowsPayload), ' '), "windows"); err == nil || err.Error() != windowsPath+": frozen_manifest_mismatch" {
		t.Fatalf("tampered Windows manifest was not refused: %v", err)
	}
	windowsOriginalBytes, err := json.Marshal(windowsOriginal)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name, code string
		mutate     func(*baselineManifest)
	}{
		{"source-head", provenanceFailure, func(m *baselineManifest) { m.SourceHead = originalSource }},
		{"workflow", provenanceFailure, func(m *baselineManifest) { m.CaptureWorkflowSHA256 = "wrong" }},
		{"artifact", provenanceFailure, func(m *baselineManifest) { m.CaptureArtifactSHA256 = "wrong" }},
		{"seal", provenanceFailure, func(m *baselineManifest) { m.CaptureSealSHA256 = "wrong" }},
		{"driver-pin", provenanceFailure, func(m *baselineManifest) { m.CaptureDriverPins["testdata/originalcapture/controls_test.go"] = "wrong" }},
		{"case-missing", provenanceFailure, func(m *baselineManifest) { m.Cases = m.Cases[:len(m.Cases)-1] }},
		{"case-altered", "capture_outputs_index_mismatch", func(m *baselineManifest) { m.Cases[0].SHA256 = strings.Repeat("0", 64) }},
		{"repeat-manifest", "capture_repeat_manifest_mismatch", func(m *baselineManifest) {
			m.CaptureRepeatManifestBytes = append(bytes.Clone(m.CaptureRepeatManifestBytes), ' ')
		}},
		{"runner-label", provenanceFailure, func(m *baselineManifest) { m.CaptureRunnerLabel = "windows-2022" }},
		{"architecture-invented", provenanceFailure, func(m *baselineManifest) { m.Architecture = "amd64" }},
		{"umask-not-applicable", provenanceFailure, func(m *baselineManifest) { m.CaptureUmask = "0077" }},
		{"probe-mode-not-applicable", provenanceFailure, func(m *baselineManifest) { m.CaptureProbeDirectoryMode = "0700" }},
	} {
		t.Run("windows-"+row.name, func(t *testing.T) {
			var altered baselineManifest
			if err := json.Unmarshal(windowsOriginalBytes, &altered); err != nil {
				t.Fatal(err)
			}
			row.mutate(&altered)
			if err := validateRefreshedBaselineProvenance(altered, "windows"); err == nil || err.Error() != row.code {
				t.Fatalf("wrong Windows refusal: %v", err)
			}
		})
	}
}

func validateWindowsC3Table(data []byte, manifest baselineManifest) error {
	if hashBytes(data) != windowsC3SHA256 {
		return fmt.Errorf("windows_c3_table_digest_mismatch")
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = '\t'
	reader.FieldsPerRecord = 6
	rows, err := reader.ReadAll()
	if err != nil || len(rows) != expectedBaselineCases+1 {
		return fmt.Errorf("windows_c3_table_invalid")
	}
	wantHeader := []string{"case", "old_path", "old_sha256", "new_path", "new_sha256", "bytes_equal"}
	if !reflect.DeepEqual(rows[0], wantHeader) {
		return fmt.Errorf("windows_c3_table_invalid")
	}
	newCases := make(map[string]string, len(manifest.Cases))
	for _, item := range manifest.Cases {
		newCases[item.Name+".json"] = item.SHA256
	}
	if len(newCases) != expectedBaselineCases {
		return fmt.Errorf("windows_c3_table_case_mismatch")
	}
	identical, different, added := 0, 0, 0
	previous := ""
	for _, row := range rows[1:] {
		name, oldPath, oldSHA, newPath, newSHA, equality := row[0], row[1], row[2], row[3], row[4], row[5]
		if name <= previous || path.Base(name) != name || !strings.HasSuffix(name, ".json") || newPath != "outputs/"+name || newCases[name] != newSHA {
			return fmt.Errorf("windows_c3_table_case_mismatch")
		}
		previous = name
		if decoded, decodeErr := hex.DecodeString(newSHA); decodeErr != nil || len(decoded) != 32 {
			return fmt.Errorf("windows_c3_table_case_mismatch")
		}
		if oldPath == "absent" || oldSHA == "absent" {
			if oldPath != "absent" || oldSHA != "absent" || equality != "n/a" {
				return fmt.Errorf("windows_c3_table_case_mismatch")
			}
			added++
		} else {
			if oldPath != "outputs/"+name {
				return fmt.Errorf("windows_c3_table_case_mismatch")
			}
			if decoded, decodeErr := hex.DecodeString(oldSHA); decodeErr != nil || len(decoded) != 32 {
				return fmt.Errorf("windows_c3_table_case_mismatch")
			}
			switch equality {
			case "yes":
				if oldSHA != newSHA {
					return fmt.Errorf("windows_c3_table_case_mismatch")
				}
				identical++
			case "no":
				if oldSHA == newSHA {
					return fmt.Errorf("windows_c3_table_case_mismatch")
				}
				different++
			default:
				return fmt.Errorf("windows_c3_table_case_mismatch")
			}
		}
		delete(newCases, name)
	}
	if len(newCases) != 0 || identical != 44 || different != 2 || added != 1 {
		return fmt.Errorf("windows_c3_table_counts_mismatch")
	}
	return nil
}

func TestWindowsC3TablePositiveAndTamperRefusal(t *testing.T) {
	base := filepath.Join("testdata", "windows", "main-267bbf47")
	manifestBytes, err := os.ReadFile(filepath.Join(base, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := verifyRefreshedManifest(manifestBytes, "windows")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(base, "old-new-windows.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateWindowsC3Table(data, manifest); err != nil {
		t.Fatalf("accepted C3 table refused: %v", err)
	}
	for _, row := range []struct {
		name string
		data []byte
	}{
		{"changed-row", bytes.Replace(data, []byte("\tyes\n"), []byte("\tno\n"), 1)},
		{"malformed-row", bytes.Replace(data, []byte("\t"), []byte(""), 1)},
	} {
		t.Run(row.name, func(t *testing.T) {
			if err := validateWindowsC3Table(row.data, manifest); err == nil || err.Error() != "windows_c3_table_digest_mismatch" {
				t.Fatalf("tampered C3 table was not refused: %v", err)
			}
		})
	}
}

func TestRefreshedRepeatObservations(t *testing.T) {
	for _, platform := range []string{"linux", "darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			base := filepath.Join("testdata", platform, "main-267bbf47")
			payload, err := os.ReadFile(filepath.Join(base, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := verifyRefreshedManifest(payload, platform)
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
				path := filepath.Join(base, "repeat-observations", expected.Name)
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
		})
	}
}
