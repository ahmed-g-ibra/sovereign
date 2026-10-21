package asn

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live DNS test against Team Cymru. Skipped unless SOVEREIGN_NET_TEST=1.
//
//	SOVEREIGN_NET_TEST=1 go test ./internal/asn/ -run Live -v
func TestLiveLookup(t *testing.T) {
	if os.Getenv("SOVEREIGN_NET_TEST") != "1" {
		t.Skip("set SOVEREIGN_NET_TEST=1 to query Team Cymru")
	}
	l := New("")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Real hops from a Muscat vantage point, plus a known control.
	for _, ip := range []string{"8.8.8.8", "81.161.90.253", "5.102.177.197",
		"3.5.50.14"} {
		rec, ok := l.Of(ctx, ip)
		if !ok {
			t.Errorf("%-16s no AS found", ip)
			continue
		}
		t.Logf("%-16s AS%-7d %-8s %-20s %s", ip, rec.ASN, rec.Country,
			rec.Prefix, rec.Name)
	}
}
