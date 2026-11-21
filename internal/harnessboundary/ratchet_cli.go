package harnessboundary

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
)

// RatchetMissingExit distinguishes missing or unavailable predecessor evidence.
const RatchetMissingExit = 23

type ratchetOptions struct {
	sha, expectedDigest, descriptor, descriptorDigest string
	bootstrapSource, bootstrapDigest                  string
	ci, forceBootstrap                                bool
}

func parseRatchetOptions(args []string) (ratchetOptions, error) {
	var options ratchetOptions
	flags := flag.NewFlagSet("harnessgate", flag.ContinueOnError)
	// Flag errors may contain untrusted argument values. Emit typed codes only.
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.sha, "predecessor-sha", "", "bound predecessor commit")
	flags.StringVar(&options.expectedDigest, "predecessor-digest", "", "bound active payload digest")
	flags.StringVar(&options.descriptor, "predecessor-descriptor", "", "explicit verified archive descriptor")
	flags.StringVar(&options.descriptorDigest, "descriptor-digest", "", "independent descriptor-byte digest")
	flags.StringVar(&options.bootstrapSource, "bootstrap-source", "", "explicit original-source authorization")
	flags.StringVar(&options.bootstrapDigest, "bootstrap-frozen-digest", "", "explicit frozen-byte authorization")
	flags.BoolVar(&options.ci, "ci", false, "trusted target event mode")
	flags.BoolVar(&options.forceBootstrap, "bootstrap", false, "request bootstrap; refuses an existing predecessor file")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return options, ratchetError(RatchetMissingInput)
	}
	return options, nil
}

func predecessorFromOptions(ctx context.Context, root string, options ratchetOptions, getenv func(string) string) (VerifiedPredecessor, error) {
	if options.ci {
		if options.descriptor != "" || options.descriptorDigest != "" || getenv("GITHUB_ACTIONS") != "true" {
			return VerifiedPredecessor{}, ratchetError(RatchetMissingInput)
		}
		data, err := boundedFile(getenv("GITHUB_EVENT_PATH"))
		if err != nil {
			return VerifiedPredecessor{}, err
		}
		// Project only the required target-event fields; never log the payload.
		var event targetEvent
		if json.Unmarshal(data, &event) != nil {
			return VerifiedPredecessor{}, ratchetError(RatchetMissingInput)
		}
		claimed := getenv("HARNESS_PREDECESSOR_SHA")
		sha, err := selectTargetPredecessor(getenv("GITHUB_EVENT_NAME"), getenv("GITHUB_REPOSITORY"), getenv("GITHUB_REF"), claimed, event)
		if err != nil {
			return VerifiedPredecessor{}, err
		}
		if options.sha != "" && options.sha != sha {
			return VerifiedPredecessor{}, ratchetError(RatchetMissingInput)
		}
		head := getenv("GITHUB_SHA")
		if !validHex(head, 40) {
			return VerifiedPredecessor{}, ratchetError(RatchetMissingInput)
		}
		return materializePredecessor(ctx, root, sha, options.expectedDigest, head, false)
	}
	if getenv("GITHUB_ACTIONS") == "true" {
		return VerifiedPredecessor{}, ratchetError(RatchetMissingInput)
	}
	if options.descriptor != "" {
		return readArchivePredecessor(options.descriptor, options.descriptorDigest, options.sha, options.expectedDigest)
	}
	if options.descriptorDigest != "" || !validHex(options.sha, 40) || !validHex(options.expectedDigest, 64) {
		return VerifiedPredecessor{}, ratchetError(RatchetMissingInput)
	}
	return materializePredecessor(ctx, root, options.sha, options.expectedDigest, "", true)
}

func ratchetDiagnostic(root string, failure *RatchetError, output io.Writer) {
	if failure.exception == nil {
		return
	}
	data, err := boundedRootFile(root, "internal/harnessboundary/testdata/vocabulary.json")
	if err != nil {
		return
	}
	var vocabulary map[string][]string
	if json.Unmarshal(data, &vocabulary) != nil {
		return
	}
	findings, err := ScanBranches(root, vocabulary)
	if err != nil {
		return
	}
	for _, finding := range ApplyExceptions(findings, failure.allowances) {
		if finding.Path == failure.exception.Path && finding.HarnessID == failure.exception.HarnessID && finding.LineSHA256 == failure.exception.LineSHA256 {
			_, _ = fmt.Fprintln(output, finding.String())
			return
		}
	}
}

// RunRatchetCLI checks an explicitly bound predecessor before accepting the
// current exceptions. It reads target event metadata through the supplied
// environment projection, writes no source/exception files and emits no raw
// event, environment, Git error, archive payload or argument values.
func RunRatchetCLI(ctx context.Context, args []string, root string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, _ = fmt.Fprintln(stdout, "harnessgate --predecessor-sha SHA --predecessor-digest SHA256 [--bootstrap --bootstrap-source SHA --bootstrap-frozen-digest SHA256] [--predecessor-descriptor PATH --descriptor-digest SHA256]; CI: --ci with trusted target-event environment")
		return 0
	}
	options, err := parseRatchetOptions(args)
	var previous VerifiedPredecessor
	if err == nil {
		previous, err = predecessorFromOptions(ctx, root, options, getenv)
	}
	if err == nil {
		_, _ = fmt.Fprintf(stdout, "predecessor_sha=%s predecessor_digest=%s active_exists=%t\n", previous.sha, previous.digest, previous.exists)
		var frozen, active ExceptionSet
		if options.forceBootstrap && previous.exists {
			err = ratchetError(RatchetBootstrapForbidden)
		}
		if err == nil {
			frozen, active, err = readCurrentExceptionSets(root)
		}
		if err == nil {
			binding := BootstrapBinding{}
			// CI authorizes the explicit original binding only after Git has
			// proven absence. Normal future runs use the historical active set.
			if options.forceBootstrap || !previous.exists || !options.ci && (options.bootstrapSource != "" || options.bootstrapDigest != "") {
				binding = BootstrapBinding{options.bootstrapSource, options.bootstrapDigest}
			}
			err = CheckRatchet(frozen, active, previous, binding)
		}
	}
	if err != nil {
		var failure *RatchetError
		if !errors.As(err, &failure) {
			failure = &RatchetError{code: RatchetMissingInput}
		}
		_, _ = fmt.Fprintln(stderr, failure.Code())
		if failure.operation != "" {
			_, _ = fmt.Fprintf(stderr, "operation=%s\n", failure.operation)
		}
		ratchetDiagnostic(root, failure, stderr)
		if failure.Code() == RatchetMissingInput {
			return RatchetMissingExit
		}
		return 24
	}
	_, _ = fmt.Fprintln(stdout, "harness_exception_ratchet=accepted")
	return 0
}
