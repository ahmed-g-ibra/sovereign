//go:build !linux && !darwin

package hostinfo

func load1() (float64, bool) { return 0, false }
