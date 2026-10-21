// Package hostinfo reports the health of the machine doing the measuring.
//
// Methodology rule 5: a loaded probe host reports latency that belongs to its
// own CPU scheduler, not to the network. Recording load with every sample is
// what makes that data discardable after the fact instead of silently believed.
package hostinfo

// Load1 returns the one-minute load average, and whether it could be read.
// Platforms without an implementation report false rather than a plausible
// zero, because a fabricated 0.0 would be worse than a missing value.
func Load1() (float64, bool) { return load1() }
