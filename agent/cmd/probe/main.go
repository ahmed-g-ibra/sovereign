// Command probe measures network latency from this machine to cloud regions.
//
// Phase 0 of Sovereign: no control plane, no database, no agent registration.
// It prints to stdout and that is deliberate — the point is to start
// accumulating real measurements from a Gulf connection today, because history
// is the one thing that cannot be backfilled later.
//
//	probe -verify              # which endpoints actually resolve?
//	probe -count 20            # 20 rounds, then a percentile summary
//	probe -count 0 -json       # run forever, JSON lines for piping to a file
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"sovereign/agent/internal/probe"
	"sovereign/agent/internal/targets"
)

func main() {
	var (
		targetsPath = flag.String("targets", "targets.json", "path to the targets file")
		filter      = flag.String("filter", "", "comma-separated terms matched against id/provider/region/country/status")
		count       = flag.Int("count", 1, "number of rounds; 0 runs until interrupted")
		interval    = flag.Duration("interval", 60*time.Second, "delay between rounds")
		timeout     = flag.Duration("timeout", 5*time.Second, "per-measurement timeout")
		asJSON      = flag.Bool("json", false, "emit one JSON object per measurement instead of a table")
		verify      = flag.Bool("verify", false, "single pass over every target including unverified ones, then report what is reachable")
		includeAll  = flag.Bool("all", false, "include targets not marked live")
	)
	flag.Parse()

	if err := run(*targetsPath, *filter, *count, *interval, *timeout, *asJSON, *verify, *includeAll); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(path, filter string, count int, interval, timeout time.Duration, asJSON, verify, includeAll bool) error {
	all, err := targets.Load(path)
	if err != nil {
		return err
	}
	ts := targets.Filter(all, filter)

	// Normal runs measure only confirmed endpoints. -verify and -all opt into
	// the unconfirmed ones, which is how the list gets curated in the first place.
	if !verify && !includeAll {
		live := ts[:0:0]
		for _, t := range ts {
			if t.Status == targets.StatusLive {
				live = append(live, t)
			}
		}
		ts = live
	}
	if len(ts) == 0 {
		return fmt.Errorf("no targets selected (loaded %d, filter %q)", len(all), filter)
	}

	if verify {
		count, interval = 1, 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	p := probe.New(timeout)
	enc := json.NewEncoder(os.Stdout)
	var results []probe.Result

	fmt.Fprintf(os.Stderr, "probing %d targets, %s rounds, %s timeout\n",
		len(ts), roundsLabel(count), timeout)

	for round := 1; count == 0 || round <= count; round++ {
		for _, t := range ts {
			// Sequential on purpose. Parallel probes from one home connection
			// compete for the same uplink and measure each other.
			r := p.Measure(ctx, t)
			results = append(results, r)

			switch {
			case asJSON:
				if err := enc.Encode(r); err != nil {
					return err
				}
			case verify:
				// printed as a table at the end
			default:
				printLine(r)
			}

			if ctx.Err() != nil {
				goto done
			}
		}
		if count == 0 || round < count {
			select {
			case <-ctx.Done():
				goto done
			case <-time.After(interval):
			}
		}
	}

done:
	if verify {
		printVerify(ts, results)
		return nil
	}
	if len(results) > len(ts) {
		printSummary(results)
	}
	return nil
}

func roundsLabel(count int) string {
	if count == 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d", count)
}

func printLine(r probe.Result) {
	if !r.OK() {
		fmt.Printf("%-28s %-22s  FAILED  %s\n", r.TargetID, r.Host, r.Error)
		return
	}
	fmt.Printf("%-28s %-22s  dns %6.1f  tcp %6.1f  tls %6.1f  total %7.1f ms\n",
		r.TargetID, r.Host, r.DNSMs, r.ConnectMs, r.TLSMs, r.TotalMs)
}

func printSummary(results []probe.Result) {
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TARGET\tPROVIDER\tN\tLOSS%\tMIN\tP50\tP90\tP95\tMAX")
	for _, s := range probe.Summarize(results) {
		fmt.Fprintf(w, "%s\t%s\t%d\t%.0f\t%.1f\t%.1f\t%.1f\t%.1f\t%.1f\n",
			s.TargetID, s.Provider, s.Samples, s.LossPct, s.Min, s.P50, s.P90, s.P95, s.Max)
	}
	w.Flush()
	fmt.Println("\nAll timings in milliseconds. Percentiles over successful probes only.")
}

func printVerify(ts []targets.Target, results []probe.Result) {
	byID := make(map[string]probe.Result, len(results))
	for _, r := range results {
		byID[r.TargetID] = r
	}
	sorted := append([]targets.Target(nil), ts...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Provider != sorted[j].Provider {
			return sorted[i].Provider < sorted[j].Provider
		}
		return sorted[i].ID < sorted[j].ID
	})

	var reachable, broken []string
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TARGET\tSTATUS\tRESULT\tDETAIL")
	for _, t := range sorted {
		r := byID[t.ID]
		if r.OK() {
			reachable = append(reachable, t.ID)
			fmt.Fprintf(w, "%s\t%s\tok\t%.1f ms via %s\n", t.ID, t.Status, r.TotalMs, r.RemoteIP)
			continue
		}
		broken = append(broken, t.ID)
		fmt.Fprintf(w, "%s\t%s\tFAIL\t%s\n", t.ID, t.Status, truncate(r.Error, 60))
	}
	w.Flush()

	fmt.Printf("\n%d reachable, %d unreachable.\n", len(reachable), len(broken))
	if len(broken) > 0 {
		fmt.Println("Unreachable endpoints are a finding, not a bug: prune them from targets.json,")
		fmt.Println("or wait for the Phase 2 in-region responders to replace the guesswork.")
		fmt.Println("  " + strings.Join(broken, "\n  "))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
