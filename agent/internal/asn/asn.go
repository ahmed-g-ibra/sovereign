// Package asn maps IP addresses to the network that announces them.
//
// A path of bare IPs is nearly unreadable. A path of AS numbers and names
// shows you that traffic from Oman to Bahrain crosses a European carrier, and
// that is the finding worth publishing.
//
// Lookups go to Team Cymru's public DNS service, which needs no API key, no
// account and no dependency beyond the standard library's resolver. Results
// are cached aggressively because IP-to-AS mappings change on the order of
// months and we query the same handful of routers every hour.
package asn

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	originZone = "origin.asn.cymru.com"
	asZone     = "asn.cymru.com"
	// Mappings are stable for months; a week is conservative.
	cacheTTL = 7 * 24 * time.Hour
)

// Record is what we know about the network announcing an address.
type Record struct {
	ASN      int       `json:"asn"`
	Name     string    `json:"name,omitempty"`
	Country  string    `json:"cc,omitempty"`
	Prefix   string    `json:"prefix,omitempty"`
	LookedUp time.Time `json:"looked_up"`
}

func (r Record) expired() bool { return time.Since(r.LookedUp) > cacheTTL }

// Lookup resolves addresses to networks, with a cache that survives restarts.
type Lookup struct {
	// TXT is the resolver, swappable in tests.
	TXT func(ctx context.Context, name string) ([]string, error)

	mu    sync.Mutex
	cache map[string]Record
	path  string
	dirty bool
}

func New(cachePath string) *Lookup {
	l := &Lookup{
		TXT: func(ctx context.Context, name string) ([]string, error) {
			return net.DefaultResolver.LookupTXT(ctx, name)
		},
		cache: map[string]Record{},
		path:  cachePath,
	}
	l.load()
	return l
}

// Private reports whether an address has no public AS and should not be
// looked up. Querying Cymru for 192.168.1.1 wastes a round trip and tells a
// third party about the shape of someone's home network.
func Private(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// Carrier-grade NAT, 100.64.0.0/10. Common on mobile and some fixed-line
	// ISPs, and not announced in BGP.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

// Of returns the network announcing addr, or ok=false if it is private or
// unresolvable.
func (l *Lookup) Of(ctx context.Context, addr string) (Record, bool) {
	ip := net.ParseIP(addr)
	if Private(ip) {
		return Record{}, false
	}

	l.mu.Lock()
	if rec, ok := l.cache[addr]; ok && !rec.expired() {
		l.mu.Unlock()
		return rec, rec.ASN != 0
	}
	l.mu.Unlock()

	rec, err := l.resolve(ctx, ip)
	if err != nil {
		// Cache the miss too, with a zero ASN, so one unroutable hop does not
		// mean a DNS query on every single trace.
		rec = Record{LookedUp: time.Now()}
	}

	l.mu.Lock()
	l.cache[addr] = rec
	l.dirty = true
	l.mu.Unlock()

	return rec, rec.ASN != 0
}

func (l *Lookup) resolve(ctx context.Context, ip net.IP) (Record, error) {
	v4 := ip.To4()
	if v4 == nil {
		return Record{}, fmt.Errorf("asn: only IPv4 is supported")
	}
	name := fmt.Sprintf("%d.%d.%d.%d.%s", v4[3], v4[2], v4[1], v4[0], originZone)

	txt, err := l.TXT(ctx, name)
	if err != nil || len(txt) == 0 {
		return Record{}, fmt.Errorf("asn: origin lookup failed: %w", err)
	}
	rec, err := parseOrigin(txt[0])
	if err != nil {
		return Record{}, err
	}
	rec.LookedUp = time.Now()

	// Second query for the human-readable name. A failure here is cosmetic.
	if txt, err := l.TXT(ctx, fmt.Sprintf("AS%d.%s", rec.ASN, asZone)); err == nil &&
		len(txt) > 0 {
		rec.Name = parseASName(txt[0])
	}
	return rec, nil
}

// parseOrigin reads a Cymru origin record:
//
//	"15169 | 8.8.8.0/24 | US | arin | 2023-12-28"
//
// Multi-origin prefixes return several ASNs in the first field, space
// separated. We take the first and accept the ambiguity rather than inventing
// a tie-break.
func parseOrigin(txt string) (Record, error) {
	fields := splitTrim(txt)
	if len(fields) < 3 {
		return Record{}, fmt.Errorf("asn: unexpected origin record %q", txt)
	}
	asns := strings.Fields(fields[0])
	if len(asns) == 0 {
		return Record{}, fmt.Errorf("asn: no AS number in %q", txt)
	}
	n, err := strconv.Atoi(asns[0])
	if err != nil {
		return Record{}, fmt.Errorf("asn: bad AS number in %q", txt)
	}
	return Record{ASN: n, Prefix: fields[1], Country: fields[2]}, nil
}

// parseASName reads a Cymru AS record:
//
//	"15169 | US | arin | 2000-03-30 | GOOGLE, US"
func parseASName(txt string) string {
	fields := splitTrim(txt)
	if len(fields) < 5 {
		return ""
	}
	return fields[4]
}

func splitTrim(s string) []string {
	parts := strings.Split(s, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// Save writes the cache to disk. Losing it costs only DNS queries, so failure
// is logged by the caller and never fatal.
func (l *Lookup) Save() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.dirty || l.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(l.cache, "", "  ")
	if err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, l.path); err != nil {
		return err
	}
	l.dirty = false
	return nil
}

func (l *Lookup) load() {
	if l.path == "" {
		return
	}
	b, err := os.ReadFile(l.path)
	if err != nil {
		return
	}
	var cache map[string]Record
	if err := json.Unmarshal(b, &cache); err == nil {
		l.cache = cache
	}
}
