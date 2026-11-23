//go:build harness_parity

package harnessparity

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func validateCapturePayloadIdentity(identity string, fields, values int) string {
	if fields < 0 || values < 0 {
		return provenanceFailure
	}
	if identity == "transformed by capture sanitizer" && (fields > 0 || values > 0) {
		return transformedCaptureFailure
	}
	if identity != "original capture" || fields != 0 || values != 0 {
		return provenanceFailure
	}
	return ""
}

type capturePolicyPins struct {
	driver, launcher, arch, goVersion, linuxDeclaration, nativeDeclaration string
	runnerLabel, umask, probeDirectoryMode                                 string
}

// Linux 07 and Darwin bind their admitted captures. Historical Linux bytes
// stay frozen separately; Windows has no admitted comparator contract.
var admittedCapturePolicies = map[string]capturePolicyPins{
	"linux": {
		driver:   "e9250a474e897a0d8aa9e0ee6f01afd0a42d8532ae6e8d812148810a4ffae911",
		launcher: "deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983",
		arch:     "amd64", goVersion: "go1.26.8", linuxDeclaration: "declared by run.sh, not attested",
		nativeDeclaration: "0077 declared by capture workflow and seal; not a measured capture probe",
	},
	"darwin": {
		driver:   "e9250a474e897a0d8aa9e0ee6f01afd0a42d8532ae6e8d812148810a4ffae911",
		launcher: "deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983",
		arch:     "arm64", goVersion: "go1.25.5",
		nativeDeclaration: "0077 declared by capture workflow and metadata; not a measured capture probe",
	},
}

var refreshedCapturePolicies = map[string]capturePolicyPins{
	"linux": {
		driver:   "b96b012122f3ecb70537e2f64ca5fefbbdf6225123ce11f3ff7d6fc038c3a6eb",
		launcher: "deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983",
		arch:     "amd64", goVersion: "go1.25.5", linuxDeclaration: "declared by run.sh, not attested",
		nativeDeclaration: "umask 0077 and probe directory mode 0700 recorded in export seal",
	},
	"darwin": {
		driver:   "b96b012122f3ecb70537e2f64ca5fefbbdf6225123ce11f3ff7d6fc038c3a6eb",
		launcher: "deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983",
		arch:     "arm64", goVersion: "go1.25.5",
		nativeDeclaration: "umask 0077 and probe directory mode 0700 recorded in export seal",
	},
	"windows": {
		driver:    "b96b012122f3ecb70537e2f64ca5fefbbdf6225123ce11f3ff7d6fc038c3a6eb",
		launcher:  "deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983",
		goVersion: "go1.25.5", runnerLabel: "windows-latest",
		nativeDeclaration: "umask not-applicable and probe directory mode not-applicable recorded in export seal",
		umask:             "not-applicable", probeDirectoryMode: "not-applicable",
	},
}

