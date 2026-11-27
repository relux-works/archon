package ciartifact

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type failingCloseEvidenceInput struct {
	*bytes.Reader
	info     os.FileInfo
	closeErr error
}

func (f *failingCloseEvidenceInput) Stat() (os.FileInfo, error) { return f.info, nil }

func (f *failingCloseEvidenceInput) Close() error { return f.closeErr }

func testAccountHomePrefix(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return `C:\fixture-account`
	}
	return "/home/fixture-account"
}

func accountHomeEnvironmentVariable() string {
	if runtime.GOOS == "windows" {
		return "USERPROFILE"
	}
	return "HOME"
}

func currentAccountHomePrefix(t *testing.T) string {
	t.Helper()
	account, err := user.Current()
	if err != nil || account == nil {
		t.Fatal("resolve operating-system account home")
	}
	home := filepath.Clean(account.HomeDir)
	if home == "" || !filepath.IsAbs(home) || filepath.Dir(home) == home {
		t.Fatal("operating-system account home is unusable")
	}
	return home
}

func sanitizeGoTestStreamForHomePrefix(data []byte, homePrefix string) ([]byte, counts, error) {
	return sanitizeContentWithHomePrefix(data, true, homePrefix)
}

func TestAccountHomePrefixUsesOperatingSystemAccount(t *testing.T) {
	want := currentAccountHomePrefix(t)
	t.Setenv(accountHomeEnvironmentVariable(), filepath.Join(t.TempDir(), "unrelated-home"))
	if got := accountHomePrefix(); got != want {
		t.Fatal("account-home prefix followed the HOME override instead of the operating-system account")
	}
}

// Claim: the stream sanitizer redacts the operating-system account-home prefix in every CI output label without losing event metadata.
func TestSanitizeGoTestStreamRedactsAccountHomeAfterEveryOutputLabel(t *testing.T) {
	homePrefix := currentAccountHomePrefix(t)
	t.Setenv(accountHomeEnvironmentVariable(), filepath.Join(t.TempDir(), "unrelated-home"))
	separator := string(os.PathSeparator)
	tmp := separator + "tmp" + separator
	tests := []struct {
		name   string
		output string
	}{
		{name: "managed-home-curator-3", output: "managed-home Form: Home:" + homePrefix + tmp + "curator-3" + separator + "TestNativeHome"},
		{name: "managed-home-curator-1", output: "managed-home Form: Home:" + homePrefix + tmp + "curator-1" + separator + "TestNativeHome"},
		{name: "adapter-codex-cli", output: "{Adapter:codex_cli Home:" + homePrefix + tmp + "fixture-run" + separator + "TestNativeHome"},
		{name: "environment-claude-code", output: "Environment:claude_code Native:" + homePrefix + tmp + "curator-3" + separator + "TestNativeHome"},
		{name: "environment-codex-cli", output: "Environment:codex_cli Native:" + homePrefix + tmp + "curator-3" + separator + "TestNativeHome"},
		{name: "environment-opencode", output: "Environment:opencode Native:" + homePrefix + tmp + "curator-3" + separator + "TestNativeHome"},
		{name: "environment-pi", output: "Environment:pi Native:" + homePrefix + tmp + "curator-3" + separator + "TestNativeHome"},
		{name: "codex-managed", output: "snapshot/codex Managed:" + homePrefix + tmp + "curator-3" + separator + "TestNativeHome"},
		{name: "native-managed", output: "Native: Managed:" + homePrefix + tmp + "curator-3" + separator + "TestNativeHome"},
		{name: "project-lease-double-slash", output: "project-lease/" + homePrefix + tmp + "fixture-run" + separator + "TestOwnershipRecovery"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event, err := json.Marshal(struct {
				Action  string
				Package string
				Output  string
			}{Action: "output", Package: "example/pkg", Output: test.output})
			if err != nil {
				t.Fatal("encode event fixture")
			}
			if residuals := accountHomePrefixResidualCount(event, homePrefix); residuals != 1 {
				t.Fatalf("planted account-home count = %d, want 1", residuals)
			}
			clean, found, err := sanitizeGoTestStream(append(event, '\n'))
			if err != nil {
				t.Fatal("sanitize event fixture")
			}
			var got struct {
				Action  string
				Package string
				Output  string
			}
			if err := json.Unmarshal(bytes.TrimSpace(clean), &got); err != nil {
				t.Fatal("decode sanitized event")
			}
			if got.Action != "output" || got.Package != "example/pkg" || !strings.Contains(got.Output, redactedPath) {
				t.Fatal("sanitization changed event metadata or omitted the path replacement")
			}
			if found.paths == 0 {
				t.Fatal("account-home replacement was not counted")
			}
			if residuals := accountHomePrefixResidualCount(clean, accountHomePrefix()); residuals != 0 {
				t.Fatalf("post-sanitize account-home residual count = %d, want 0", residuals)
			}
		})
	}
}

