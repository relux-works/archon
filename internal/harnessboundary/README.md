# Harness source boundaries

The Go checker scans production Go sources across the repository, including
project admission, lease, authority, runtime, context-package, helper-file,
manifest and closure boundaries. Test sources, testdata, ignored scratch,
Git metadata and the task-board store are not production scan inputs.
Concrete adapters, the API and the static registry have explicit ownership.

`testdata/source-ownership.json` maps all thirteen inventory rows to eleven
contract ownership groups, shared effect owners, capabilities and adapter
packages. It records intended migration ownership. The staged methods do not
yet implement their corresponding native surfaces.

The conservative legacy exception list was captured from original source head
`643e727edd2db21307ca6d8c0332935e33d50ac8`. Every allowance names a path,
canonical ID and exact source-line SHA-256. Counts are consumed one-to-one;
copying a permitted line cannot reuse its allowance. The frozen allowance bytes
are digest-pinned. Active exceptions must be counted subsets of both this frozen ceiling and the
previous accepted active set. Restoring a removed allowance is refused. Source hashes for the
original production inventory are retained separately. A move removes its own
active entries; it never expands the frozen baseline.

The checker folds string concatenations and local constants in ID comparisons
and cases. It scans native policy literals conservatively, including comments.
It reports concrete paths and lines. This is a source architecture aid, not a
proof about runtime-composed identifiers. New generated production sources or
generation directives fail closed until a generated-input scanning contract is
qualified. Exact-candidate review remains necessary for dynamic code and registry
changes.

The effect checker parses imports, aliases, selectors and directives. Adapters
and the value/format helpers have the exact pure import vocabulary. Filesystem
path inspection, output functions, unapproved helper edges, credentials, process
launch, network, reflection, unsafe, cgo and linkname are rejected. Plants are
parsed and never executed.

Run with Go:

```sh
go test -count=1 ./internal/harnessboundary ./internal/harnessprobe -run '^(TestNoHarnessBranches|TestAdapterEffectBoundary|TestAdapterSurfaceOwnership|TestAPIVersionRefusal|TestRegisteredProbePolicies|TestProbeEnvironmentPassThrough|TestProbeSpecRefusal)$'
```

The paired controls cover new/computed branches, duplicate occurrences,
exception growth, omitted packages, effect imports/calls, registration/API
versions, staged capability claims and the full closed probe tuple. Capture
local output under ignored `.temp/` in disposable homes and caches.

## Historical predecessor gate

Go and Git run the historical comparison. The local command requires an explicit
predecessor SHA and exact active-file SHA-256, and verifies that SHA against a
fresh `origin` observation of `refs/heads/main`:

```sh
go run ./internal/harnessboundary/cmd/harnessgate \
  --predecessor-sha <accepted-default-sha> \
  --predecessor-digest <accepted-active-file-sha256>
```

The command fetches that exact commit, verifies ancestry and materializes only
`internal/harnessboundary/testdata/exceptions.json` from it. Shallow checkouts
also fetch the actual checked-out head's complete history so ancestry can be
proved; this never chooses a different predecessor. SHA and digest receipts go
to stdout. Source/exception files remain unchanged. Missing, zero, unreachable,
head-selected or nonancestor CI inputs fail with
`harness_predecessor_input_missing`, exit 23. Binding and growth refusals exit 24.
Git error payloads, event JSON and argument values are not printed.

CI passes the predecessor through step environment from the target event:
`pull_request.base.sha`, `merge_group.base_sha`, or a main push's `before`.
The command checks it against the same projected event fields. Unsupported or
missing event inputs fail closed. A head-controlled descriptor/local mode
cannot replace the CI event. The one workflow step runs predecessor, source and
parity checks even when an earlier check fails, preserving the predecessor
refusal's exit code.

The initial introduction has a separate explicit authorization:
`--bootstrap-source 643e727edd2db21307ca6d8c0332935e33d50ac8`
and `--bootstrap-frozen-digest d400920c0956af1ad90b9e964828c6c9dea2fbe8786b3064d133d825e945f0e9`.
It applies only after the existing predecessor commit tree proves the active
path absent. An empty or failed read is not absence. Its explicit payload digest
is SHA-256 of empty bytes, distinguished from an existing file by that proven
absence. Local `--bootstrap` refuses a predecessor that already has the file.
Future CI runs compare the existing predecessor rather than taking bootstrap.

For an offline/archive check, pass `--predecessor-descriptor <path>` and an
independently admitted `--descriptor-digest <sha256>` together with the explicit
predecessor SHA and payload digest. Descriptor version 1 binds
`predecessor_sha`, `predecessor_digest`, `active_exists` and the exact
base64-encoded `active_payload` bytes. A proven-absence descriptor has no payload.
The descriptor-byte pin must come from the accepted external evidence;
hashing a head-created descriptor does not establish acceptance. There is no
implicit bootstrap or fallback when the descriptor/pins are missing.

The mandatory source gate includes paired historical, event-source, real Git,
archive, argument and state-preservation controls. The restoration control also
checks the diagnostic identifies a newly restored physical occurrence. All
fixtures use temporary repositories and private homes; they do not contact
providers or use signing/authentication channels. Capture local receipts under
ignored `.temp/`.
