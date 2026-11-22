# Legacy parity baseline refresh provenance

## Capture subject and source

The refreshed Linux and Darwin fixtures record the unchanged legacy implementation at source head
`267bbf477354f4f3d35c9530ec8da0503cd86b8a`. The sealed capture source is head
`44ff5d16c7dc6d2382981b83e14b0efd9ffddb85`, tree
`d72b3900459be99d1b407ef74c4cb3b9db942097`. Its only changes from the legacy base are the capture workflow and six
test sources:

- `.github/workflows/original-capture-native.yml`
- `testdata/originalcapture/ancestor_projection_test.go`
- `testdata/originalcapture/capture_homes_test.go`
- `testdata/originalcapture/capture_launchers_test.go`
- `testdata/originalcapture/controls_test.go`
- `testdata/originalcapture/launcher_fixture_test.go`
- `testdata/originalcapture/legacy_fixture_test.go`

The workflow file SHA-256 is `948e0cfe0ea0b5f6434b26a9ed64e5c1a6ebd7b9e1acb7035902adee29af1b1a`. The capture seal pins
these six source files:

| Capture source | SHA-256 |
|---|---|
| `testdata/originalcapture/ancestor_projection_test.go` | `03e153275c7ca9a7a83ade0705ae9d4aac8e4ee3d30a36e381b8f97dbaf1851f` |
| `testdata/originalcapture/capture_homes_test.go` | `35457d6728df2436292d85702621318d33b420b7d8aafd891cc96421be8d293b` |
| `testdata/originalcapture/capture_launchers_test.go` | `e202ab658c45b6ebaba731c2a72eda82de4cb94583b251d0f73054361e124518` |
| `testdata/originalcapture/controls_test.go` | `2928b6a0001dccfd39f430caa93ced8528b23653f4b3542103639eef09b908e4` |
| `testdata/originalcapture/launcher_fixture_test.go` | `deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983` |
| `testdata/originalcapture/legacy_fixture_test.go` | `b96b012122f3ecb70537e2f64ca5fefbbdf6225123ce11f3ff7d6fc038c3a6eb` |

## Sealed platform evidence

| Platform | Architecture / Go | Archive SHA-256 | Seal SHA-256 | Case manifest SHA-256 | Repeat manifest SHA-256 |
|---|---|---|---|---|---|
| Linux | `amd64` / `go1.25.5` | `bc2367c9d77da74f5dad48d107519042cfbd8829a6167197ee34cc459c78029e` | `bfd31772f93127efac7d8a689469cf6effe0fac6c43fd6fa8f57783c7978f56d` | `5a3c534a7c2ace839ccf939ce795f6a25a94622cdd5c8cecd2a2226dc2c8d1a8` | `7b5ff8a45e7cd2cb05bd4c524c3975ffcd5b6626f52273f733ba994825893d6c` |
| macOS | `arm64` / `go1.25.5` | `8e55fbb5849f76d1cc277002bc3a8395d922b9042e5ee83cba903d0850983ace` | `269654f39eac1bad06c253e916bbab5b6d4301a3d4238351901e4b76b0270187` | `612a02d752d4e8232ef0c483707b6b75eee64639d662403d4f86eecd10c7ec71` | `40d635560094090587939014376ccd0fe13833ab555e2ff74f9bc15715f9bca0` |
| Windows | `amd64` / `go1.25.5` | `2b3427c41ffa245be17cd88eba23f813c874f54fee044dba5f1e3d029135ad6c` | `16cd0f3f48f2e0ef9ed78982745c6b23a40fc1ad8cc46cf4f629cc11aec94956` | `c7d23d51b4773d20df6b5227ac5c9ff236e7a0f37f0787ea711f1b4205d97f65` | `9cadc6f24a23439c8d05fe498768a2d8bffffc73b212206c5661dbdba7aea8c2` |

All three archives, seals, output manifests, repeat manifests, 47 case payloads and six capture source pins were verified.
Linux and macOS record umask `0077` and probe-directory mode `0700`. The repeat records bind two distinct temporary
roots while omitting their paths and produce the same result digest per platform.

Windows evidence is captured and verified, and the Windows comparison follows its own amendment. This refresh adds
no Windows baseline and claims no Windows parity or complete three-platform C3 result.

## Linux and Darwin case comparison

