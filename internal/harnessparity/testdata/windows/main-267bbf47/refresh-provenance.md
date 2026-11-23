# Windows refreshed capture provenance

This index records the immutable Windows capture used as the PR0 comparison side. The 47 case files and two repeat observations are the exact exported bytes. They are not regenerated from the PR0 replay.

## Capture pins

- Legacy source base: `267bbf477354f4f3d35c9530ec8da0503cd86b8a`.
- Capture run: `45005829041`; capture driver and workflow head: `44ff5d16c7dc6d2382981b83e14b0efd9ffddb85`; tree: `d72b3900459be99d1b407ef74c4cb3b9db942097`.
- Workflow SHA-256: `948e0cfe0ea0b5f6434b26a9ed64e5c1a6ebd7b9e1acb7035902adee29af1b1a`.
- Runner label: `windows-latest`. The accepted seal does not bind the runner image version or host patch-level digest; no image version is asserted here.
- Tool versions: Go `1.25.5`; Git `2.56.1`.
- Artifact `original-capture-06-windows-latest` (ID `14747249311`), ZIP SHA-256 `2b3427c41ffa245be17cd88eba23f813c874f54fee044dba5f1e3d029135ad6c`.
- Export seal SHA-256: `16cd0f3f48f2e0ef9ed78982745c6b23a40fc1ad8cc46cf4f629cc11aec94956`.
- Output manifest SHA-256: `c7d23d51b4773d20df6b5227ac5c9ff236e7a0f37f0787ea711f1b4205d97f65`; 47 cases. Repeat manifest SHA-256: `9cadc6f24a23439c8d05fe498768a2d8bffffc73b212206c5661dbdba7aea8c2`; two observations.
- Capture driver file hashes are pinned individually in `manifest.json`; the seal verifies these six files and does not assert the full source tree.
- The seal records `umask` and `probe_directory_mode` as `not-applicable`. It contains no architecture field; this index makes no capture-architecture assertion.

The seal does not bind the runner image or patch level, tool executable bytes beyond the normalized Go and Git version strings, or driver source files beyond the six listed pins. The artifact expires on 2026-11-29; the accepted archive and seal remain in private task evidence custody for verification.

## C3: old and refreshed Windows observations

`old-new-windows.tsv` is copied byte-for-byte from the accepted C16 r1 attachment; source SHA-256 `72e13b1d3f05f5b354cc582e063738ca340b430ab8429d283df609ad1d2a20b0`. It records all 47 cases by old and new path, SHA-256, and byte-equality result. The accepted TSV does not contain size columns.

The comparison is 44 byte-identical cases, two C15 ancestor-span changes (`claude_code-empty.json` and `claude_code-mcp.json`), and one new capture case (`codex_cli-native-parent.json`); no old case is missing. The two changed cases are attributed to generated ancestor exclusions in `f572da6540f7b902c9f72249701e9fe978aa2b36`. The new 13,887-byte case (SHA-256 `201e0ada97bf801f4bcfce29d3208c3542ae74bc5ace7d2ebdecaa37b91a8924`) restores capture coverage through the capture-side driver change; it is not a product change. The historical old artifact remains the unsealed 46-file capture.

## Contract references

- Effective baseline manifest: `EFFECTIVE-REVISION-01`, SHA-256 `1e5f8d035bc6cf7917eefeec53e662b48b2d28b039cd9873b7f85fd2f00ffae1`.
- Accepted C15 r3: `BASELINE-REVISION-01-AMENDMENT-C15-r3.md`, SHA-256 `0ce1718345bb9f67ff48d71974c2f203a8a2fd5da0685c76ffe993583c875720`.
- Accepted C16 r1: `BASELINE-REVISION-01-AMENDMENT-C16-r1.md`, SHA-256 `d7002e764a59b4760a1e65ff5b955d10a7458bdc97b347f48b6d3d8c190e2da6`.

This provenance does not claim that the capture seal binds runner image identity, host patch level, Windows architecture, or any other field absent from the seal.
