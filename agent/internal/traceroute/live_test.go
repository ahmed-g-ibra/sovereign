package traceroute

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live network test. Skipped unless SOVEREIGN_NET_TEST=1, so CI stays
// hermetic and a contributor running `go test ./...` on a locked-down network
// does not see a confusing failure.
//
//	SOVEREIGN_NET_TEST=1 go test ./internal/traceroute/ -run Live -v
func TestLiveTrace(t *testing.T) {
	if os.Getenv("SOVEREIGN_NET_TEST") != "1" {
		t.Skip("set SOVEREIGN_NET_TEST=1 to run against the real network")
	}
	host := os.Getenv("SOVEREIGN_NET_TARGET")
	if host == "" {
		host = "s3.me-central-1.amazonaws.com"
	}

	tr, err := New()
	if err != nil {
		t.Fatalf("could not open an ICMP socket: %v", err)
	}
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	trace, err := tr.Trace(ctx, "live", host,
		Options{MaxTTL: 20, HopTimeout: time.Second})
	if err != nil {
		t.Fatalf("trace failed: %v", err)
	}

	t.Logf("path to %s (%s), complete=%v", host, trace.DestIP, trace.Complete)
	answered := 0
	for _, h := range trace.Hops {
		if h.Timeout {
			t.Logf("  %2d  *", h.TTL)
			continue
		}
		answered++
		t.Logf("  %2d  %-16s %7.1f ms%s", h.TTL, h.IP, h.RTTMs,
			map[bool]string{true: "  <- destination"}[h.Final])
	}
	if answered == 0 {
		t.Error("no hop answered; ICMP is probably blocked on this network")
	}
}