The previous captures remain unchanged. For Linux, the old side is under
`internal/harnessparity/testdata/linux/capture-07/cases/`; for Darwin it is under
`internal/harnessparity/testdata/darwin/cases/`. The refreshed cases are under each platform's
`main-267bbf47/cases/` directory. A byte comparison found exactly two changed payloads per platform; the other 45 of
47 payloads on each platform are byte-identical.

| Platform | Case | Old fixture SHA-256 | Refreshed fixture SHA-256 | Causing main commits |
|---|---|---|---|---|
| Linux | `claude_code-empty` | `b8e48867c85314b5d34bb13065fe92716b636e30a5d07e7a2a43be8549ca6b2b` | `f95f37ae3e32b9fea3f6120392c1f7135476fce53e986785ca675467754bf894` | `f572da6540f7b902c9f72249701e9fe978aa2b36`, `4b37c035004e6fa690bd79965b74d003a951a871` |
| Linux | `claude_code-mcp` | `db8eba3ee09f06f9ce51cb770205f6b320991e8378f4d94ac972e6889246440b` | `e35d817370b289b23a44ab3498383330351d522ccedb984f93aacaaa1560e286` | `f572da6540f7b902c9f72249701e9fe978aa2b36`, `4b37c035004e6fa690bd79965b74d003a951a871` |
| Darwin | `claude_code-empty` | `c58df3df9cf782767181f02b17078c5c7db66fee4dd7403bafc4249f48e12715` | `d10a834b09e4fe67fb0389d46e49b4f3e20d69ebe6170e9eb96fb21f2e66c71c` | `f572da6540f7b902c9f72249701e9fe978aa2b36`, `4b37c035004e6fa690bd79965b74d003a951a871` |
| Darwin | `claude_code-mcp` | `5b596c4701067f447db8fd42e49775a02e12a84a0c0fb4724dc9c388a9f32d04` | `18c91b7c76435116b76a74c3da984a0088f9bd4682336ef906a65d2d64e4d0ae` | `f572da6540f7b902c9f72249701e9fe978aa2b36`, `4b37c035004e6fa690bd79965b74d003a951a871` |

The two Claude settings files now include the generated ancestor-memory exclusions. Their changed output is attributed
to the implementation and portability commits above; the later test-only commit `05383d71` is not a cause of the
payload change.

## Scoped ancestor projection

The accepted C15 revision applies only to the managed `settings.json` for `claude_code-empty` and `claude_code-mcp`,
and only to the top-level `claudeMdExcludes` array. Each case binds its home, profile `sample`, environment ID
`claude_code`, project and launch directory, and full settings path before product execution. The project and launch
directory are `<FIXTURE_ROOT>/project`; the home is `<FIXTURE_ROOT>/manager`. The harness records lexical and
canonical fixture-root spellings, proves that the project is outside every Git work tree, and refuses an unavailable
or uncertain proof.

The test-only oracle derives the complete expected emitted-path set from that case table and filesystem metadata,
independently of product output. The projector requires exactly one of every expected decoded value in one contiguous
span and replaces only that span with `${FIXTURE_ROOT_ANCESTORS}`. A missing, additional, duplicate, noncontiguous or
out-of-chain path refuses. No other array entry, key, file or byte is projected. Bytes outside the matched span and
the complete result after projection remain byte-identical. Order, JSON escape spelling, separators and whitespace
inside the matched span are not compared because the entire span is replaced; this is the only C5 serialization limit.

The capture and parity package carry identical projector bytes:

| Use | Path | Git blob | SHA-256 |
|---|---|---|---|
| Capture source | `testdata/originalcapture/ancestor_projection_test.go` | `a6ea3758aa3d5cabe7881959bb175a2dc5a899ce` | `03e153275c7ca9a7a83ade0705ae9d4aac8e4ee3d30a36e381b8f97dbaf1851f` |
| Parity consumer | `internal/harnessparity/ancestor_projection_test.go` | `a6ea3758aa3d5cabe7881959bb175a2dc5a899ce` | `03e153275c7ca9a7a83ade0705ae9d4aac8e4ee3d30a36e381b8f97dbaf1851f` |

The replay fixture drivers are byte-identical to their capture sources at head
`44ff5d16c7dc6d2382981b83e14b0efd9ffddb85`:

