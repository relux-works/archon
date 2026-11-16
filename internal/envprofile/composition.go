package envprofile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/relux-works/curator/internal/contextlock"
	"github.com/relux-works/curator/internal/contextpkg"
	"github.com/relux-works/curator/internal/envauthority"
	"github.com/relux-works/curator/internal/hashing"
	"github.com/relux-works/curator/internal/identifiers"
	"github.com/relux-works/curator/internal/identity"
)

// VisitCompositionProfile retains default local verification for existing
// callers. Configured callers use VisitCompositionProfileWithPolicy.
func VisitCompositionProfile(home, name, expected string, visit func(*contextlock.Lock, contextlock.Member, string) error) error {
	_, err := VisitCompositionProfileWithPolicy(home, name, expected, Policy{}, visit)
	return err
}

// VisitCompositionProfileWithPolicy reads the explicit protected selection and
// every required observation before applying policy. It acquires no source,
// synthesizes no evidence and publishes no home. The caller holds the home lock.
// Source generation must remain stable even when initial mismatches are allowed.
func VisitCompositionProfileWithPolicy(home, name, expected string, policy Policy, visit func(*contextlock.Lock, contextlock.Member, string) error) ([]string, error) {
	if !identifiers.Valid(name) || expected == "" || visit == nil {
		return nil, fmt.Errorf("invalid composition profile selection")
	}
	var initialGeneration []string
	check := func(initial bool) (*contextlock.Lock, []string, error) {
		if err := envauthority.Validate(home); err != nil {
			return nil, nil, err
		}
		if err := validateLockBoundary(home, name, nil); err != nil {
			return nil, nil, err
		}
		lock, hash, err := readLock(home, name)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimPrefix(hash, "sha256:") != expected {
			return nil, nil, fmt.Errorf("selected profile lock changed")
		}
		if failure := validateNamedStoreBoundaries(home, lock, nil); failure != nil {
			return nil, nil, failure.Err
		}
		checks, err := startupChecks(home, lock)
		if err != nil {
			return nil, nil, err
		}
		verdict, err := startupVerdict(policy, lock, checks)
		if err != nil {
			return nil, nil, err
		}
		if !verdict.Allow {
			return nil, verdict.Warnings, fmt.Errorf("%s: composition verification refused", strings.Join(verdict.Refusals, "; "))
		}
		generation := make([]string, 0, len(lock.Members))
		for _, member := range lock.Members {
			digest, err := hashing.ContentSHA256WithVersion(memberEntryPath(home, member), map[string]bool{}, hashing.VersionV2)
			if err != nil {
				return nil, verdict.Warnings, err
			}
			generation = append(generation, digest)
		}
		if initial {
			initialGeneration = generation
		} else if !equalStrings(initialGeneration, generation) {
			return nil, verdict.Warnings, fmt.Errorf("composition profile source generation changed")
		}
		return lock, verdict.Warnings, nil
	}
	lock, warnings, err := check(true)
	if err != nil {
		return warnings, err
	}
	manager := newGitManager(home)
	for _, member := range lock.Members {
		entry := manager.entryPath(home, resolvedOf(member))
		cleanup := func() {}
		if member.Commit != "" {
			var err error
			entry, cleanup, _, err = extractPinnedSnapshot(home, member)
			if err != nil {
				return warnings, err
			}
		}
		root, err := selectedPackageRoot(entry, member.Directory)
		if err == nil {
			err = visit(lock, member, root)
		}
		cleanup()
		if err != nil {
			return warnings, err
		}
		if err := envauthority.Validate(home); err != nil {
			return warnings, err
		}
	}
	_, _, err = check(false)
	return warnings, err
}

// Composition agreement (environments §10.1, §12.1): the effective machine
// overlay declarations of a profile and the overlay members its lock records
// must agree before a fragment is emitted, a home is repaired, or a scope is
// switched onto that lock. A disagreement is composition_lock_mismatch; it is
// converged only by `profile compose` or `profile update`, never silently.

