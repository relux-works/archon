//go:build harness_parity

package harnessparity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const creationFailure = "harness_parity_environment_invalid"
const provenanceFailure = "harness_parity_provenance_invalid"
const countFailure = "harness_parity_case_count_invalid"
const creationFailureExit = 31
const transformedCaptureFailure = "harness_parity_native_output_transformed"
const expectedBaselineCases = 47
const originalSource = "643e727edd2db21307ca6d8c0332935e33d50ac8"
const replayDriverPin = "08df01cb11059b976cd57b7e395b2e82c98de96e81a366366305ac0eb909923c"
const replayLauncherDriverPin = "0a860d67daaf54ddf9691a8fe230a45996e2614528c04f2149ee1db381ddc2b8"
const provenancePrefix = "HARNESS_PARITY_RUN "

type creationProvenance struct {
	OS                        string `json:"os"`
	Arch                      string `json:"arch"`
	GoVersion                 string `json:"go_version"`
	SourceHead                string `json:"reference_source_head"`
	Driver                    string `json:"replay_driver_sha256"`
	CaptureDriver             string `json:"capture_driver_sha256"`
	LauncherDriver            string `json:"replay_launcher_driver_sha256"`
	CaptureLauncherDriver     string `json:"capture_launcher_driver_sha256"`
	EffectiveUmask            string `json:"effective_umask"`
	ProbeRequestedMode        uint32 `json:"probe_requested_mode"`
	ProbeActualMode           uint32 `json:"probe_actual_mode"`
	OriginalPolicyDeclaration string `json:"original_linux_policy,omitempty"`
	CapturePolicyDeclaration  string `json:"capture_creation_policy,omitempty"`
	CaptureArch               string `json:"capture_architecture"`
	CaptureGoVersion          string `json:"capture_go_version"`
	CapturePayloadIdentity    string `json:"capture_payload_identity"`
	CaptureRedactedFields     int    `json:"capture_redacted_fields"`
	CaptureRedactedValues     int    `json:"capture_redacted_values"`
	Expected                  int    `json:"baseline_cases_expected"`
	Executed                  int    `json:"baseline_cases_executed"`
	Invocations               int    `json:"legacy_invocations"`
	MutantApplied             bool   `json:"golden_mode_control_applied"`
	Code                      string `json:"code,omitempty"`
}

var processProvenance creationProvenance
var executedCases = map[string]bool{}

