package envprofile

// profile compose (cli/curator.md, environments §12.1): a declaration change
// for an installed profile rebuilds that profile's lock from the complete
// resulting composition under the same held profile operation, then publishes
// the declaration. Before a profile has a lock, only the declaration changes.

import (
	"fmt"
	"io"
)

// ComposeOptions selects one declaration-plus-lock composition change.
type ComposeOptions struct {
	// Policy carries the machine gates with the COMPLETE resulting overlay
	// declarations for the profile already applied: the lock is rebuilt from
	// policy.EffectiveOverlays(name), nothing is read back from the file.
	Policy Policy
	// PublishDeclaration writes the machine configuration carrying the
	// resulting declaration. It runs under the held profile operation after
	// the lock publication, so the only crash window leaves a lock that the
	// declaration does not yet match — a state env resolve refuses with
	// composition_lock_mismatch and a rerun converges. A nil func writes no
	// declaration.
	PublishDeclaration func() error
	// WarningSink and SurfacingSink receive the §9.2 update warnings and the
	// §2.3 surfacing rows at their emission points, exactly as profile update.
	WarningSink   io.Writer
	SurfacingSink io.Writer
}

// ComposeResult reports what one compose changed.
type ComposeResult struct {
	// Info is the rebuilt profile when LockRebuilt is true.
	Info Info
	// LockRebuilt reports that the profile had a lock, which was re-resolved
	// from the complete resulting composition. False means the profile has no
	// lock yet and only the declaration changed.
	LockRebuilt bool
	// LockMoved reports that the rebuilt lock differs from the previous one.
	LockMoved bool
}

// Compose applies one overlay declaration change to profile name
// (environments §12.1). For an installed profile with a lock it re-resolves
// and audits the complete resulting composition exactly as profile update
// does — a blocking finding leaves the old lock in place and writes no
// declaration — publishes the new lock through the operation journal, then
// writes the declaration. Nothing changes unless the whole result validates,
// resolves, and audits. A profile without a lock gets only the declaration;
// its first install resolves the overlays jointly.
//
// The hardened install/update refusals (manager §7.1: source_allowlist_empty
// is "the install/update error"; passable_env_names_unbounded_refused) gate
// the lock rebuild alone, because only the rebuild resolves sources. The
// declaration edit of a profile without a lock resolves nothing — it is the
// informative `profile compose` row of environments §12.1 — so it is not
// refused; its first install meets the same gate in Install before resolving
// the declared overlays. Whether the profile has a lock is known only under
// the held operation, so the gate runs after beginOperation and before
// updateLocked. That ordering is safe: beginOperation takes the mutation lock
// and replays a prior interrupted journal, as every profile operation does,
// and writes nothing of this operation; every write of this one —
// ensureDefault, lock publication, declaration — follows the gate, so a
// refusal writes neither the lock nor the declaration.
func Compose(home, name string, options ComposeOptions) (ComposeResult, error) {
	var result ComposeResult
	if err := refusePreparedSkillPlanAdoption(home, name); err != nil {
		return result, err
	}
	op, err := beginOperation(home)
	if err != nil {
		return result, err
	}
	defer func() { _ = op.close() }()
	installed := false
	var source Source
	if validProfileName(name) {
		source, err = readSource(home, name)
		switch {
		case err == nil:
			installed = true
		case !isStateAbsent(err):
			return result, err
		}
	}
	hasLock := false
	if installed {
		if _, _, err := readLock(home, name); err == nil {
			hasLock = true
		} else if !isStateAbsent(err) {
			return result, err
		}
	}
	if hasLock {
		// The rebuild is a profile update: refuse before any source is
		// resolved or anything is written.
		if err := options.Policy.checkHardenedProfileMutation(); err != nil {
			return result, err
		}
		if source.Kind == KindLocal {
			// The builtin profile does not move (§9.4); composing it would
			// leave a declaration no lock can ever match.
			return result, fmt.Errorf("%s: profile %q is the builtin local profile and takes no overlays", DiagUpdateBlocked, name)
		}
		info, moved, err := updateLocked(op, home, name, options.Policy, options.SurfacingSink, options.WarningSink, false, nil, nil, nil, true)
		if err != nil {
			return ComposeResult{Info: info}, err
		}
		result = ComposeResult{Info: info, LockRebuilt: true, LockMoved: moved}
	}
	if options.PublishDeclaration != nil {
		if err := options.PublishDeclaration(); err != nil {
			if result.LockRebuilt {
				// The lock already matches the requested composition; the
				// declaration on disk does not. Say so with the diagnostic
				// the next env resolve will raise.
				return result, fmt.Errorf("%s: profile %q lock %s is published but its declaration was not written (%v); rerun the same profile compose to converge", DiagCompositionLockMismatch, name, result.Info.LockHash, err)
			}
			return result, err
		}
	}
	return result, nil
}