// DiagCompositionLockMismatch reports that the effective machine overlay
// declarations and the selected profile lock disagree (environments §10.1,
// §12.1). It names the profile and the missing or extra overlay members.
const DiagCompositionLockMismatch = "composition_lock_mismatch"

// overlayMembershipKey renders the membership identity of one overlay. A git
// overlay is keyed by its canonical source identity and directory, which is
// what the lock records for a git member; a path overlay is keyed by its
// package name, because a path member records no source. Requirement forms
// and weights are not part of membership: a moved pin is `profile update`'s
// resolved-version delta, not a composition disagreement.
func overlayMembershipKey(kind, source, directory, name string) string {
	switch kind {
	case "git":
		if directory != "" {
			return "git " + source + " (directory " + directory + ")"
		}
		return "git " + source
	default:
		return "path " + name
	}
}

// declaredOverlayKeys maps the effective declarations onto membership keys.
// A path declaration reads only its manifest name; an unreadable path package
// keeps the established path-source diagnostic rather than a mismatch.
func declaredOverlayKeys(decls []OverlaySpec) ([]string, error) {
	keys := make([]string, 0, len(decls))
	for _, decl := range decls {
		switch identity.ClassifySource(decl.Source) {
		case identity.SourcePath:
			manifest, err := contextpkg.LoadManifest(decl.Source)
			if err != nil {
				return nil, pathManifestDiag(decl.Source, err)
			}
			keys = append(keys, overlayMembershipKey("path", "", "", manifest.Name))
		case identity.SourceGit:
			canonical, err := canonicalGit(decl.Source)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", DiagSourceInvalid, err)
			}
			keys = append(keys, overlayMembershipKey("git", canonical, decl.Directory, ""))
		default:
			return nil, sourceKindRefusal("overlay source", decl.Source)
		}
	}
	return keys, nil
}

// lockedOverlayKeys maps the lock's overlay members onto membership keys.
func lockedOverlayKeys(lock *contextlock.Lock) []string {
	var keys []string
	for _, member := range lock.Members {
		if !member.Overlay {
			continue
		}
		if member.Source != "" {
			keys = append(keys, overlayMembershipKey("git", member.Source, member.Directory, member.Name))
			continue
		}
		keys = append(keys, overlayMembershipKey("path", "", "", member.Name))
	}
	return keys
}

func keyDifference(from, against []string) []string {
	present := map[string]bool{}
	for _, key := range against {
		present[key] = true
	}
	seen := map[string]bool{}
	var missing []string
	for _, key := range from {
		if !present[key] && !seen[key] {
			missing = append(missing, key)
			seen[key] = true
		}
	}
	sort.Strings(missing)
	return missing
}

// checkCompositionLock compares the effective overlay declarations of profile
// (environments §12.2: a forbidding overlays_allowed empties them, so a
// root-only effective composition matches a lock without overlay members)
// with the overlay members recorded in lock. A disagreement refuses with
// composition_lock_mismatch naming the profile and the members missing from
// or extra in the lock; the caller must perform no write, repair, or switch.
func checkCompositionLock(profile string, lock *contextlock.Lock, policy Policy) error {
	declared, err := declaredOverlayKeys(policy.EffectiveOverlays(profile))
	if err != nil {
		return err
	}
	locked := lockedOverlayKeys(lock)
	missing := keyDifference(declared, locked)
	extra := keyDifference(locked, declared)
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "declared but missing from the lock: "+strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		parts = append(parts, "in the lock but not declared: "+strings.Join(extra, ", "))
	}
	return fmt.Errorf("%s: profile %q effective overlay declarations and its lock disagree (%s); rerun 'curator profile compose %s add|remove ...' or 'curator profile update %s' to converge",
		DiagCompositionLockMismatch, profile, strings.Join(parts, "; "), profile, profile)
}
