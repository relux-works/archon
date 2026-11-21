package ciworkflow

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func checkSanitizedArtifactUploads(data []byte) error {
	var wf struct {
		Jobs map[string]struct {
			Steps []step `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return err
	}
	uploads := 0
	seenPaths := map[string]bool{}
	for jobName, job := range wf.Jobs {
		for i, current := range job.Steps {
			if current.Uses != "actions/upload-artifact@v4" {
				continue
			}
			uploads++
			path := current.With["path"]
			if !strings.HasPrefix(path, ".temp/public-artifacts-") || !strings.HasSuffix(path, "/") ||
				!strings.Contains(path, "${{ github.run_id }}-${{ github.run_attempt }}-") {
				return fmt.Errorf("%s: artifact upload uses an unsanitized path", jobName)
			}
			if seenPaths[path] {
				return fmt.Errorf("%s: sanitized upload destination is reused", jobName)
			}
			seenPaths[path] = true
			found := false
			for _, prior := range job.Steps[:i] {
				if prior.If == "always()" && prior.Shell == "bash" &&
					strings.Contains(prior.Run, "go run ./tools/sanitize-ci-evidence -source .temp/") &&
					strings.Contains(prior.Run, "-destination .temp/public-artifacts-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT-") {
					_, destination, ok := strings.Cut(prior.Run, "-destination ")
					want := strings.TrimSuffix(path, "/")
					want = strings.ReplaceAll(want, "${{ github.run_id }}", "$GITHUB_RUN_ID")
					want = strings.ReplaceAll(want, "${{ github.run_attempt }}", "$GITHUB_RUN_ATTEMPT")
					if ok && destination == want {
						found = true
					}
				}
			}
			if !found {
				return fmt.Errorf("%s: upload has no preceding sanitizer step", jobName)
			}
		}
	}
	if uploads == 0 {
		return fmt.Errorf("workflow contains no artifact uploads")
	}
	return nil
}

func TestCIArtifactUploadsUseSanitizedCopies(t *testing.T) {
	good := workflowBytes(t)
	if err := checkSanitizedArtifactUploads(good); err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][2]string{
		"raw-upload-path":   {"path: .temp/public-artifacts-", "path: .temp/ci-evidence-"},
		"missing-sanitizer": {"go run ./tools/sanitize-ci-evidence", "echo skipped-sanitizer"},
		"wrong-destination": {"-destination .temp/public-artifacts-", "-destination .temp/ci-evidence-"},
	} {
		t.Run(name, func(t *testing.T) {
			planted := strings.Replace(string(good), pair[0], pair[1], 1)
			if planted == string(good) {
				t.Fatal("mutation did not apply")
			}
			if err := checkSanitizedArtifactUploads([]byte(planted)); err == nil {
				t.Fatal("unsanitized upload survived the control")
			}
		})
	}
}

func TestPostLandingArtifactUploadUsesSanitizedCopy(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/post-landing-mutation-tests.yml")
	if err != nil {
		t.Fatal("read workflow:", err)
	}
	if err := checkSanitizedArtifactUploads(data); err != nil {
		t.Fatal(err)
	}
}