func validateCreationProvenance(p creationProvenance) string {
	return validateCreationContract(p, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

func validateCreationContract(p creationProvenance, osName, arch, goVersion string) string {
	pins, known := admittedCapturePolicies[p.OS]
	if !known {
		return "harness_parity_native_creation_policy_missing"
	}
	if p.OS != osName || p.Arch == "" || p.Arch != arch || p.GoVersion == "" || p.GoVersion != goVersion || p.SourceHead != originalSource || p.Driver != replayDriverPin || p.LauncherDriver != replayLauncherDriverPin || p.CaptureDriver != pins.driver || p.CaptureLauncherDriver != pins.launcher || p.CaptureArch != pins.arch || p.CaptureGoVersion != pins.goVersion || p.OriginalPolicyDeclaration != pins.linuxDeclaration || p.CapturePolicyDeclaration != pins.nativeDeclaration || p.Expected != expectedBaselineCases {
		return provenanceFailure
	}
	if p.OS == "darwin" && p.Arch != pins.arch {
		return provenanceFailure
	}
	if code := validateCapturePayloadIdentity(p.CapturePayloadIdentity, p.CaptureRedactedFields, p.CaptureRedactedValues); code != "" {
		return code
	}
	if p.EffectiveUmask != "0077" || p.ProbeRequestedMode != 0777 || p.ProbeActualMode != 0700 {
		return creationFailure
	}
	return ""
}

func measureCreationPolicy(dir string) (uint32, string, error) {
	f, err := os.OpenFile(filepath.Join(dir, "effective-creation-policy"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0777)
	if err != nil {
		return 0, "", err
	}
	info, statErr := f.Stat()
	closeErr := f.Close()
	if statErr != nil || closeErr != nil {
		return 0, "", fmt.Errorf("creation probe unavailable")
	}
	mode := uint32(info.Mode().Perm())
	return mode, fmt.Sprintf("%04o", uint32(0777)^mode), nil
}

func emitCreationProvenance(dir string) bool {
	processProvenance.Executed = len(executedCases)
	payload, err := json.Marshal(processProvenance)
	if err != nil {
		return false
	}
	if os.WriteFile(filepath.Join(dir, "creation-environment.json"), append(payload, '\n'), 0600) != nil {
		return false
	}
	_, err = fmt.Fprintln(os.Stdout, provenancePrefix+string(payload))
	return err == nil
}

func runParityProcess(m *testing.M) int {
	selectCreationMask(os.Getenv("HARNESS_PARITY_POLICY_CONTROL"))
	dir, err := os.MkdirTemp("", "parity-creation-")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, creationFailure)
		return creationFailureExit
	}
	// Keep the separate private receipt for the caller's evidence collector.
	processProvenance = creationProvenance{OS: runtime.GOOS, Arch: runtime.GOARCH, GoVersion: runtime.Version(), SourceHead: originalSource, Expected: expectedBaselineCases, ProbeRequestedMode: 0777}
	if runtime.GOOS == "linux" {
		processProvenance.OriginalPolicyDeclaration = "declared by run.sh, not attested"
		processProvenance.CapturePayloadIdentity = "original capture"
	}
	driver, driverErr := os.ReadFile("legacy_fixture_test.go")
	launcher, launcherErr := os.ReadFile("launcher_fixture_test.go")
	if driverErr == nil {
		processProvenance.Driver = hashBytes(driver)
	}
	if launcherErr == nil {
		processProvenance.LauncherDriver = hashBytes(launcher)
	}
	manifestBytes, manifestErr := os.ReadFile(filepath.Join("testdata", runtime.GOOS, "manifest.json"))
	if manifestErr == nil {
		if manifest, err := verifyManifest(manifestBytes, runtime.GOOS); err == nil {
			processProvenance.CaptureDriver = manifest.CaptureDriverSHA256
			processProvenance.CaptureLauncherDriver = manifest.LauncherDriverSHA256
			processProvenance.CaptureArch = manifest.Architecture
			processProvenance.CaptureGoVersion = manifest.GoVersion
			processProvenance.CapturePolicyDeclaration = manifest.CaptureCreationPolicy
			if runtime.GOOS != "linux" {
				processProvenance.CapturePayloadIdentity = manifest.CapturePayloadIdentity
			}
			processProvenance.CaptureRedactedFields = manifest.CaptureRedactedFields
			processProvenance.CaptureRedactedValues = manifest.CaptureRedactedValues
		}
	}
	mode, mask, probeErr := measureCreationPolicy(dir)
	if runtime.GOOS == "windows" {
		mask = "not-applicable"
	}
	processProvenance.ProbeActualMode, processProvenance.EffectiveUmask = mode, mask
	if _, known := admittedCapturePolicies[runtime.GOOS]; !known {
		processProvenance.Code = "harness_parity_native_creation_policy_missing"
	} else if probeErr != nil {
		processProvenance.Code = creationFailure
	} else {
		processProvenance.Code = validateCreationProvenance(processProvenance)
	}
	if control := os.Getenv("HARNESS_PARITY_POLICY_CONTROL"); control != "" && control != "0077" && control != "0022" {
		processProvenance.Code = creationFailure
	}
	if processProvenance.Code != "" {
		_ = emitCreationProvenance(dir)
		_, _ = fmt.Fprintln(os.Stderr, processProvenance.Code)
		return creationFailureExit
	}
	code := m.Run()
	if processProvenance.Executed = len(executedCases); processProvenance.Executed != expectedBaselineCases {
		processProvenance.Code = countFailure
		_, _ = fmt.Fprintln(os.Stderr, countFailure)
		code = creationFailureExit
	}
	if !emitCreationProvenance(dir) {
		_, _ = fmt.Fprintln(os.Stderr, provenanceFailure)
		return creationFailureExit
	}
	return code
}

func countedCase(t *testing.T, c fixtureCase) fixtureResult {
	t.Helper()
	actual := runCase(t, c)
	processProvenance.Invocations++
	if actual.Executed {
		executedCases[c.Name] = true
	}
	return actual
}

func laneBaseline(t *testing.T, c fixtureCase) fixtureResult {
	t.Helper()
	expected := loadBaseline(t, c)
	if os.Getenv("HARNESS_PARITY_GOLDEN_MODE_CONTROL") == "0644" && c.Name == "cursor-project-mirror" && !processProvenance.MutantApplied {
		expected.Tree = append([]treeNode(nil), expected.Tree...)
		for n, node := range expected.Tree {
			if node.Path == ".cursor/rules/.csk-managed.json" && node.Mode == 0600 {
				expected.Tree[n].Mode = 0644
				processProvenance.MutantApplied = true
				t.Log("one in-memory 0644 golden-mode control applied")
				break
			}
		}
		if !processProvenance.MutantApplied {
			t.Fatal("golden-mode control was not applied")
		}
	}
	return expected
}

// Every required field is checked before any legacy operation. The valid
// current-process record pairs missing/contradictory provenance refusals.
func testCreationProvenanceControls(t *testing.T) {
	if code := validateCreationProvenance(processProvenance); code != "" {
		t.Fatal(code)
	}
	for _, mutate := range []func(*creationProvenance){
		func(p *creationProvenance) { p.OS = "" }, func(p *creationProvenance) { p.Arch = "" }, func(p *creationProvenance) { p.GoVersion = "" }, func(p *creationProvenance) { p.SourceHead = "" }, func(p *creationProvenance) { p.Driver = "" }, func(p *creationProvenance) { p.LauncherDriver = "" }, func(p *creationProvenance) { p.CaptureDriver = "" }, func(p *creationProvenance) { p.CaptureLauncherDriver = "" }, func(p *creationProvenance) { p.OriginalPolicyDeclaration = "attested" }, func(p *creationProvenance) { p.EffectiveUmask = "0022" },
		func(p *creationProvenance) { p.CaptureArch = "" }, func(p *creationProvenance) { p.CaptureGoVersion = "" }, func(p *creationProvenance) { p.CapturePolicyDeclaration = "attested" },
		func(p *creationProvenance) { p.CapturePayloadIdentity = "" },
	} {
		before := processProvenance.Invocations
		altered := processProvenance
		mutate(&altered)
		if validateCreationProvenance(altered) == "" || processProvenance.Invocations != before {
			t.Fatal("invalid provenance accepted or invoked legacy API")
		}
	}
}

func parseCreationReceipt(output []byte) (creationProvenance, error) {
	var receipt creationProvenance
	count := 0
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, provenancePrefix) {
			if json.Unmarshal([]byte(strings.TrimPrefix(line, provenancePrefix)), &receipt) != nil {
				return receipt, fmt.Errorf("invalid creation receipt")
			}
			count++
		}
	}
	if count != 1 {
		return receipt, fmt.Errorf("missing/duplicate creation receipt")
	}
	return receipt, nil
}
