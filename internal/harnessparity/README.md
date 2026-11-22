# Original-output parity

The parity subject is the unchanged legacy implementation at
`267bbf477354f4f3d35c9530ec8da0503cd86b8a`. Staged adapters are never used as
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

The active Linux and Darwin evidence has separate manifests under
`testdata/<GOOS>/main-267bbf47/`. A missing platform baseline fails; it is never
counted as a skipped or equivalent case. The manifests pin the source head,
capture drivers, sealed artifacts, platform/toolchain, case bytes, repeat
observations and existing corpus bytes. Expected values come only from
original-code execution. A transformed capture cannot qualify exact
original-output parity. Tests have no regeneration/update mode. Changing the
frozen baseline requires an explicit reviewed contract revision.

The refreshed capture projects only the generated `claudeMdExcludes` entries in
the managed `settings.json` for `claude_code-empty` and `claude_code-mcp` to
`${FIXTURE_ROOT_ANCESTORS}`. The shared projector validates the prebound case
table and exact contiguous membership before the existing fixture-root
substitution. All bytes outside that matched span remain subject to byte
identity and the unchanged sanitizer. The Linux and Darwin captures changed
only those two settings cases; the other 45 cases on each platform remain
byte-identical to their prior capture.

The untagged replay fixture drivers are byte-identical to capture commit
`44ff5d16c7dc6d2382981b83e14b0efd9ffddb85`:

| Replay source | Capture blob | SHA-256 |
|---|---|---|
| `internal/harnessparity/legacy_fixture_test.go` | `350d1c9f09d43beecd5d2b2c22109b7dee4d072b` | `b96b012122f3ecb70537e2f64ca5fefbbdf6225123ce11f3ff7d6fc038c3a6eb` |
| `internal/harnessparity/launcher_fixture_test.go` | `694957de2215229459d5a6eea2afeaaa040002e9` | `deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983` |

The projector remains untagged. The two fixture files supply its case tables in
the ordinary package build; the 47-case capture/replay comparator remains
dedicated to the `harness_parity` build tag.

The historical Darwin capture supplies a fixture-owned external directory for
the native-parent case. The older Linux captures remain preserved for
comparison. The current Linux and Darwin baselines use the same external-origin
fixture contract. Separate origin-boundary tests retain ancestor, managed and
aliased-origin rejection, paired with external-origin success. Include the
47-case sweep when running these controls so the unchanged execution-count gate
can assess the complete lane:

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

The 47-case capture/replay comparator and its TestMain are compiled only with
`harness_parity`. The ordinary package build runs the untagged projector
contract tests but not that comparator. The required dedicated invocation is:

```sh
GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=harness_parity" go test -count=1 ./internal/harnessparity -run '^(TestHomeByteIdentity|TestHomeByteIdentityMutationControl|TestAllAdaptersHaveBaselineCases)$'
```

The Linux and Darwin test processes establish umask077 before unchanged legacy
API calls. A private creation probe requests0777 and must observe0700; an
effective policy other than077 refuses with
`harness_parity_environment_invalid` before any legacy operation. Expected
artifacts are never chmodded or normalized. The refreshed Linux amd64 and
Darwin arm64 captures used Go1.25.5 and record umask077 and probe mode0700 in
their export seals. The replay measures its own policy and provides consistency
evidence rather than historical attestation. Earlier Linux and Darwin baselines
remain pinned at their original paths.

A separate private creation-environment.json receipt records OS, architecture,
current Go version, original reference source, distinct capture/replay driver
pins and the policy measured in the creating process. Required missing or
contradictory provenance refuses. Its sanitized `HARNESS_PARITY_RUN` output
contains the same record for the caller's evidence capture. A successful full
lane reports47/47 unique baseline rows and also reports total legacy invocations;
coverage and mutation checks intentionally repeat some rows.

The controls separately require022 typed failure with zero legacy calls, a
complete077 pass and an in-memory0644 golden-mode mutant rejected by the same
comparator. The expected files remain byte-for-byte unchanged. Linux and
Darwin's47 supplied cases and their derived manifest hashes are bound
separately. Pure contract and fixture-integrity checks can run on either host
without claiming native replay. Darwin replay requires the captured arm64
architecture. The earlier derived Darwin artifact's sanitizer deleted six
fields and replaced one value across seven cases. Its bytes remain preserved
for diagnosis; `harness_parity_native_output_transformed` refuses parity before
legacy calls. The original captures are admitted through their sealed producer
manifests and consumer provenance indexes. No regenerated golden or field
exemption is inferred.

Windows evidence is captured and verified, and the Windows comparison follows
its own amendment. No Windows baseline or parity result is claimed by this
refresh.
