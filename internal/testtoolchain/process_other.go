//go:build !darwin && !linux

package testtoolchain

func hostGOROOTProcessAlive(int) bool { return false }
