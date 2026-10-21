package probe

import (
	"math"
	"testing"
)

func TestPercentile(t *testing.T) {
	cases := []struct {
		name string
		in   []float64
		p    float64
		want float64
	}{
		{"empty", nil, 50, 0},
		{"single", []float64{42}, 95, 42},
		{"median odd", []float64{1, 2, 3}, 50, 2},
		{"median even", []float64{1, 2, 3, 4}, 50, 2},
		{"p95 of ten", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 95, 10},
		{"p90 of ten", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 90, 9},
		{"min clamp", []float64{5, 1, 9}, 0, 1},
		{"max clamp", []float64{5, 1, 9}, 100, 9},
		{"unsorted input", []float64{9, 1, 5, 3, 7}, 50, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Percentile(append([]float64(nil), c.in...), c.p)
			if math.Abs(got-c.want) > 1e-9 {
				t.Fatalf("Percentile(%v, %v) = %v, want %v", c.in, c.p, got, c.want)
			}
		})
	}
}

func TestSummarizeSeparatesFailuresFromTimings(t *testing.T) {
	results := []Result{
		{TargetID: "a", Provider: "aws", TotalMs: 10},
		{TargetID: "a", Provider: "aws", TotalMs: 30},
		{TargetID: "a", Provider: "aws", Error: "connect: timeout"},
		{TargetID: "b", Provider: "gcp", TotalMs: 5},
	}

	got := Summarize(results)
	if len(got) != 2 {
		t.Fatalf("got %d summaries, want 2", len(got))
	}

	// Sorted by p50, so "b" (5ms) comes first.
	if got[0].TargetID != "b" {
		t.Errorf("summaries not sorted by p50: got %q first", got[0].TargetID)
	}

	var a Summary
	for _, s := range got {
		if s.TargetID == "a" {
			a = s
		}
	}
	if a.Samples != 3 {
		t.Errorf("Samples = %d, want 3", a.Samples)
	}
	if a.Failures != 1 {
		t.Errorf("Failures = %d, want 1", a.Failures)
	}
	// The failed probe must not drag the timings: min stays 10, not 0.
	if a.Min != 10 {
		t.Errorf("Min = %v, want 10 (failed probe leaked into timings)", a.Min)
	}
	if a.Max != 30 {
		t.Errorf("Max = %v, want 30", a.Max)
	}
	if math.Abs(a.LossPct-33.333333) > 0.001 {
		t.Errorf("LossPct = %v, want ~33.33", a.LossPct)
	}
}

func TestSummarizeAllFailures(t *testing.T) {
	got := Summarize([]Result{
		{TargetID: "dead", Error: "dns: no such host"},
		{TargetID: "dead", Error: "dns: no such host"},
	})
	if len(got) != 1 {
		t.Fatalf("got %d summaries, want 1", len(got))
	}
	if got[0].LossPct != 100 {
		t.Errorf("LossPct = %v, want 100", got[0].LossPct)
	}
	if got[0].P50 != 0 {
		t.Errorf("P50 = %v, want 0 for an unreachable target", got[0].P50)
	}
}
