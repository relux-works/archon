//go:build harness_parity

package harnessparity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
const refreshedSource = "267bbf477354f4f3d35c9530ec8da0503cd86b8a"
const replayDriverPin = "b96b012122f3ecb70537e2f64ca5fefbbdf6225123ce11f3ff7d6fc038c3a6eb"
const replayLauncherDriverPin = "deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983"
const provenancePrefix = "HARNESS_PARITY_RUN "

func TestReplayFixtureSourcePins(t *testing.T) {
	pins := []struct{ path, sha256 string }{
		{"internal/harnessparity/legacy_fixture_test.go", replayDriverPin},
		{"internal/harnessparity/launcher_fixture_test.go", replayLauncherDriverPin},
		{"internal/config/launcher_extensions_test.go", "4390dca8fd9c96a5cd604ad41a706918ffe8393d836abbeb40951989a291e693"},
		{"cmd/curator/config_launcher_test.go", "5929248e6b69061440efdca4f0c0ee62cbc61aa3e02bcfff281498313fa851d2"},
	}
	for _, pin := range pins {
		t.Run(pin.path, func(t *testing.T) {
			path := filepath.Join("..", "..", filepath.FromSlash(pin.path))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			matchesPin := func(payload []byte) bool {
				digest := sha256.Sum256(payload)
				return hex.EncodeToString(digest[:]) == pin.sha256
			}
			if !matchesPin(data) {
				t.Fatal("replay fixture source digest mismatch")
			}
			lf := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
			if !bytes.Equal(lf, data) {
				t.Fatal("replay fixture source checkout is not LF-normalized")
			}
			crlf := bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
			if bytes.Equal(crlf, lf) {
				t.Fatal("replay fixture source has no LF line endings to convert")
			}
			if matchesPin(crlf) {
				t.Fatal("CRLF-normalized replay fixture source passed the LF blob pin")
			}
			mutant := append([]byte(nil), data...)
			mutant[len(mutant)/2] ^= 1
			if matchesPin(mutant) {
				t.Fatal("one-byte replay fixture source mutation passed the pin")
			}
		})
	}
}

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
	CaptureRunnerLabel        string `json:"capture_runner_label,omitempty"`
	CaptureUmask              string `json:"capture_umask,omitempty"`
	CaptureProbeDirectoryMode string `json:"capture_probe_directory_mode,omitempty"`
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
	pins, known := refreshedCapturePolicies[p.OS]
	if !known {
		return "harness_parity_native_creation_policy_missing"
	}
	if p.OS != osName || p.Arch == "" || p.Arch != arch || p.GoVersion == "" || p.GoVersion != goVersion || p.SourceHead != refreshedSource || p.Driver != replayDriverPin || p.LauncherDriver != replayLauncherDriverPin || p.CaptureDriver != pins.driver || p.CaptureLauncherDriver != pins.launcher || p.CaptureArch != pins.arch || p.CaptureGoVersion != pins.goVersion || p.OriginalPolicyDeclaration != pins.linuxDeclaration || p.CapturePolicyDeclaration != pins.nativeDeclaration || p.CaptureRunnerLabel != pins.runnerLabel || p.CaptureUmask != pins.umask || p.CaptureProbeDirectoryMode != pins.probeDirectoryMode || p.Expected != expectedBaselineCases {
		return provenanceFailure
	}
	if p.OS == "darwin" && p.Arch != pins.arch {
		return provenanceFailure
	}
	if code := validateCapturePayloadIdentity(p.CapturePayloadIdentity, p.CaptureRedactedFields, p.CaptureRedactedValues); code != "" {
		return code
	}
	if p.OS == "windows" {
		if p.EffectiveUmask != "not-applicable" || p.ProbeRequestedMode != 0 || p.ProbeActualMode != 0 {
			return creationFailure
		}
	} else if p.EffectiveUmask != "0077" || p.ProbeRequestedMode != 0777 || p.ProbeActualMode != 0700 {
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

func parityModuleCachePath(goPath, home string) (string, error) {
	if goPath == "" {
		if home == "" {
			return "", fmt.Errorf("original host home is unavailable")
		}
		goPath = filepath.Join(home, "go")
	}
	first := strings.Split(goPath, string(os.PathListSeparator))[0]
	if first == "" || !filepath.IsAbs(first) {
		return "", fmt.Errorf("original Go module cache root is not absolute")
	}
	return filepath.Join(first, "pkg", "mod"), nil
}

func pinParityModuleCache() error {
	if os.Getenv("GOMODCACHE") != "" {
		return nil
	}
	goPath := os.Getenv("GOPATH")
	home, err := os.UserHomeDir()
	if err != nil && goPath == "" {
		return fmt.Errorf("resolve original host home for Go module cache: %w", err)
	}
	cache, err := parityModuleCachePath(goPath, home)
	if err != nil {
		return err
	}
	return os.Setenv("GOMODCACHE", cache)
}

func runParityProcess(m *testing.M) int {
	if err := pinParityModuleCache(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, creationFailure)
		return creationFailureExit
	}
	selectCreationMask(os.Getenv("HARNESS_PARITY_POLICY_CONTROL"))
	dir, err := os.MkdirTemp("", "parity-creation-")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, creationFailure)
		return creationFailureExit
	}
	// Keep the separate private receipt for the caller's evidence collector.
	requestedMode := uint32(0777)
	if runtime.GOOS == "windows" {
		requestedMode = 0
	}
	processProvenance = creationProvenance{OS: runtime.GOOS, Arch: runtime.GOARCH, GoVersion: runtime.Version(), SourceHead: refreshedSource, Expected: expectedBaselineCases, ProbeRequestedMode: requestedMode}
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
	manifestBytes, manifestErr := os.ReadFile(filepath.Join("testdata", runtime.GOOS, "main-267bbf47", "manifest.json"))
	var windowsC3Err error
	if manifestErr == nil {
		if manifest, err := verifyRefreshedManifest(manifestBytes, runtime.GOOS); err == nil {
			processProvenance.CaptureDriver = manifest.CaptureDriverSHA256
			processProvenance.CaptureLauncherDriver = manifest.LauncherDriverSHA256
			processProvenance.CaptureArch = manifest.Architecture
			processProvenance.CaptureGoVersion = manifest.GoVersion
			processProvenance.CapturePolicyDeclaration = manifest.CaptureCreationPolicy
			processProvenance.CaptureRunnerLabel = manifest.CaptureRunnerLabel
			processProvenance.CaptureUmask = manifest.CaptureUmask
			processProvenance.CaptureProbeDirectoryMode = manifest.CaptureProbeDirectoryMode
			if runtime.GOOS != "linux" {
				processProvenance.CapturePayloadIdentity = manifest.CapturePayloadIdentity
			}
			processProvenance.CaptureRedactedFields = manifest.CaptureRedactedFields
			processProvenance.CaptureRedactedValues = manifest.CaptureRedactedValues
			if runtime.GOOS == "windows" {
				c3, err := os.ReadFile(filepath.Join("testdata", "windows", "main-267bbf47", "old-new-windows.tsv"))
				if err != nil {
					windowsC3Err = err
				} else {
					windowsC3Err = validateWindowsC3Table(c3, manifest)
				}
			}
		}
	}
	var mode uint32
	mask := "not-applicable"
	var probeErr error
	if runtime.GOOS != "windows" {
		mode, mask, probeErr = measureCreationPolicy(dir)
		processProvenance.ProbeRequestedMode = 0777
	}
	processProvenance.ProbeActualMode, processProvenance.EffectiveUmask = mode, mask
	if windowsC3Err != nil {
		processProvenance.Code = provenanceFailure
	} else if _, known := refreshedCapturePolicies[runtime.GOOS]; !known {
		processProvenance.Code = "harness_parity_native_creation_policy_missing"
	} else if probeErr != nil {
		processProvenance.Code = creationFailure
	} else {
		processProvenance.Code = validateCreationProvenance(processProvenance)
	}
	if control := os.Getenv("HARNESS_PARITY_POLICY_CONTROL"); control != "" {
		if runtime.GOOS == "windows" && control != "not-applicable" || runtime.GOOS != "windows" && control != "0077" && control != "0022" {
			processProvenance.Code = creationFailure
		}
	}
	if processProvenance.Code != "" {
		_ = emitCreationProvenance(dir)
		_, _ = fmt.Fprintln(os.Stderr, processProvenance.Code)
		return creationFailureExit
	}
	os.Args = privateLogControlChildArgs(os.Args, os.Getenv("CAPTURE_PRIVATE_LOG_PRINT_FIXTURE"), os.Getenv("CAPTURE_PRIVATE_LOG_PRINT_STREAM"))
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