| Capture source | Replay source | Git blob | SHA-256 and scope |
|---|---|---|---|
| `testdata/originalcapture/legacy_fixture_test.go` | `internal/harnessparity/legacy_fixture_test.go` | `350d1c9f09d43beecd5d2b2c22109b7dee4d072b` | `b96b012122f3ecb70537e2f64ca5fefbbdf6225123ce11f3ff7d6fc038c3a6eb` (whole file) |
| `testdata/originalcapture/launcher_fixture_test.go` | `internal/harnessparity/launcher_fixture_test.go` | `694957de2215229459d5a6eea2afeaaa040002e9` | `deaefe19fadacd5d449176bfab0aa60d029c842f14c5070fbea7e9f0e539e983` (whole file) |
| `testdata/originalcapture/capture_homes_test.go:44-55` | `internal/harnessparity/capture_helpers_test.go` (`onlyHomeAndPathEnvironment`) | `f77056788ebb66069533d99e25d3ffe14fbb17c6` | `cad800194f71650a36aa53e4751adc947e5decf894a931a88503cd5bcfb6f397` (function text; full source file SHA-256 `35457d6728df2436292d85702621318d33b420b7d8aafd891cc96421be8d293b`) |

The parity lane pins both replay source files and pairs each pin with an
independent one-byte mutation control. The helper is the only definition copied
from `capture_homes_test.go`; its capture test is not copied.

## Exception inventory refresh

The original inventory contained 940 occurrences at `643e727edd2db21307ca6d8c0332935e33d50ac8`. The refreshed
active and frozen manifests each contain 963 occurrences at `267bbf477354f4f3d35c9530ec8da0503cd86b8a`. Their full
multiset delta on `(path, harness_id, line_sha256)` is exactly 23 additions; there are no removals, replacements,
duplicates or PR-owned source sites. The detector and ratchet comparison logic are unchanged. Each row belongs to the
Claude Code move (DESIGN-04 section 6, order 8); the `internal/claudememory` rows are assigned to that move by the
accepted amendment even though those paths are absent from the section 1.1 map.

| Path:line | Harness ID | Line SHA-256 | Causing main commit |
|---|---|---|---|
| `internal/claudememory/ancestors.go:20` | `claude_code` | `7f1e2a9ca497eb8bc9af56a6f0d71aa235b63272d5295d3695d604e913505069` | `4b37c035004e6fa690bd79965b74d003a951a871` |
| `internal/claudememory/ancestors.go:29` | `claude_code` | `b1f61ef0f6a8fde554a2d1ed00ac308065dce9c140032f93e2d33fdf16163226` | `4b37c035004e6fa690bd79965b74d003a951a871` |
| `internal/claudememory/ancestors.go:33` | `claude_code` | `af2a6a4a90545d1fb2f231c8613da7eea5956762ae2dda769d70d9328db2acee` | `4b37c035004e6fa690bd79965b74d003a951a871` |
| `internal/claudememory/ancestors.go:36` | `claude_code` | `7a7f48bd0102c1785fb6d748499b1b5dafce41932d29d0459fb7ace8586161cb` | `4b37c035004e6fa690bd79965b74d003a951a871` |
| `internal/claudememory/ancestors.go:47` | `claude_code` | `b24fdcafe8921684cb129d5793ca78aa22badb686089e62e005562cd9260f6ac` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/ancestors.go:141` | `claude_code` | `d5e715643cd078efef47611695666f2299a3501f6b9beb5df5e5cebbc85e47b4` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/ancestors.go:141` | `codex_cli` | `d5e715643cd078efef47611695666f2299a3501f6b9beb5df5e5cebbc85e47b4` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/ancestors.go:141` | `cursor` | `d5e715643cd078efef47611695666f2299a3501f6b9beb5df5e5cebbc85e47b4` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/ancestors.go:141` | `gemini` | `d5e715643cd078efef47611695666f2299a3501f6b9beb5df5e5cebbc85e47b4` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/ancestors.go:141` | `muse` | `d5e715643cd078efef47611695666f2299a3501f6b9beb5df5e5cebbc85e47b4` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/ancestors.go:141` | `opencode` | `d5e715643cd078efef47611695666f2299a3501f6b9beb5df5e5cebbc85e47b4` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/ancestors.go:141` | `pi` | `d5e715643cd078efef47611695666f2299a3501f6b9beb5df5e5cebbc85e47b4` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/ancestors.go:141` | `windsurf` | `d5e715643cd078efef47611695666f2299a3501f6b9beb5df5e5cebbc85e47b4` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/ancestors.go:170` | `claude_code` | `d325dad9b349b26564843b6ecc0febf4cf8c3b8dfb4a806b21c03fccc1abf47b` | `1f9f71673c5d14558226dc9daeeb6117f001b610` |
| `internal/claudememory/ancestors.go:174` | `claude_code` | `5245ffcb04f910f028d0106fd7283bb6e37635a33e444a4e59ba818d8400d038` | `1f9f71673c5d14558226dc9daeeb6117f001b610` |
| `internal/claudememory/ownership.go:77` | `claude_code` | `cbdd0a7c5bf60a539cf110cd2baa9c5d9a6bd57ac0658213de76748b8e27076f` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/ownership.go:122` | `claude_code` | `b76f15c9d17229fa1732549b255c4b7626bb97e6ae3009bd20fe6896c11a33f9` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/claudememory/project.go:23` | `claude_code` | `0d85fb334c76adb1c7600ee451977a25bc483b44f012febb2ee30c33a6f513c9` | `4b37c035004e6fa690bd79965b74d003a951a871` |
| `internal/claudememory/project.go:32` | `claude_code` | `b50d009c49997d426ceb3095a7e5934cc926e671b688a1d5ebebf9e1799687fb` | `4b37c035004e6fa690bd79965b74d003a951a871` |
| `internal/envprofile/claude_ancestor_settings.go:35` | `claude_code` | `384e0d5b4333f3006c6c3e390d98e8cc7c0c71e4d4ef58fe5b13fe9f395577b3` | `4b37c035004e6fa690bd79965b74d003a951a871` |
| `internal/envprofile/claude_ancestor_settings.go:85` | `claude_code` | `8e612b88655d72cb75b97a4eeafeb541a0b426b29e213a6e2961a811812349e5` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/envprofile/claude_ancestor_settings.go:163` | `claude_code` | `28e2fbff27bb0a0685039284ec0585cba9936824ef0b1bb9815282d4e79f3070` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |
| `internal/envprofile/claude_ancestor_settings.go:205` | `claude_code` | `8eadbb37ec3a7c02df02c9b1d7eb59b8133121abfeffa22153cf03b418721cf8` | `f572da6540f7b902c9f72249701e9fe978aa2b36` |