// Valid Linux, Darwin and Windows records pass their pure contracts; this does
// not execute another platform on the current host. Missing, contradictory and
// unsupported-platform evidence is refused before any legacy call.
func testNativeCreationContract(t *testing.T) {
	for _, osName := range []string{"linux", "darwin", "windows"} {
		t.Run(osName, func(t *testing.T) {
			// Every admitted raw case and pinned corpus is readable and intact.
			// Loading a captured case is not a native replay on this host.
			t.Run("captured-fixtures", func(t *testing.T) {
				seen := map[string]bool{}
				for _, c := range allCases() {
					baseline := loadPlatformBaseline(t, c, osName)
					if seen[baseline.Case.Name] {
						t.Fatal("duplicate captured case")
					}
					seen[baseline.Case.Name] = true
				}
				if len(seen) != expectedBaselineCases {
					t.Fatal("incomplete captured case set")
				}
			})
			pins := refreshedCapturePolicies[osName]
			arch := pins.arch
			umask, requestedMode, actualMode := "0077", uint32(0777), uint32(0700)
			if arch == "" {
				arch = runtime.GOARCH
			}
			if osName == "windows" {
				umask, requestedMode, actualMode = "not-applicable", 0, 0
			}
			p := creationProvenance{OS: osName, Arch: arch, GoVersion: "go1.25.5", SourceHead: refreshedSource, Driver: replayDriverPin, CaptureDriver: pins.driver, LauncherDriver: replayLauncherDriverPin, CaptureLauncherDriver: pins.launcher, CaptureArch: pins.arch, CaptureGoVersion: pins.goVersion, OriginalPolicyDeclaration: pins.linuxDeclaration, CapturePolicyDeclaration: pins.nativeDeclaration, CaptureRunnerLabel: pins.runnerLabel, CaptureUmask: pins.umask, CaptureProbeDirectoryMode: pins.probeDirectoryMode, Expected: 47, EffectiveUmask: umask, ProbeRequestedMode: requestedMode, ProbeActualMode: actualMode}
			p.CapturePayloadIdentity = "original capture"
			if code := validateCreationContract(p, osName, arch, p.GoVersion); code != "" {
				t.Fatal(code)
			}
			changes := []struct {
				name   string
				mutate func(*creationProvenance)
				code   string
			}{
				{"capture-toolchain-missing", func(p *creationProvenance) { p.CaptureGoVersion = "" }, provenanceFailure},
				{"capture-architecture-substitution", func(p *creationProvenance) { p.CaptureArch = "not-the-pinned-architecture" }, provenanceFailure},
				{"driver-substitution", func(p *creationProvenance) { p.CaptureDriver = "not-the-pinned-driver" }, provenanceFailure},
				{"declaration-substitution", func(p *creationProvenance) { p.CapturePolicyDeclaration = "attested" }, provenanceFailure},
				{"runner-label-substitution", func(p *creationProvenance) { p.CaptureRunnerLabel = "windows-2022" }, provenanceFailure},
				{"capture-umask-substitution", func(p *creationProvenance) { p.CaptureUmask = "0077" }, provenanceFailure},
				{"capture-probe-mode-substitution", func(p *creationProvenance) { p.CaptureProbeDirectoryMode = "0700" }, provenanceFailure},
				{"identity-missing", func(p *creationProvenance) { p.CapturePayloadIdentity = "" }, provenanceFailure},
				{"transformed-output", func(p *creationProvenance) {
					p.CapturePayloadIdentity = "transformed by capture sanitizer"
					p.CaptureRedactedFields = 6
					p.CaptureRedactedValues = 1
				}, transformedCaptureFailure},
				{"undeclared-transformation", func(p *creationProvenance) { p.CaptureRedactedValues = 1 }, provenanceFailure},
				{"mode-mismatch", func(p *creationProvenance) { p.ProbeActualMode = 0755 }, creationFailure},
				{"policy-mismatch", func(p *creationProvenance) { p.EffectiveUmask = "0022" }, creationFailure},
				{"unsupported-platform", func(p *creationProvenance) { p.OS = "plan9" }, "harness_parity_native_creation_policy_missing"},
			}
			if pins.arch != "" {
				changes = append(changes, struct {
					name   string
					mutate func(*creationProvenance)
					code   string
				}{"capture-architecture-missing", func(p *creationProvenance) { p.CaptureArch = "" }, provenanceFailure})
			}
			for _, change := range changes {
				t.Run(change.name, func(t *testing.T) {
					before := processProvenance.Invocations
					altered := p
					change.mutate(&altered)
					if code := validateCreationContract(altered, osName, arch, p.GoVersion); code != change.code || processProvenance.Invocations != before {
						t.Fatalf("wrong refusal or legacy call: %s", code)
					}
				})
			}
		})
	}
	// The admitted original index passes before the historical transformation
	// declaration (six removed fields, one changed value) is refused without
	// invoking an API. The historical raw wrapper is retained outside fixtures.
	t.Run("darwin-derived-declaration-refusal", func(t *testing.T) {
		payload, err := os.ReadFile(filepath.Join("testdata", "darwin", "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := verifyManifest(payload, "darwin")
		if err != nil {
			t.Fatal(err)
		}
		before := processProvenance.Invocations
		if code := validateCapturePayloadIdentity(manifest.CapturePayloadIdentity, manifest.CaptureRedactedFields, manifest.CaptureRedactedValues); code != "" {
			t.Fatal(code)
		}
		manifest.CapturePayloadIdentity = "transformed by capture sanitizer"
		manifest.CaptureRedactedFields, manifest.CaptureRedactedValues = 6, 1
		if validateCapturePayloadIdentity(manifest.CapturePayloadIdentity, manifest.CaptureRedactedFields, manifest.CaptureRedactedValues) != transformedCaptureFailure || processProvenance.Invocations != before {
			t.Fatal("derived native capture was accepted or invoked the legacy API")
		}
	})
}