// Claim: Windows account-home paths are redacted in raw text, Go test JSON output, and forward-slash form on every host OS.
func TestSanitizeGoTestStreamRedactsWindowsAccountHomeForms(t *testing.T) {
	const windowsHome = `C:\Users\example`
	const slashWindowsHome = `C:/Users/example`
	const rawWindowsPath = windowsHome + `\tmp\fixture-run\TestNativeHome`
	tests := []struct {
		name       string
		homePrefix string
		output     string
		jsonEvent  bool
	}{
		{name: "raw_backslashes", homePrefix: slashWindowsHome, output: "Native: " + rawWindowsPath},
		{name: "go_test_json_escaped_backslashes", homePrefix: slashWindowsHome, output: "Native: " + rawWindowsPath, jsonEvent: true},
		{name: "forward_slashes", homePrefix: slashWindowsHome, output: "Native: " + `C:/Users/example/tmp/fixture-run/TestNativeHome`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var input []byte
			if test.jsonEvent {
				event, err := json.Marshal(struct {
					Action  string
					Package string
					Output  string
				}{Action: "output", Package: "example/pkg", Output: test.output})
				if err != nil {
					t.Fatal("encode JSON test event")
				}
				if !bytes.Contains(event, []byte(`C:\\Users\\example`)) {
					t.Fatal("JSON fixture did not contain escaped Windows separators")
				}
				input = append(event, '\n')
			} else {
				input = []byte(test.output)
			}

			clean, found, err := sanitizeGoTestStreamForHomePrefix(input, test.homePrefix)
			if err != nil {
				t.Fatal("sanitize Windows account-home fixture")
			}
			if !bytes.Contains(clean, []byte(redactedPath)) || found.paths == 0 {
				t.Fatal("Windows account-home path was not redacted and counted")
			}
			if !bytes.Contains(clean, []byte("Native:")) {
				t.Fatal("sanitization removed the output label")
			}
			if residuals := accountHomePrefixResidualCount(clean, test.homePrefix); residuals != 0 {
				t.Fatalf("post-sanitize Windows account-home residual count = %d, want 0", residuals)
			}
			if test.jsonEvent {
				var got struct {
					Action  string
					Package string
					Output  string
				}
				if err := json.Unmarshal(bytes.TrimSpace(clean), &got); err != nil {
					t.Fatal("decode sanitized JSON test event")
				}
				if got.Action != "output" || got.Package != "example/pkg" ||
					!strings.HasPrefix(got.Output, "Native:") || !strings.Contains(got.Output, redactedPath) {
					t.Fatal("sanitization changed event metadata or omitted the path replacement")
				}
			}
		})
	}
}

func TestAccountHomePrefixResidualCountCatchesPostSanitizeWrite(t *testing.T) {
	homePrefix := currentAccountHomePrefix(t)
	t.Setenv(accountHomeEnvironmentVariable(), filepath.Join(t.TempDir(), "unrelated-home"))
	clean, _, err := sanitizeGoTestStream([]byte(`{"Action":"output","Output":"safe witness"}`))
	if err != nil {
		t.Fatal("sanitize initial evidence")
	}
	finalBytes := append(append([]byte(nil), clean...), []byte("\nlate-write:"+homePrefix+string(os.PathSeparator)+"tmp/fixture/TestLateWrite")...)
	if residuals := accountHomePrefixResidualCount(finalBytes, accountHomePrefix()); residuals != 1 {
		t.Fatalf("post-write residual count = %d, want 1", residuals)
	}
}

func TestNonAccountHomeFixturePathRemainsUntouchedInJSONOutput(t *testing.T) {
	const accountHome = "/home/fixture-account"
	const nonAccountFixture = "/home/TestNativeHome/tmp/curator-3/TestFixture"
	input, err := json.Marshal(struct {
		Action string
		Output string
	}{Action: "output", Output: "managed-home Form: Home:" + nonAccountFixture})
	if err != nil {
		t.Fatal("encode non-account path fixture")
	}
	clean, found, err := sanitizeJSONStrings(input, true, accountHome)
	if err != nil {
		t.Fatal("sanitize non-account path fixture")
	}
	var got struct{ Output string }
	if err := json.Unmarshal(clean, &got); err != nil {
		t.Fatal("decode sanitized non-account fixture")
	}
	if got.Output != "managed-home Form: Home:"+nonAccountFixture || found.paths != 0 {
		t.Fatal("non-account /home fixture path was modified")
	}
}

