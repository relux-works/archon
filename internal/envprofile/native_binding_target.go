package envprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/relux-works/curator/internal/pathboundary"
	"github.com/relux-works/curator/internal/stateread"
)

// validateNativeBindingTarget inspects only the explicitly bound directory's
// location and metadata. It never reads credential or session payloads, and
// never treats a managed directory as a native source, including via aliases.
func validateNativeBindingTarget(manager, native string, owner pathboundary.OwnerLookup) error {
	if manager == "" {
		return fmt.Errorf("%s: explicit manager location is required", DiagNativeHomeBindingRequired)
	}
	manager, err := filepath.Abs(manager)
	if err != nil {
		return err
	}
	nativeLocation, err := repairDirectoryLocation(native)
	if err != nil {
		return fmt.Errorf("%s: cannot establish bound native location: %w", DiagNativeHomeBindingRequired, err)
	}
	managedLocation, err := repairDirectoryLocation(manager)
	if err != nil {
		return fmt.Errorf("%s: cannot establish manager location: %w", DiagNativeHomeBindingRequired, err)
	}
	if bindingPathsOverlap(managedLocation, nativeLocation) {
		return fmt.Errorf("%s: native target overlaps manager-owned state", DiagNativeHomeBindingRequired)
	}
	managerState, err := stateread.Stat(manager)
	if err != nil {
		return err
	}
	// Inspect only metadata on the exact route, never credential payloads.
	for current := native; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("%s: native directory or ancestor is unavailable: %w", DiagNativeHomeBindingRequired, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s: native directory and ancestors must be direct directories", DiagNativeHomeBindingRequired)
		}
		if managerState.Kind == stateread.KindPresent && managerState.Info != nil && os.SameFile(info, managerState.Info) {
			return fmt.Errorf("%s: native target aliases manager-owned state", DiagNativeHomeBindingRequired)
		}
		if filepath.Dir(current) == current {
			break
		}
	}

	if err := pathboundary.ValidateRootWithOwner(native, boundaryOwner(owner)); err != nil {
		return err
	}
	return nil
}

func bindingPathsOverlap(a, b string) bool {
	within := func(root, child string) bool {
		rel, err := filepath.Rel(root, child)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
	}
	return within(a, b) || within(b, a)
}
