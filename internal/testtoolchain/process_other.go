//go:build !darwin && !linux && !windows

package testtoolchain

func hostGOROOTProcessAlive(int) bool { return false }
