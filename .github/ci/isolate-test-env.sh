#!/usr/bin/env bash
# Shell equivalent of internal/testgitenv.Isolate, for standalone fixtures.
# Source this file and call isolate_test_env with a fresh owned directory before
# running any fixture command. The caller owns cleanup and the process lifetime;
# this function never touches or restores caller repositories or native homes.

isolate_test_env() {
	local root key upper windows entry complete expected seen
	local -a keys=()
	root="$1"
	# Refuse reuse and canonicalize before creating config or exporting paths.
	(umask 077; mkdir "$root") || return 2
	root="$(cd "$root" && pwd -P)" || return 2
	windows="$(uname -s)" || return 2
	case "$windows" in
		MINGW*|MSYS*|CYGWIN*) windows=1 ;;
		*) windows=0 ;;
	esac
	# Use the same OS environment API as isolated-test.cjs: Bash's variable
	# table omits inherited nonidentifier names. Frame the complete snapshot
	# before writing it, so a NUL-boundary truncation followed by a completion
	# marker cannot turn a partial inventory into success. Node is already a
	# prerequisite of the CI test wrapper. Values stay in a pipe, never a file.
	complete=0
	expected=''
	seen=0
	while IFS= read -r -d '' entry; do
		if [ "$complete" -eq 1 ]; then complete=0; break; fi
		if [ -z "$expected" ]; then
			if [[ "$entry" =~ ^isolate-test-env-count:([1-9][0-9]*)$ ]]; then
				expected="${BASH_REMATCH[1]}"
				continue
			fi
			break
		fi
		if [ "$entry" = 'isolate-test-env-complete' ]; then
			if [ "$seen" != "$expected" ]; then break; fi
			complete=1
			continue
		fi
		case "$entry" in *=*) key="${entry%%=*}" ;; *) break ;; esac
		seen=$((seen + 1))
		keys+=("$key")
	done < <(node -e '
try {
  const entries = Object.entries(process.env);
  if (entries.length === 0) throw new Error("empty environment");
  const records = entries.map(([key, value]) => key + "=" + value + "\0");
  process.stdout.write("isolate-test-env-count:" + entries.length + "\0" +
    records.join("") + "isolate-test-env-complete\0");
} catch {
  process.exitCode = 2;
}
')
	# Reading through EOF also rejects extra complete or unterminated records
	# after the trailer. Do not mutate the caller environment before validation.
	if [ "$complete" -ne 1 ] || [ -n "$entry" ]; then
		printf '%s\n' 'isolate-test-env: complete environment enumeration unavailable; refusing before fixture commands' >&2
		return 2
	fi
	for key in "${keys[@]}"; do
		upper="$(printf '%s' "$key" | LC_ALL=C tr '[:lower:]' '[:upper:]')" || return 2
		case "$upper" in
			GIT_*)
				case "$key" in
					*[!a-zA-Z0-9_]*)
						printf '%s\n' 'isolate-test-env: unsupported inherited Git name; refusing before fixture commands' >&2
						return 2 ;;
				esac ;;
		esac
		case "$upper" in
			GIT_*|SSH_AUTH_SOCK|SSH_ASKPASS|HOME|XDG_CONFIG_HOME|CODEX_HOME|CLAUDE_CONFIG_DIR|PI_CODING_AGENT_DIR|\
			XDG_DATA_HOME|XDG_STATE_HOME|XDG_CACHE_HOME)
				unset "$key" || return 2 ;;
			USERPROFILE)
				if [ "$windows" -eq 1 ]; then unset "$key" || return 2; fi ;;
		esac
	done
	(umask 077; mkdir "$root/home" "$root/xdg" "$root/tmp" && : >"$root/gitconfig") || return 2
	HOME="$root/home"
	XDG_CONFIG_HOME="$root/xdg"
	# Git allowlist: use only the fixture-owned empty global config.
	GIT_CONFIG_GLOBAL="$root/gitconfig"
	# Do not read host system Git configuration or its helpers.
	GIT_CONFIG_NOSYSTEM=1
	TMPDIR="$root/tmp"
	TMP="$TMPDIR"
	TEMP="$TMPDIR"
	export HOME XDG_CONFIG_HOME GIT_CONFIG_GLOBAL GIT_CONFIG_NOSYSTEM
	export TMPDIR TMP TEMP
	if [ "$windows" -eq 1 ]; then
		USERPROFILE="$HOME"
		export USERPROFILE
	fi
}
