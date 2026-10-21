package probe

import "sort"

// Summary aggregates repeated measurements of one target.
//
// Percentiles, not averages: an average hides the tail, and the tail is the
// entire reason anyone reads a latency comparison.
type Summary struct {
	TargetID string  `json:"target_id"`
	Provider string  `json:"provider"`
	Region   string  `json:"region"`
	Samples  int     `json:"samples"`
	Failures int     `json:"failures"`
	Min      float64 `json:"min_ms"`
	P50      float64 `json:"p50_ms"`
	P90      float64 `json:"p90_ms"`
	P95      float64 `json:"p95_ms"`
	P99      float64 `json:"p99_ms"`
	Max      float64 `json:"max_ms"`
	LossPct  float64 `json:"loss_pct"`
}

// Percentile returns the p-th percentile (0..100) of xs using nearest-rank.
// xs is sorted in place. Returns 0 for an empty slice.
func Percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sort.Float64s(xs)
	if p <= 0 {
		return xs[0]
	}
	if p >= 100 {
		return xs[len(xs)-1]
	}
	rank := int(float64(len(xs))*p/100.0 + 0.5)
	if rank < 1 {
		rank = 1
	}
	if rank > len(xs) {
		rank = len(xs)
	}
	return xs[rank-1]
}

// Summarize groups results by target and computes percentiles over the
// successful ones. Failed probes count toward loss but never toward timings.
func Summarize(results []Result) []Summary {
	type bucket struct {
		s      Summary
		values []float64
	}
	order := []string{}
	buckets := map[string]*bucket{}

	for _, r := range results {
		b, ok := buckets[r.TargetID]
		if !ok {
			b = &bucket{s: Summary{TargetID: r.TargetID, Provider: r.Provider, Region: r.Region}}
			buckets[r.TargetID] = b
			order = append(order, r.TargetID)
		}
		b.s.Samples++
		if !r.OK() {
			b.s.Failures++
			continue
		}
		b.values = append(b.values, r.TotalMs)
	}

	out := make([]Summary, 0, len(order))
	for _, id := range order {
		b := buckets[id]
		if len(b.values) > 0 {
			sort.Float64s(b.values)
			b.s.Min = b.values[0]
			b.s.Max = b.values[len(b.values)-1]
			b.s.P50 = Percentile(b.values, 50)
			b.s.P90 = Percentile(b.values, 90)
			b.s.P95 = Percentile(b.values, 95)
			b.s.P99 = Percentile(b.values, 99)
		}
		if b.s.Samples > 0 {
			b.s.LossPct = float64(b.s.Failures) / float64(b.s.Samples) * 100
		}
		out = append(out, b.s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].P50 == out[j].P50 {
			return out[i].TargetID < out[j].TargetID
		}
		if out[i].P50 == 0 {
			return false
		}
		if out[j].P50 == 0 {
			return true
		}
		return out[i].P50 < out[j].P50
	})
	return out
}