func TestSanitizeJSONPreservesGoEventContract(t *testing.T) {
	hostPath := strings.Join([]string{"/", "home", "runner", "checkout", "internal", "fixture.go"}, "")
	input := `{"Time":"2026-01-02T03:04:05Z","Action":"output","Package":"github.com/relux-works/archon/internal/ciartifact","Test":"TestSanitizedEvidence","Elapsed":0.25,"Output":"at ` + hostPath + ` TOKEN=secret-value GOAMD64=v3 runner.internal.invalid 192.0.2.44 environment_store_untrusted: store entry failed ownership check: fixture"}`
	clean, found, err := sanitizeGoTestStream([]byte(input))
	if err != nil {
		t.Fatal("sanitize JSON:", err)
	}
	if !json.Valid(clean) {
		t.Fatal("sanitized event is invalid JSON")
	}
	rawPrefix, _, _ := strings.Cut(input, `,"Output":`)
	cleanPrefix, _, _ := strings.Cut(string(clean), `,"Output":`)
	if rawPrefix == "" || rawPrefix != cleanPrefix {
		t.Fatal("non-Output event bytes changed")
	}
	var event map[string]any
	if err := json.Unmarshal(clean, &event); err != nil {
		t.Fatal("decode sanitized event:", err)
	}
	for key, want := range map[string]any{
		"Time": "2026-01-02T03:04:05Z", "Action": "output", "Package": curatorModulePath + "/internal/ciartifact", "Test": "TestSanitizedEvidence", "Elapsed": 0.25,
	} {
		if event[key] != want {
			t.Errorf("event field %q changed: got %v, want %v", key, event[key], want)
		}
	}
	output, ok := event["Output"].(string)
	if !ok {
		t.Fatal("Output was not preserved as a string")
	}
	for _, forbidden := range []string{hostPath, "secret-value", "GOAMD64=v3", "runner.internal.invalid", "192.0.2.44"} {
		if strings.Contains(output, forbidden) {
			t.Errorf("sensitive fixture survived: %q", forbidden)
		}
	}
	for _, witness := range []string{"environment_store_untrusted:", "failed ownership check:"} {
		if !strings.Contains(output, witness) {
			t.Errorf("compatibility witness %q was removed", witness)
		}
	}
	if found.paths == 0 || found.assignments == 0 || found.hostnames == 0 || found.ipAddresses == 0 {
		t.Errorf("negative controls were not all exercised: %+v", found)
	}
}

func TestSanitizeGoTestStreamNormalizesUnsafeSubtestMetadata(t *testing.T) {
	input := []byte(
		`{"Action":"run","Package":"` + curatorModulePath + `/internal/ciartifact","Test":"TestSanitizedEvidence/worker.internal.invalid","Output":"safe witness"}` + "\n" +
			`{"Action":"run","Package":"` + curatorModulePath + `/internal/ciartifact","Test":"TestSanitizedEvidence/child-/home/fixture/private","Output":"safe witness"}` + "\n" +
			`{"Action":"run","Package":"` + curatorModulePath + `/internal/ciartifact","Test":"TestSanitizedEvidence/safe-child","Output":"safe witness"}` + "\n",
	)
	clean, found, err := sanitizeGoTestStream(input)
	if err != nil {
		t.Fatal("sanitize subtest metadata:", err)
	}
	var got []string
	for _, line := range bytes.Split(bytes.TrimSpace(clean), []byte("\n")) {
		if !json.Valid(line) {
			t.Fatalf("normalized event is invalid JSON: %s", line)
		}
		var event struct {
			Package string
			Test    string
		}
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal("decode normalized event:", err)
		}
		if event.Package != curatorModulePath+"/internal/ciartifact" {
			t.Fatalf("package metadata changed: %q", event.Package)
		}
		got = append(got, event.Test)
	}
	want := []string{"TestSanitizedEvidence/" + redactedSubtest, "TestSanitizedEvidence/" + redactedSubtest, "TestSanitizedEvidence/safe-child"}
	if len(got) != len(want) {
		t.Fatalf("normalized event count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("normalized test metadata %d = %q, want %q", i, got[i], want[i])
		}
	}
	for _, forbidden := range []string{"worker.internal.invalid", "/home/fixture/private"} {
		if bytes.Contains(clean, []byte(forbidden)) {
			t.Errorf("unsafe subtest metadata survived: %q", forbidden)
		}
	}
	if found.hostnames == 0 || found.paths == 0 {
		t.Fatalf("host and path controls were not both exercised: %+v", found)
	}
}

func TestSanitizeTextIsIdempotent(t *testing.T) {
	input := "https://worker.internal.invalid/private/data TOKEN=fixture-value 192.0.2.44"
	first, _ := sanitizeText(input)
	second, found := sanitizeText(first)
	if first != second {
		t.Fatal("sanitized output changed on a second pass")
	}
	if found != (counts{}) {
		t.Fatalf("second pass found residual sensitive values: %+v", found)
	}
}

