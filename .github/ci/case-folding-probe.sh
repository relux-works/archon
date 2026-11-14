#!/usr/bin/env bash
# Measure the temporary filesystem used by the profile case fixture. This
# reports capability only; it never infers case identity from the target GOOS.
set -eu
root="${TMPDIR:-${TMP:-${TEMP:-/tmp}}}"
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*)
    # testgitenv.ShortTempDir uses the ordinary Windows temporary volume root.
    ordinary="$(cygpath -w "${TMP:-${TEMP:-$root}}")"
    case "$ordinary" in
      [A-Za-z]:*) root="$(cygpath -u "${ordinary:0:2}/")" ;;
      *) echo 'case-folding-probe: unsupported temporary volume' >&2; exit 2 ;;
    esac
    ;;
esac
work="$(mktemp -d "${root%/}/case-folding.XXXXXX")" || exit 2
trap 'rm -rf "$work"' EXIT
printf 'case identity witness\n' >"$work/CaseProbe" || exit 2
if [ -e "$work/caseprobe" ]; then
  [ "$work/CaseProbe" -ef "$work/caseprobe" ] || exit 2
  echo folding
else
  echo sensitive
fi
