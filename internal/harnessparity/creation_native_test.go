//go:build harness_parity && !linux && !darwin

package harnessparity

// Native capture policy is unavailable until separately admitted evidence and
// the explicit per-platform comparator arrive. No POSIX policy is inferred.
func selectCreationMask(string) {}