func TestSanitizeJSONRedactsSensitiveObjectValues(t *testing.T) {
	fixturePath := "/" + strings.Join([]string{"private", "runner", "item"}, "/")
	mixedValue := "fixture-key-value " + fixturePath + " worker.internal.invalid"
	input := []byte(`{"evidence":{"api_key":"` + mixedValue + `","password":{"nested":"fixture-password-value"}},"safe":"keep this witness"}`)
	clean, _, err := sanitizeTextOrJSON(input)
	if err != nil {
		t.Fatal("sanitize JSON:", err)
	}
	for _, forbidden := range []string{"fixture-key-value", fixturePath, "worker.internal.invalid", "fixture-password-value"} {
		if strings.Contains(string(clean), forbidden) {
			t.Errorf("sensitive object value retained %q", forbidden)
		}
	}
	if !strings.Contains(string(clean), `"api_key":"[redacted credential]"`) || !strings.Contains(string(clean), `"password":"[redacted credential]"`) {
		t.Fatal("sensitive object value survived")
	}
	if !strings.Contains(string(clean), "keep this witness") {
		t.Fatal("safe evidence value was removed")
	}
}

func TestSanitizeGoTestStreamRedactsEscapedPath(t *testing.T) {
	hostPath := "/" + strings.Join([]string{"home", "runner", "checkout", "file.go"}, "/")
	escapedPath := strings.ReplaceAll(hostPath, "/", `\u002f`)
	input := []byte(`{"Action":"output","Package":"example/pkg","Output":"at ` + escapedPath + `:12"}`)
	clean, _, err := sanitizeGoTestStream(input)
	if err != nil {
		t.Fatal("sanitize escaped JSON:", err)
	}
	var event map[string]string
	if err := json.Unmarshal(clean, &event); err != nil {
		t.Fatal("decode sanitized event:", err)
	}
	if strings.Contains(event["Output"], hostPath) {
		t.Fatal("escaped path survived JSON decoding and redaction")
	}
}

func TestSanitizeGoTestStreamRedactsAccountHomePrefixAtAnyPosition(t *testing.T) {
	homePrefix := "fixture-run"
	separator := string(os.PathSeparator)

	tests := []struct {
		name    string
		output  string
		witness string
	}{
		{
			name:    "after-home-label-colon",
			output:  "entry={Adapter:codex_cli Home:" + homePrefix + separator + "tmp" + separator + "fixture-run" + separator + "TestNativeHome}",
			witness: "Home:",
		},
		{
			name:    "embedded-after-path-segment",
			output:  "target project-lease/" + homePrefix + separator + "tmp" + separator + "curator-1" + separator + "TestOwnershipRecovery",
			witness: "project-lease/",
		},
		{
			name:    "tmpdir-path-after-letter",
			output:  "TMPDIR" + homePrefix + separator + "tmp" + separator + "curator-3" + separator + "TestTemporaryDir",
			witness: "TMPDIR",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, err := json.Marshal(struct {
				Action  string
				Package string
				Output  string
			}{Action: "output", Package: "example/pkg", Output: test.output})
			if err != nil {
				t.Fatal("encode event fixture")
			}
			clean, found, err := sanitizeGoTestStreamForHomePrefix(append(input, '\n'), homePrefix)
			if err != nil {
				t.Fatal("sanitize event fixture")
			}
			if !json.Valid(clean) {
				t.Fatal("sanitized event is not valid JSON")
			}
			var got struct {
				Action  string
				Package string
				Output  string
			}
			if err := json.Unmarshal(bytes.TrimSpace(clean), &got); err != nil {
				t.Fatal("decode sanitized event")
			}
			if got.Action != "output" || got.Package != "example/pkg" {
				t.Fatal("sanitization changed Go test event metadata")
			}
			if strings.Contains(got.Output, homePrefix) {
				t.Fatal("account-home prefix survived in Go test Output")
			}
			if !strings.Contains(got.Output, test.witness) {
				t.Fatal("safe output witness was removed")
			}
			if found.paths+found.assignments == 0 {
				t.Fatal("account-home redaction was not counted")
			}
		})
	}
}

func TestSanitizeGoTestStreamRedactsAccountHomePrefixAfterEquals(t *testing.T) {
	homePrefix := testAccountHomePrefix(t)
	separator := string(os.PathSeparator)
	input, err := json.Marshal(struct {
		Action  string
		Package string
		Output  string
	}{Action: "output", Package: "example/pkg", Output: "TMPDIR=" + homePrefix + separator + "tmp" + separator + "curator-4" + separator + "TestTemporaryDir"})
	if err != nil {
		t.Fatal("encode event fixture")
	}
	clean, found, err := sanitizeGoTestStreamForHomePrefix(append(input, '\n'), homePrefix)
	if err != nil {
		t.Fatal("sanitize event fixture")
	}
	var got struct{ Output string }
	if err := json.Unmarshal(bytes.TrimSpace(clean), &got); err != nil {
		t.Fatal("decode sanitized event")
	}
	if strings.Contains(got.Output, homePrefix) {
		t.Fatal("account-home prefix survived after equals")
	}
	if !strings.Contains(got.Output, "TMPDIR=") || found.paths+found.assignments == 0 {
		t.Fatal("safe assignment witness was lost or redaction was not counted")
	}
}

