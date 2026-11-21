# Original-output parity

The parity subject is the unchanged legacy implementation at
`643e727edd2db21307ca6d8c0332935e33d50ac8`. Staged adapters are never used as
its rendering subject. A migration must explicitly switch its harness subject,
implement the corresponding operations and remove that harness's active source
exceptions.

The matrix contains 17 managed-home/skill cases and 30 launcher cases. It
executes all eight existing IDs, including actual project projection and native
no-mirror behavior. Managed cases cover fresh output, user settings, current and
modified output, missing, unreadable and malformed marker state, and native-origin
selection. The complete managed tree, manager-owned rendered bytes,
native/project trees before and after, modes, link targets, notices and refusal
outputs are compared. Legacy linked-native reconciliation is recorded as part
of the original behavior.

The launcher cases retain all rows from the four original config tests and two
original CLI tests. They capture edited configuration, typed/effective
projections and refusal outputs from original code. Invalid configuration stays
an input: its unchanged bytes are asserted in-process, and its digest is
recorded without exporting hostile fields as emitted configuration. The six
original witnesses remain unchanged. The original Codex seed-C corpus and both
launcher witness source files are digest-pinned by the manifest and execute in
the unchanged repository suites.

Platform evidence has its own manifest under `testdata/<GOOS>/`. A missing
platform baseline fails; it is never counted as a skipped or Linux-equivalent
case. Manifest entries pin the original source, capture-driver digests, actual
platform/toolchain, case bytes and existing corpus bytes. Expected values come
only from original-code execution. A transformed capture is evidence for
analysis and cannot qualify exact original-output parity. Tests have no regeneration/update mode.
Changing the frozen baseline requires an explicit reviewed contract revision.

The original Darwin capture supplies a fixture-owned external directory for
the native-parent case. The historical Linux capture instead supplied the
manager ancestor and recorded a credential-conflict refusal. The replay uses
the external directory; the unchanged Linux capsule therefore exposes an input
contract mismatch. Neither that failure nor cross-host mode differences qualify
native parity. Separate origin-boundary tests retain ancestor, managed and
aliased-origin rejection, paired with external-origin success. Include the
47-case sweep when running these controls so the unchanged execution-count
gate can assess the complete lane:

```sh
go test -tags harness_parity -count=1 ./internal/harnessparity -run '^(TestHomeByteIdentity|TestHomeByteIdentityMutationControl|TestAllAdaptersHaveBaselineCases|TestCaptureIndexRefusals|TestMacOriginalReplay|TestNativeParentFixtureExternalOrigin|TestNativeParentFixtureOriginBoundaries)$'
```

The only byte parameter is the exact disposable fixture-root address, represented
as `${FIXTURE_ROOT}`. No timestamps, hashes, fields or paths are removed to make
comparison pass. Fixture Git dates are fixed. Tests operate only on private
temporary managed/native/project homes and synthetic metadata targets; credential
links are not followed by the tree reader.

Go runs the comparator and its paired controls:

```sh
GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=harness_parity" go test -count=1 ./internal/harnessparity -run '^(TestHomeByteIdentity|TestHomeByteIdentityMutationControl|TestAllAdaptersHaveBaselineCases)$'
```

The one-byte controls mutate the actual rendered MCP bytes behind the home link
and a launcher projection. Both require the same comparator to name the exact
path and differing digests. Coverage counts actual successful executions, not
fixture names or registry rows. Run in isolated HOME/TMP and Go caches, and keep
local logs under ignored `.temp/`.


## Dedicated creation environment

Parity cases and their TestMain are compiled only with `harness_parity`.
The ordinary full suite runs without that tag and performs no parity cases;
it does not mark them skipped. The required dedicated invocation is:

```sh
GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=harness_parity" go test -count=1 ./internal/harnessparity -run '^(TestHomeByteIdentity|TestHomeByteIdentityMutationControl|TestAllAdaptersHaveBaselineCases)$'
```

The Linux and Darwin test processes establish umask077 before unchanged legacy API calls.
A private creation probe requests0777 and must observe0700; an effective policy
other than077 refuses with `harness_parity_environment_invalid` before any
legacy operation. Expected artifacts are never chmodded or normalized.
The original Linux077 is labelled "declared by run.sh, not attested". Its
historical 47-case capture remains pinned under `testdata/linux/` and is not
the active comparator because it used a different driver. Linux parity uses
the separate byte-preserving capture under `testdata/linux/capture-07/`, bound
to the unchanged source and its producer seal. Darwin's arm64 Go1.25.5 capture
declares0077 in its workflow and metadata; it did not record a creation probe.
The dedicated077 replay measures its own policy and provides consistency
evidence rather than historical attestation.

A separate private creation-environment.json receipt records OS, architecture,
current Go version, original reference source, distinct capture/replay driver
pins and the policy measured in the creating process. Required missing or
contradictory provenance refuses. Its sanitized `HARNESS_PARITY_RUN` output
contains the same record for the caller's evidence capture. A successful full
lane reports47/47 unique baseline rows and also reports total legacy invocations;
coverage and mutation checks intentionally repeat some rows.

The controls separately require022 typed failure with zero legacy calls, a
complete077 pass and an in-memory0644 golden-mode mutant rejected by the same
comparator. The expected files remain byte-for-byte unchanged. Darwin's47
supplied cases and their derived manifest hashes are bound separately from Linux.
Pure contract and fixture-integrity checks can run on either host without
claiming native replay. Darwin replay requires the captured arm64 architecture.
The earlier derived Darwin artifact's sanitizer deleted six fields and replaced
one value across seven cases. Its bytes are preserved for diagnosis;
`harness_parity_native_output_transformed` refuses parity before legacy calls.
The later original capture is admitted separately through its sealed producer
manifest and consumer provenance index. Neither a regenerated golden nor a
field exemption is inferred.

Windows capture retains47 successful case rows, but its repeat check failed.
The capture helper does not substitute slash-spelled disposable roots in
`claudeMdExcludes`, so the raw observation does not establish inherent output
nondeterminism independently of fixture relocation. The two repeat trees were
not exported. Windows has no admitted comparison policy here and fails with
`harness_parity_native_creation_policy_missing`; no Linux/Darwin policy or
field exemption substitutes for it. No native skip or golden update is used.
