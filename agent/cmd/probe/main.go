// Command probe measures network latency from this machine to cloud regions.
//
// Three modes:
//
//	probe -verify                       # which endpoints resolve, from targets.json
//	probe -count 20                     # measure locally, print a summary
//	probe -register -server URL -token T # enrol with a control plane
//	probe -server URL                   # run as a fleet member, uploading samples
//
// The local modes need no server and are how a new contributor checks the tool
// before trusting it with anything.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"crypto/ed25519"

	"sovereign/agent/internal/client"
	"sovereign/agent/internal/hostinfo"
	"sovereign/agent/internal/identity"
	"sovereign/agent/internal/probe"
	"sovereign/agent/internal/spool"
	"sovereign/agent/internal/targets"
)

type options struct {
	targetsPath string
	filter      string
	count       int
	interval    time.Duration
	timeout     time.Duration
	asJSON      bool
	verify      bool
	includeAll  bool

	server   string
	stateDir string
	register bool
	token    string
	name     string
	country  string
	city     string
	isp      string
	tier     string
	spoolMB  int64
}

func main() {
	var o options
	flag.StringVar(&o.targetsPath, "targets", "targets.json", "path to the targets file (local modes only)")
	flag.StringVar(&o.filter, "filter", "", "comma-separated terms matched against id/provider/region/country/status")
	flag.IntVar(&o.count, "count", 1, "number of rounds; 0 runs until interrupted")
	flag.DurationVar(&o.interval, "interval", 60*time.Second, "delay between rounds")
	flag.DurationVar(&o.timeout, "timeout", 5*time.Second, "per-measurement timeout")
	flag.BoolVar(&o.asJSON, "json", false, "emit one JSON object per measurement instead of a table")
	flag.BoolVar(&o.verify, "verify", false, "single pass over every target including unverified ones")
	flag.BoolVar(&o.includeAll, "all", false, "include targets not marked live")

	flag.StringVar(&o.server, "server", "", "control plane base URL; enables fleet mode")
	flag.StringVar(&o.stateDir, "state", "state", "directory for identity and spool")
	flag.BoolVar(&o.register, "register", false, "enrol this probe, then exit")
	flag.StringVar(&o.token, "token", "", "enrolment token (with -register)")
	flag.StringVar(&o.name, "name", "", "probe name (with -register)")
	flag.StringVar(&o.country, "country", "", "ISO 3166-1 alpha-2 country (with -register)")
	flag.StringVar(&o.city, "city", "", "city (with -register)")
	flag.StringVar(&o.isp, "isp", "", "ISP you believe you are on (with -register)")
	flag.StringVar(&o.tier, "tier", "community", "core, vps, cloud or community (with -register)")
	flag.Int64Var(&o.spoolMB, "spool-mb", 64, "maximum megabytes of unsent samples to keep")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch {
	case o.register:
		err = runRegister(ctx, o)
	case o.server != "":
		err = runDaemon(ctx, o)
	default:
		err = runLocal(ctx, o)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// Enrolment
// ---------------------------------------------------------------------------

func runRegister(ctx context.Context, o options) error {
	switch {
	case o.server == "":
		return errors.New("-register needs -server")
	case o.token == "":
		return errors.New("-register needs -token")
	case o.name == "":
		return errors.New("-register needs -name")
	case len(o.country) != 2:
		return errors.New("-register needs -country as a two-letter code, e.g. OM")
	}

	if identity.Exists(o.stateDir) {
		// Re-enrolling would strand the existing probe's history under an
		// identity nothing can sign for any more.
		return fmt.Errorf("%s already holds an identity; delete it to enrol again", o.stateDir)
	}

	id, publicKey, err := identity.Generate(o.server)
	if err != nil {
		return err
	}

	probeID, err := client.Register(ctx, o.server, client.RegisterInfo{
		EnrollmentToken: o.token,
		PublicKey:       publicKey,
		Name:            o.name,
		Country:         strings.ToUpper(o.country),
		City:            o.city,
		ISPClaimed:      o.isp,
		Tier:            o.tier,
	})
	if err != nil {
		return err
	}

	id.ProbeID = probeID
	if err := id.Save(o.stateDir); err != nil {
		return err
	}

	fmt.Printf("enrolled as %s\n", probeID)
	fmt.Printf("identity saved to %s\n", filepath.Join(o.stateDir, "identity.json"))
	fmt.Printf("start measuring with: probe -server %s -state %s\n", o.server, o.stateDir)
	return nil
}

// ---------------------------------------------------------------------------
// Fleet mode
// ---------------------------------------------------------------------------

func runDaemon(ctx context.Context, o options) error {
	id, err := identity.Load(o.stateDir)
	if err != nil {
		return fmt.Errorf("no identity in %s: run -register first (%w)", o.stateDir, err)
	}
	key, err := signerOrFail(id)
	if err != nil {
		return err
	}

	c := client.New(o.server, id.ProbeID, key)
	sp, err := spool.New(filepath.Join(o.stateDir, "spool"), o.spoolMB<<20)
	if err != nil {
		return err
	}

	log.Printf("probe %s reporting to %s", id.ProbeID, o.server)
	if pending := sp.Pending(); pending > 0 {
		log.Printf("spool holds %d bytes from a previous run", pending)
	}

	var (
		cfg      *client.Config
		cfgAt    time.Time
		interval = o.interval
	)

	for round := 1; ; round++ {
		if cfg == nil || time.Since(cfgAt) > time.Duration(cfg.ConfigTTLS)*time.Second {
			fresh, err := c.Config(ctx)
			switch {
			case err == nil:
				cfg, cfgAt = fresh, time.Now()
				if err := saveCachedConfig(o.stateDir, cfg); err != nil {
					log.Printf("could not cache config: %v", err)
				}
				if cfg.IntervalS > 0 {
					interval = time.Duration(cfg.IntervalS) * time.Second
				}
				log.Printf("config: %d targets, every %s", len(cfg.Targets), interval)
			case cfg != nil:
				// Keep measuring on the last known config. A control plane
				// outage must not stop the fleet; that is precisely when the
				// data matters.
				log.Printf("config refresh failed, continuing on cached config: %v", err)
			default:
				// Nothing in memory: this is a cold start during an outage.
				// Without the on-disk copy the probe would sit idle through
				// exactly the event it exists to record.
				stale, lerr := loadCachedConfig(o.stateDir)
				if lerr != nil {
					return fmt.Errorf("initial config fetch: %w", err)
				}
				cfg, cfgAt = stale, time.Now()
				log.Printf("control plane unreachable (%v); measuring on the "+
					"config cached on disk: %d targets", err, len(cfg.Targets))
			}
		}

		results := measureAll(ctx, cfg.Targets, time.Duration(cfg.TimeoutS)*time.Second)
		if err := sp.Append(results); err != nil {
			log.Printf("spool append failed: %v", err)
		}
		flush(ctx, c, sp)

		if ctx.Err() != nil {
			log.Printf("shutting down; %d bytes left in spool", sp.Pending())
			return nil
		}
		if o.count > 0 && round >= o.count {
			return nil
		}

		select {
		case <-ctx.Done():
			log.Printf("shutting down; %d bytes left in spool", sp.Pending())
			return nil
		case <-time.After(interval):
		}
	}
}

const configCacheName = "config.json"

// saveCachedConfig keeps the last known instructions on disk so a probe that
// restarts while the control plane is down still knows what to measure.
func saveCachedConfig(stateDir string, cfg *client.Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(stateDir, configCacheName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadCachedConfig(stateDir string) (*client.Config, error) {
	b, err := os.ReadFile(filepath.Join(stateDir, configCacheName))
	if err != nil {
		return nil, err
	}
	var cfg client.Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Targets) == 0 {
		return nil, errors.New("cached config has no targets")
	}
	return &cfg, nil
}

func signerOrFail(id *identity.Identity) (ed25519.PrivateKey, error) {
	key, err := id.Signer()
	if err != nil {
		return nil, fmt.Errorf("identity is unusable: %w", err)
	}
	return key, nil
}

func measureAll(ctx context.Context, ts []targets.Target, timeout time.Duration) []probe.Result {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	p := probe.New(timeout)
	load, haveLoad := hostinfo.Load1()

	out := make([]probe.Result, 0, len(ts))
	for _, t := range ts {
		if ctx.Err() != nil {
			break
		}
		r := p.Measure(ctx, t)
		if haveLoad {
			r.HostLoad1 = load
		}
		out = append(out, r)
	}
	return out
}

// flush uploads everything pending. Failure is normal and cheap: the data
// stays on disk and goes out with the next round.
func flush(ctx context.Context, c *client.Client, sp *spool.Spool) {
	pending, commit, err := sp.Claim()
	if err != nil {
		log.Printf("spool read failed: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	resp, err := c.Upload(ctx, pending)
	if err != nil {
		log.Printf("upload of %d samples failed, keeping them: %v", len(pending), err)
		return
	}
	if err := commit(); err != nil {
		// The server has the data; failing to clear the spool only means we
		// will send it again, which is free because ingest is idempotent.
		log.Printf("uploaded but could not clear spool: %v", err)
	}

	log.Printf("uploaded %d samples (%d new, %d already held)",
		len(pending), resp.Accepted, resp.Duplicates)
	if resp.Quarantined {
		log.Printf("WARNING: this probe is quarantined and excluded from published "+
			"figures (clock skew %d ms). Fix the clock, then ask an operator to clear it.",
			resp.ClockSkewMs)
	}
}

// ---------------------------------------------------------------------------
// Local modes
// ---------------------------------------------------------------------------

func runLocal(ctx context.Context, o options) error {
	all, err := targets.Load(o.targetsPath)
	if err != nil {
		return err
	}
	ts := targets.Filter(all, o.filter)

	if !o.verify && !o.includeAll {
		live := ts[:0:0]
		for _, t := range ts {
			if t.Status == targets.StatusLive {
				live = append(live, t)
			}
		}
		ts = live
	}
	if len(ts) == 0 {
		return fmt.Errorf("no targets selected (loaded %d, filter %q)", len(all), o.filter)
	}

	count, interval := o.count, o.interval
	if o.verify {
		count, interval = 1, 0
	}

	p := probe.New(o.timeout)
	enc := json.NewEncoder(os.Stdout)
	var results []probe.Result

	fmt.Fprintf(os.Stderr, "probing %d targets, %s rounds, %s timeout\n",
		len(ts), roundsLabel(count), o.timeout)

	for round := 1; count == 0 || round <= count; round++ {
		for _, t := range ts {
			// Sequential on purpose. Parallel probes from one home connection
			// compete for the same uplink and measure each other.
			r := p.Measure(ctx, t)
			results = append(results, r)

			switch {
			case o.asJSON:
				if err := enc.Encode(r); err != nil {
					return err
				}
			case o.verify:
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
	if o.verify {
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