const privateLogProbeRunArg = "-test.run=^TestNativeCapturePrivateLogCannotReachOutput$"
const privateLogProbeWithBaselineRunArg = "-test.run=^(TestMacOriginalReplay|TestNativeCapturePrivateLogCannotReachOutput)$"

// Preserve TestMain's 47-case floor in the private-log output probe child.
func privateLogControlChildArgs(args []string, fixture, stream string) []string {
	if fixture == "" || (stream != "stdout" && stream != "stderr") {
		return args
	}
	for i, arg := range args {
		if arg == privateLogProbeRunArg {
			adjusted := append([]string(nil), args...)
			adjusted[i] = privateLogProbeWithBaselineRunArg
			return adjusted
		}
	}
	return args
}

func TestPrivateLogControlChildArgsRetainsFullBaselineGate(t *testing.T) {
	probeArgs := []string{"harnessparity.test", privateLogProbeRunArg}
	for _, stream := range []string{"stdout", "stderr"} {
		got := privateLogControlChildArgs(probeArgs, "fixture.raw", stream)
		if len(got) != len(probeArgs) || got[1] != privateLogProbeWithBaselineRunArg {
			t.Fatalf("stream %s did not include the full baseline replay: %v", stream, got)
		}
	}
	if probeArgs[1] != privateLogProbeRunArg {
		t.Fatal("control-child selection mutated the caller arguments")
	}
	for _, control := range []struct {
		name, fixture, stream, run string
	}{
		{"missing-fixture", "", "stdout", privateLogProbeRunArg},
		{"invalid-stream", "fixture.raw", "other", privateLogProbeRunArg},
		{"unrelated-filter", "fixture.raw", "stdout", "-test.run=^TestHomeByteIdentity$"},
	} {
		args := []string{"harnessparity.test", control.run}
		got := privateLogControlChildArgs(args, control.fixture, control.stream)
		if len(got) != len(args) || got[1] != control.run {
			t.Errorf("%s changed an unrelated child selection: %v", control.name, got)
		}
	}
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
	mutations := []func(*creationProvenance){
		func(p *creationProvenance) { p.OS = "" }, func(p *creationProvenance) { p.Arch = "" }, func(p *creationProvenance) { p.GoVersion = "" }, func(p *creationProvenance) { p.SourceHead = "" }, func(p *creationProvenance) { p.Driver = "" }, func(p *creationProvenance) { p.LauncherDriver = "" }, func(p *creationProvenance) { p.CaptureDriver = "" }, func(p *creationProvenance) { p.CaptureLauncherDriver = "" }, func(p *creationProvenance) { p.OriginalPolicyDeclaration = "attested" }, func(p *creationProvenance) { p.EffectiveUmask = "0022" },
		func(p *creationProvenance) { p.CaptureArch = "not-the-pinned-architecture" }, func(p *creationProvenance) { p.CaptureGoVersion = "" }, func(p *creationProvenance) { p.CapturePolicyDeclaration = "attested" },
		func(p *creationProvenance) { p.CaptureRunnerLabel = "changed-runner" },
		func(p *creationProvenance) { p.CaptureUmask = "0077" }, func(p *creationProvenance) { p.CaptureProbeDirectoryMode = "0700" },
		func(p *creationProvenance) { p.CapturePayloadIdentity = "" },
	}
	if runtime.GOOS != "windows" {
		mutations = append(mutations, func(p *creationProvenance) { p.CaptureArch = "" })
	}
	for _, mutate := range mutations {
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
