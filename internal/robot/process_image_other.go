//go:build !darwin && !linux && !windows

package robot

func processImagePath(int) string { return "" }