The initial bootstrap rebind uses source head `267bbf477354f4f3d35c9530ec8da0503cd86b8a` and frozen exception
digest `a186ea55e67809751365ae5d5d08e9af47af3d16fc629b682e1ee544cc03ae1c`. Later moves remain shrink-only; the
refreshed frozen inventory does not replenish removed allowances.

## Accepted contract inputs

The effective revision manifest and its files are pinned here for C6:

| Input path | SHA-256 |
|---|---|
| `in/p9/EFFECTIVE-REVISION-01.md` | `1e5f8d035bc6cf7917eefeec53e662b48b2d28b039cd9873b7f85fd2f00ffae1` |
| `in/p9/BASELINE-REVISION-01.md` | `4ec02b204e1903aa0acc658f4a43e6d9a814c7f5eaa754b491655f67c4ba490d` |
| `in/p9/BASELINE-REVISION-01-ERRATUM-01.md` | `9a1ce4ea1a8dff64858004d5a6f97d0fa6ebfc45bacf95a4a02dea013ce7beee` |
| `in/p9/BASELINE-REVISION-01-ADDENDUM-R4.md` | `b687043132e39356802027ecb9061b1dade5024a0cb62d2f41fdf67560598f76` |
| `in/p9/BASELINE-REVISION-01-ADDENDUM-R4B.md` | `a3dfcc4ed0cb63e4bf3dc426b30bd1d08f5e7c58460d167777680b28ee78406b` |
| `in/p9/BASELINE-REVISION-01-AMENDMENT-B1-R2.md` | `29a52c1d5ec8c185d6a5b90c2ade54ac412a3c13e2c7d000c58eff62b11ad0b6` |
| `in/c15r3/BASELINE-REVISION-01-AMENDMENT-C15-r3.md` | `0ce1718345bb9f67ff48d71974c2f203a8a2fd5da0685c76ffe993583c875720` |

The accepted C15 projection is limited to these two managed Claude settings outputs. C16 remains a separate
amendment; this provenance makes no Windows comparison claim.
