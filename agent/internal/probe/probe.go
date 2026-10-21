// Package probe performs one network measurement against one endpoint.
//
// Timings are split into DNS, TCP connect and TLS handshake so that a slow
// result can be attributed. A single blended number tells you something is
// wrong but never what.
package probe

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"sovereign/agent/internal/targets"
)

// Result is a single measurement. Zero values for timings mean "not reached":
// if the TCP connect failed, TLSMs stays 0 rather than being invented.
type Result struct {
	Timestamp time.Time `json:"ts"`
	TargetID  string    `json:"target_id"`
	Provider  string    `json:"provider"`
	Region    string    `json:"region"`
	Host      string    `json:"host"`
	RemoteIP  string    `json:"remote_ip,omitempty"`
	DNSMs     float64   `json:"dns_ms"`
	ConnectMs float64   `json:"connect_ms"`
	TLSMs     float64   `json:"tls_ms"`
	TotalMs   float64   `json:"total_ms"`
	Error     string    `json:"error,omitempty"`
}

// OK reports whether the measurement completed end to end.
func (r Result) OK() bool { return r.Error == "" }

// Prober performs measurements. The zero value is not usable; use New.
type Prober struct {
	Timeout  time.Duration
	Resolver *net.Resolver
	// PreferIPv4 keeps samples comparable over time. A host that silently
	// switches between v4 and v6 produces a step change in latency that looks
	// like a network event and is not one.
	PreferIPv4 bool
}

// New returns a Prober with sensible defaults.
func New(timeout time.Duration) *Prober {
	return &Prober{
		Timeout:    timeout,
		Resolver:   net.DefaultResolver,
		PreferIPv4: true,
	}
}

// Measure times one probe of one target.
func (p *Prober) Measure(ctx context.Context, t targets.Target) Result {
	r := Result{
		Timestamp: time.Now().UTC(),
		TargetID:  t.ID,
		Provider:  t.Provider,
		Region:    t.Region,
		Host:      t.Host,
	}

	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	// DNS. Measured separately because resolver latency is a property of the
	// vantage point's ISP, not of the cloud region, and conflating the two is
	// the most common way these measurements go wrong.
	start := time.Now()
	addrs, err := p.Resolver.LookupIPAddr(ctx, t.Host)
	r.DNSMs = msSince(start)
	if err != nil {
		r.Error = "dns: " + err.Error()
		return r
	}
	ip := pickIP(addrs, p.PreferIPv4)
	if ip == "" {
		r.Error = "dns: no usable address"
		return r
	}
	r.RemoteIP = ip

	// TCP connect: the round-trip time plus any middlebox cost.
	start = time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", t.Addr(ip))
	r.ConnectMs = msSince(start)
	if err != nil {
		r.Error = "connect: " + err.Error()
		return r
	}
	defer conn.Close()

	// TLS handshake: several more round trips, and the number that actually
	// predicts how a user experiences a first page load.
	start = time.Now()
	tc := tls.Client(conn, &tls.Config{ServerName: t.Host, MinVersion: tls.VersionTLS12})
	err = tc.HandshakeContext(ctx)
	r.TLSMs = msSince(start)
	if err != nil {
		r.Error = "tls: " + err.Error()
		return r
	}

	r.TotalMs = r.DNSMs + r.ConnectMs + r.TLSMs
	return r
}

func pickIP(addrs []net.IPAddr, preferV4 bool) string {
	if preferV4 {
		for _, a := range addrs {
			if a.IP.To4() != nil {
				return a.IP.String()
			}
		}
	}
	if len(addrs) > 0 {
		return addrs[0].IP.String()
	}
	return ""
}

func msSince(t time.Time) float64 {
	return float64(time.Since(t).Microseconds()) / 1000.0
}
