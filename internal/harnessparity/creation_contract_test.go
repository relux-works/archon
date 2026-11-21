//go:build harness_parity

package harnessparity

import (
	"os"
	"path/filepath"
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

// Valid Linux and Darwin records pass their respective pure contracts; this
// does not execute Darwin on Linux. Missing, contradictory and cross-platform
// evidence is refused before any legacy call; Windows has no implicit fallback.
func testNativeCreationContract(t *testing.T) {
	for _, osName := range []string{"linux", "darwin"} {
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
			pins := admittedCapturePolicies[osName]
			p := creationProvenance{OS: osName, Arch: pins.arch, GoVersion: "go1.25.5", SourceHead: originalSource, Driver: replayDriverPin, LauncherDriver: replayLauncherDriverPin, CaptureDriver: pins.driver, CaptureLauncherDriver: pins.launcher, CaptureArch: pins.arch, CaptureGoVersion: pins.goVersion, OriginalPolicyDeclaration: pins.linuxDeclaration, CapturePolicyDeclaration: pins.nativeDeclaration, Expected: 47, EffectiveUmask: "0077", ProbeRequestedMode: 0777, ProbeActualMode: 0700}
			p.CapturePayloadIdentity = "original capture"
			if code := validateCreationContract(p, osName, pins.arch, p.GoVersion); code != "" {
				t.Fatal(code)
			}
			for _, change := range []struct {
				name   string
				mutate func(*creationProvenance)
				code   string
			}{
				{"capture-architecture-missing", func(p *creationProvenance) { p.CaptureArch = "" }, provenanceFailure},
				{"capture-toolchain-missing", func(p *creationProvenance) { p.CaptureGoVersion = "" }, provenanceFailure},
				{"driver-substitution", func(p *creationProvenance) { p.CaptureDriver = "not-the-pinned-driver" }, provenanceFailure},
				{"declaration-substitution", func(p *creationProvenance) { p.CapturePolicyDeclaration = "attested" }, provenanceFailure},
				{"identity-missing", func(p *creationProvenance) { p.CapturePayloadIdentity = "" }, provenanceFailure},
				{"transformed-output", func(p *creationProvenance) {
					p.CapturePayloadIdentity = "transformed by capture sanitizer"
					p.CaptureRedactedFields = 6
					p.CaptureRedactedValues = 1
				}, transformedCaptureFailure},
				{"undeclared-transformation", func(p *creationProvenance) { p.CaptureRedactedValues = 1 }, provenanceFailure},
				{"mode-mismatch", func(p *creationProvenance) { p.ProbeActualMode = 0755 }, creationFailure},
				{"policy-mismatch", func(p *creationProvenance) { p.EffectiveUmask = "0022" }, creationFailure},
				{"windows-fallback", func(p *creationProvenance) { p.OS = "windows" }, "harness_parity_native_creation_policy_missing"},
			} {
				t.Run(change.name, func(t *testing.T) {
					before := processProvenance.Invocations
					altered := p
					change.mutate(&altered)
					if code := validateCreationContract(altered, osName, pins.arch, p.GoVersion); code != change.code || processProvenance.Invocations != before {
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