func TestSanitizePlainTextAndUnparsedLines(t *testing.T) {
	hostPath := strings.Join([]string{"D:", `\`, "a", `\`, "checkout", `\`, "src", `\`, "item.go"}, "")
	input := []byte("not-json " + hostPath + " host=worker.internal.invalid API_TOKEN=fixture-secret PID=123\n")
	clean, found, err := sanitizeTextOrJSON(input)
	if err != nil {
		t.Fatal("sanitize text:", err)
	}
	for _, forbidden := range []string{hostPath, "worker.internal.invalid", "fixture-secret", "PID=123"} {
		if strings.Contains(string(clean), forbidden) {
			t.Errorf("sensitive fixture survived: %q", forbidden)
		}
	}
	if found.paths == 0 || found.hostnames == 0 || found.assignments == 0 {
		t.Errorf("text negative controls were not all exercised: %+v", found)
	}
}

func TestSanitizeWindowsHomePrefixFormsAcrossPlatforms(t *testing.T) {
	const homePrefix = `C:\fixture-account`
	tests := []struct {
		name string
		path string
	}{
		{name: "backslash", path: `C:\fixture-account\private.txt`},
		{name: "forward-slash", path: "C:/fixture-account/private.txt"},
		{name: "case-varied", path: `c:\FIXTURE-ACCOUNT\private.txt`},
		{name: "short-name-path", path: `C:\FIXTUR~1\private.txt`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			clean, found := sanitizeTextWithHomePrefix(test.path+" tail=visible", homePrefix)
			if found.paths == 0 || !strings.Contains(clean, redactedPath) {
				t.Fatal("Windows account-home path was not redacted")
			}
			if accountHomePrefixResidualCount([]byte(clean), homePrefix) != 0 {
				t.Fatal("Windows account-home path survived sanitization")
			}
			if !strings.Contains(clean, "tail=visible") {
				t.Fatal("safe text after the path was removed")
			}
		})
	}

	input := []byte(`{"Output":"C:\\fixture-account\\private.txt tail=visible","Action":"output"}`)
	clean, found, err := sanitizeJSONStrings(input, true, homePrefix)
	if err != nil {
		t.Fatal("sanitize JSON-escaped Windows path:", err)
	}
	if !json.Valid(clean) {
		t.Fatal("sanitized Windows event is not valid JSON")
	}
	var got struct {
		Output string
		Action string
	}
	if err := json.Unmarshal(clean, &got); err != nil {
		t.Fatal("decode sanitized Windows event:", err)
	}
	if got.Action != "output" || !strings.Contains(got.Output, redactedPath) || !strings.Contains(got.Output, "tail=visible") {
		t.Fatal("sanitization changed safe Windows event content")
	}
	if found.paths == 0 || accountHomePrefixResidualCount(clean, homePrefix) != 0 {
		t.Fatal("JSON-escaped Windows account-home path survived sanitization")
	}
}

func TestVerifyNoAccountHomePrefixRecognizesWindowsForms(t *testing.T) {
	const homePrefix = `C:\fixture-account`
	leaks := []struct {
		name string
		data []byte
	}{
		{name: "backslash", data: []byte(`C:\fixture-account\private.txt`)},
		{name: "forward-slash", data: []byte("C:/fixture-account/private.txt")},
		{name: "case-varied", data: []byte(`c:\FIXTURE-ACCOUNT\private.txt`)},
		{name: "JSON-escaped", data: []byte(`{"Output":"C:\\fixture-account\\private.txt"}`)},
	}
	for _, leak := range leaks {
		leak := leak
		t.Run(leak.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "evidence.txt"), leak.data, 0o600); err != nil {
				t.Fatal("write leak fixture:", err)
			}
			residuals, err := verifyNoAccountHomePrefixInTree(root, homePrefix)
			if err == nil || residuals != 1 {
				t.Fatalf("Windows residual check = (%d, %v), want one residual and refusal", residuals, err)
			}
		})
	}

	t.Run("safe-control", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "evidence.txt"), []byte("safe witness"), 0o600); err != nil {
			t.Fatal("write safe fixture:", err)
		}
		residuals, err := verifyNoAccountHomePrefixInTree(root, homePrefix)
		if err != nil || residuals != 0 {
			t.Fatalf("safe Windows residual check = (%d, %v), want zero and acceptance", residuals, err)
		}
	})
}

func TestSanitizeWindowsUNCAccountHomePrefixFormsAcrossPlatforms(t *testing.T) {
	const homePrefix = `\\server\share\fixture-account`
	tests := []struct {
		name string
		path string
	}{
		{name: "backslash", path: `\\server\share\fixture-account\private.txt`},
		{name: "forward-slash", path: "//server/share/fixture-account/private.txt"},
		{name: "case-varied", path: `\\SERVER\SHARE\FIXTURE-ACCOUNT\private.txt`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			clean, found := sanitizeTextWithHomePrefix(test.path+" tail=visible", homePrefix)
			if found.paths == 0 || !strings.Contains(clean, redactedPath) {
				t.Fatal("Windows UNC account-home path was not redacted")
			}
			if accountHomePrefixResidualCount([]byte(clean), homePrefix) != 0 {
				t.Fatal("Windows UNC account-home path survived sanitization")
			}
			if !strings.Contains(clean, "tail=visible") {
				t.Fatal("safe text after the UNC path was removed")
			}
		})
	}
}

func TestVerifyNoAccountHomePrefixRecognizesWindowsUNCForms(t *testing.T) {
	const homePrefix = `\\server\share\fixture-account`
	leaks := []struct {
		name string
		data []byte
	}{
		{name: "backslash", data: []byte(`\\server\share\fixture-account\private.txt`)},
		{name: "forward-slash", data: []byte("//server/share/fixture-account/private.txt")},
		{name: "case-varied", data: []byte(`\\SERVER\SHARE\FIXTURE-ACCOUNT\private.txt`)},
	}
	for _, leak := range leaks {
		leak := leak
		t.Run(leak.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "evidence.txt"), leak.data, 0o600); err != nil {
				t.Fatal("write UNC leak fixture:", err)
			}
			residuals, err := verifyNoAccountHomePrefixInTree(root, homePrefix)
			if err == nil || residuals != 1 {
				t.Fatalf("Windows UNC residual check = (%d, %v), want one residual and refusal", residuals, err)
			}
		})
	}
}

func TestSanitizeTreeKeepsSourceAndRejectsExistingDestination(t *testing.T) {
	homePrefix := testAccountHomePrefix(t)
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "sanitized")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal("create source:", err)
	}
	fixturePath := filepath.Join(homePrefix, "private.txt")
	original, err := json.Marshal(struct {
		Output string
		Action string
	}{Output: fixturePath + " token=fixture-secret", Action: "output"})
	if err != nil {
		t.Fatal("encode source fixture:", err)
	}
	original = append(original, byte(10))
	if err := os.WriteFile(filepath.Join(source, "go-test.json"), original, 0o600); err != nil {
		t.Fatal("write source:", err)
	}
	counts, err := sanitizeTreeWithHomePrefix(source, destination, os.RemoveAll, openEvidenceInput, homePrefix, nil, nil)
	if err != nil {
		t.Fatal("sanitize tree:", err)
	}
	if counts.Files != 1 || counts.Paths == 0 || counts.Credentials == 0 || counts.AccountHomePrefixResiduals != 0 {
		t.Fatalf("unexpected sanitizer totals: %+v", counts)
	}
	gotSource, err := os.ReadFile(filepath.Join(source, "go-test.json"))
	if err != nil {
		t.Fatal("read source:", err)
	}
	if string(gotSource) != string(original) {
		t.Fatal("source evidence was modified")
	}
	gotClean, err := os.ReadFile(filepath.Join(destination, "go-test.json"))
	if err != nil {
		t.Fatal("read sanitized copy:", err)
	}
	if strings.Contains(string(gotClean), fixturePath) || strings.Contains(string(gotClean), "fixture-secret") {
		t.Fatal("sanitized destination retained a forbidden value")
	}
	if !json.Valid(bytes.TrimSpace(gotClean)) {
		t.Fatal("sanitized destination is not valid JSON")
	}
	if _, err := SanitizeTree(source, destination); err == nil {
		t.Fatal("existing destination was accepted")
	}
	afterRefusal, err := os.ReadFile(filepath.Join(destination, "go-test.json"))
	if err != nil || !bytes.Equal(afterRefusal, gotClean) {
		t.Fatal("refusing an existing destination changed or removed it")
	}
}

func TestSanitizeTreeRejectsAccountHomeResidualWrittenAfterCopy(t *testing.T) {
	homePrefix := testAccountHomePrefix(t)
	latePath := filepath.Join(homePrefix, "tmp", "fixture", "TestLateWrite")
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "sanitized")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal("create source:", err)
	}
	original := []byte(`{"Action":"output","Output":"safe witness"}` + "\n")
	if err := os.WriteFile(filepath.Join(source, "go-test.json"), original, 0o600); err != nil {
		t.Fatal("write source evidence:", err)
	}
	afterCopy := func(stage string) error {
		file, err := os.OpenFile(filepath.Join(stage, "go-test.json"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		_, writeErr := file.WriteString("late-write:" + latePath)
		closeErr := file.Close()
		return errors.Join(writeErr, closeErr)
	}
	counts, err := sanitizeTreeWithHomePrefix(source, destination, os.RemoveAll, openEvidenceInput, homePrefix, afterCopy, nil)
	if err == nil || counts.AccountHomePrefixResiduals != 1 {
		t.Fatalf("late-write verification = (%d, %v), want one residual and refusal", counts.AccountHomePrefixResiduals, err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("late account-home residual published a destination")
	}
}

func TestSanitizeTreeRejectsResidualBetweenStagedAndDestinationScans(t *testing.T) {
	homePrefix := testAccountHomePrefix(t)
	latePath := filepath.Join(homePrefix, "tmp", "fixture", "TestAfterStageScan")
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "sanitized")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal("create source:", err)
	}
	original := []byte(`{"Action":"output","Output":"safe witness"}` + "\n")
	if err := os.WriteFile(filepath.Join(source, "go-test.json"), original, 0o600); err != nil {
		t.Fatal("write source evidence:", err)
	}
	afterPublish := func(published string) error {
		file, err := os.OpenFile(filepath.Join(published, "go-test.json"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		_, writeErr := file.WriteString("late-write:" + latePath)
		closeErr := file.Close()
		return errors.Join(writeErr, closeErr)
	}
	counts, err := sanitizeTreeWithHomePrefix(source, destination, os.RemoveAll, openEvidenceInput, homePrefix, nil, afterPublish)
	if err == nil || counts.AccountHomePrefixResiduals != 1 {
		t.Fatalf("post-stage-scan verification = (%d, %v), want one residual and refusal", counts.AccountHomePrefixResiduals, err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("post-stage-scan residual left a published destination")
	}
}

func TestSanitizeTreeFailsClosedWithoutUsableAccountHomePrefix(t *testing.T) {
	rootHome := filepath.VolumeName(os.TempDir()) + string(os.PathSeparator)
	tests := []struct {
		name string
		home string
	}{
		{name: "relative", home: "relative-fixture-home"},
		{name: "filesystem-root", home: rootHome},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source")
			destination := filepath.Join(root, "sanitized")
			if err := os.Mkdir(source, 0o755); err != nil {
				t.Fatal("create evidence source")
			}
			if _, err := sanitizeTreeWithHomePrefix(source, destination, os.RemoveAll, openEvidenceInput, test.home, nil, nil); err == nil {
				t.Fatal("sanitizer accepted an unusable account-home prefix")
			}
			if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("sanitizer published evidence without a usable account-home prefix")
			}
		})
	}
}

func TestVerifyNoAccountHomePrefixFailsClosed(t *testing.T) {
	const homePrefix = "/home/fixture-account"
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "safe.txt"), []byte("safe witness"), 0o600); err != nil {
		t.Fatal("write safe evidence fixture")
	}
	if err := os.WriteFile(filepath.Join(root, "evidence.json"), []byte(`{"Output":"`+homePrefix+`/tmp/fixture"}`), 0o600); err != nil {
		t.Fatal("write leaking evidence fixture")
	}
	residuals, err := verifyNoAccountHomePrefixInTree(root, homePrefix)
	if err == nil {
		t.Fatal("residual account-home prefix was accepted")
	}
	if residuals != 1 {
		t.Fatalf("account-home residual count = %d, want 1", residuals)
	}
}

func TestReadSanitizedEvidenceFileRefusesOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("outside staged root"), 0o600); err != nil {
		t.Fatal("write outside-root fixture:", err)
	}

	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal("open staged root:", err)
	}
	t.Cleanup(func() {
		if err := rootFS.Close(); err != nil {
			t.Errorf("close staged root: %v", err)
		}
	})

	if _, err := readSanitizedEvidenceFile(rootFS, root, outsidePath); err == nil {
		t.Fatal("outside-root evidence path was accepted")
	}
}

func TestSanitizeTreeRejectsOverlappingAndSymlinkInputs(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal("create source:", err)
	}
	if _, err := SanitizeTree(source, filepath.Join(source, "nested")); err == nil {
		t.Fatal("overlapping destination was accepted")
	}
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(source, "link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := SanitizeTree(source, filepath.Join(root, "sanitized")); err == nil {
		t.Fatal("symlink entry was accepted")
	}
}

func TestSanitizeTreeRejectsBinaryContent(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "sanitized")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal("create source:", err)
	}
	if err := os.WriteFile(filepath.Join(source, "evidence.bin"), []byte{0, 1, 2}, 0o600); err != nil {
		t.Fatal("write binary fixture:", err)
	}
	if _, err := SanitizeTree(source, destination); err == nil {
		t.Fatal("binary evidence was copied without inspection")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("failed sanitization left a destination behind")
	}
}

func TestSanitizeTreeDoesNotPublishWhenEvidenceCloseFails(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "sanitized")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal("create source:", err)
	}
	original := []byte(`{"Output":"token=fixture-secret","Action":"output"}` + "\n")
	if err := os.WriteFile(filepath.Join(source, "go-test.json"), original, 0o600); err != nil {
		t.Fatal("write source:", err)
	}
	closeErr := errors.New("injected close failure")
	openInput := func(_ *os.Root, relative string) (evidenceInput, error) {
		info, err := os.Stat(filepath.Join(source, relative))
		if err != nil {
			return nil, err
		}
		return &failingCloseEvidenceInput{
			Reader:   bytes.NewReader(original),
			info:     info,
			closeErr: closeErr,
		}, nil
	}
	_, err := sanitizeTree(source, destination, os.RemoveAll, openInput)
	if !errors.Is(err, errEvidenceFileClose) {
		t.Fatalf("close failure was not reported: %v", err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("close failure published a destination: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal("read staging parent:", err)
	}
	if len(entries) != 1 || entries[0].Name() != "source" {
		t.Fatalf("close failure left staging output behind: %v", entries)
	}
}

func TestSanitizeTreeDoesNotPublishWhenStageRemovalFails(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "sanitized")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal("create source:", err)
	}
	if err := os.WriteFile(filepath.Join(source, "evidence.bin"), []byte{0, 1, 2}, 0o600); err != nil {
		t.Fatal("write binary fixture:", err)
	}
	var stagedPath string
	t.Cleanup(func() {
		if stagedPath != "" {
			if err := os.RemoveAll(stagedPath); err != nil {
				t.Errorf("remove test staging directory: %v", err)
			}
		}
	})
	removeErr := errors.New("injected staging removal failure")
	removeStage := func(path string) error {
		stagedPath = path
		return removeErr
	}
	_, err := sanitizeTree(source, destination, removeStage, openEvidenceInput)
	if !errors.Is(err, errEvidenceStageRemove) || !strings.Contains(err.Error(), "binary content") {
		t.Fatalf("sanitization and cleanup failures were not reported: %v", err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stage removal failure published a destination: %v", err)
	}
}

func TestSanitizeTreeSuccessfulPublishSkipsStageCleanup(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "sanitized")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal("create source:", err)
	}
	original := []byte(`{"Action":"output","Output":"safe evidence"}` + "\n")
	if err := os.WriteFile(filepath.Join(source, "go-test.json"), original, 0o600); err != nil {
		t.Fatal("write source evidence:", err)
	}
	removeCalls := 0
	removeStage := func(string) error {
		removeCalls++
		return errors.New("unexpected cleanup after successful publish")
	}
	if _, err := sanitizeTree(source, destination, removeStage, openEvidenceInput); err != nil {
		t.Fatalf("successful publish reported a stage cleanup error: %v", err)
	}
	if removeCalls != 0 {
		t.Fatalf("published stage was cleaned up %d times", removeCalls)
	}
	clean, err := os.ReadFile(filepath.Join(destination, "go-test.json"))
	if err != nil {
		t.Fatal("read published evidence:", err)
	}
	if !strings.Contains(string(clean), "safe evidence") {
		t.Fatalf("published evidence is missing the safe witness: %s", clean)
	}
}

func TestSanitizeTreeRefusesForbiddenNonOutputEventIdentifiers(t *testing.T) {
	tests := []struct {
		name        string
		packagePath string
		testName    string
	}{
		{
			name:        "account path in test name",
			packagePath: curatorModulePath + "/internal/ciartifact",
			testName:    "/home/fixture/account/private",
		},
		{
			name:        "host name in package",
			packagePath: curatorModulePath + "/runner.internal.invalid",
			testName:    "TestSafeMetadata",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source")
			destination := filepath.Join(root, "sanitized")
			if err := os.Mkdir(source, 0o755); err != nil {
				t.Fatal("create source:", err)
			}
			event, err := json.Marshal(map[string]string{
				"Action":  "run",
				"Output":  "safe output",
				"Package": test.packagePath,
				"Test":    test.testName,
				"Time":    "2026-01-02T03:04:05Z",
			})
			if err != nil {
				t.Fatal("encode event fixture:", err)
			}
			if err := os.WriteFile(filepath.Join(source, "go-test.json"), append(event, '\n'), 0o600); err != nil {
				t.Fatal("write event fixture:", err)
			}
			if _, err := SanitizeTree(source, destination); err == nil {
				t.Fatal("forbidden non-Output event identifier was accepted")
			}
			if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("refused event published a destination: %v", err)
			}
		})
	}
}
