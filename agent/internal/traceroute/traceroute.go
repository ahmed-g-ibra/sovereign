package traceroute

import (
	"context"
	"errors"
	"net"
	"time"
)

// ErrUnsupported means this platform or this user cannot open an ICMP socket.
// It is not a failure to report upward: the agent keeps measuring latency and
// simply contributes no path data.
var ErrUnsupported = errors.New("traceroute: ICMP sockets unavailable")

// conn abstracts the socket so the runner can be tested without the network.
type conn interface {
	SetTTL(ttl int) error
	SetDeadline(t time.Time) error
	WriteTo(b []byte, dst net.IP) error
	ReadFrom(b []byte) (int, net.IP, error)
	Close() error
}

// Hop is one step along the path. An IP of "" means nothing answered within
// the timeout, which is common and not an error: plenty of routers decline to
// send Time Exceeded.
type Hop struct {
	TTL     int     `json:"ttl"`
	IP      string  `json:"ip,omitempty"`
	RTTMs   float64 `json:"rtt_ms,omitempty"`
	Final   bool    `json:"final,omitempty"`
	ASN     int     `json:"asn,omitempty"`
	ASName  string  `json:"as_name,omitempty"`
	Timeout bool    `json:"timeout,omitempty"`
}

// Trace is one complete path measurement.
type Trace struct {
	TargetID  string    `json:"target_id"`
	Host      string    `json:"host"`
	DestIP    string    `json:"dest_ip"`
	Timestamp time.Time `json:"ts"`
	Hops      []Hop     `json:"hops"`
	// Complete means we reached the destination rather than running out of TTL.
	Complete bool `json:"complete"`
}

// Options control one trace.
type Options struct {
	MaxTTL     int
	HopTimeout time.Duration
	PayloadLen int
	// MaxConsecutiveSilent abandons a trace after this many hops in a row go
	// unanswered. An interactive traceroute keeps going because a human is
	// watching; an hourly job against an unreachable target would otherwise
	// spend MaxTTL x HopTimeout -- a full minute -- learning nothing. The
	// trace is still recorded, just marked incomplete.
	MaxConsecutiveSilent int
}

func (o Options) withDefaults() Options {
	if o.MaxTTL <= 0 {
		o.MaxTTL = 30
	}
	if o.HopTimeout <= 0 {
		o.HopTimeout = 2 * time.Second
	}
	if o.PayloadLen <= 0 {
		o.PayloadLen = 24
	}
	if o.MaxConsecutiveSilent <= 0 {
		o.MaxConsecutiveSilent = 5
	}
	return o
}

// Tracer runs traces over one socket.
type Tracer struct {
	conn conn
	seq  uint16
	id   uint16
}

// New opens an ICMP socket. Returns ErrUnsupported where that is not permitted.
func New() (*Tracer, error) {
	c, err := newICMPConn()
	if err != nil {
		return nil, err
	}
	return &Tracer{conn: c, id: uint16(time.Now().UnixNano())}, nil
}

func (t *Tracer) Close() error { return t.conn.Close() }

// Trace walks the path to host by raising the TTL one hop at a time.
//
// Probes are sent one at a time and waited on. A parallel implementation would
// finish sooner, but it would also put a burst of ICMP at every router on the
// path, and this runs hourly against a fixed set of destinations: there is no
// reason to be in a hurry.
func (t *Tracer) Trace(ctx context.Context, targetID, host string,
	opts Options) (*Trace, error) {
	opts = opts.withDefaults()

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var dst net.IP
	for _, a := range ips {
		if v4 := a.IP.To4(); v4 != nil {
			dst = v4
			break
		}
	}
	if dst == nil {
		return nil, errors.New("traceroute: no IPv4 address for host")
	}

	out := &Trace{
		TargetID:  targetID,
		Host:      host,
		DestIP:    dst.String(),
		Timestamp: time.Now().UTC(),
	}

	silent := 0
	for ttl := 1; ttl <= opts.MaxTTL; ttl++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		hop, final, err := t.probe(dst, ttl, opts)
		if err != nil {
			return out, err
		}
		out.Hops = append(out.Hops, hop)
		if final {
			out.Complete = true
			break
		}

		if hop.Timeout {
			silent++
			if silent >= opts.MaxConsecutiveSilent {
				// The path has gone dark. Record what we learned and move on
				// rather than spending another half-minute on silence.
				break
			}
		} else {
			silent = 0
		}
	}
	return out, nil
}

func (t *Tracer) probe(dst net.IP, ttl int, opts Options) (Hop, bool, error) {
	t.seq++
	seq := t.seq

	if err := t.conn.SetTTL(ttl); err != nil {
		return Hop{}, false, err
	}
	deadline := time.Now().Add(opts.HopTimeout)
	if err := t.conn.SetDeadline(deadline); err != nil {
		return Hop{}, false, err
	}

	sent := time.Now()
	if err := t.conn.WriteTo(echoRequest(t.id, seq, opts.PayloadLen), dst); err != nil {
		return Hop{}, false, err
	}

	buf := make([]byte, 1500)
	for {
		n, from, err := t.conn.ReadFrom(buf)
		if err != nil {
			// A deadline here means nobody answered for this TTL. That is a
			// normal result, recorded as a silent hop, not an error.
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return Hop{TTL: ttl, Timeout: true}, false, nil
			}
			return Hop{}, false, err
		}

		r, perr := parseReply(buf[:n])
		if perr != nil || r.Seq != seq {
			// Another process's ping, or a reply to a probe we already gave up
			// on. Keep waiting until our own deadline.
			if time.Now().After(deadline) {
				return Hop{TTL: ttl, Timeout: true}, false, nil
			}
			continue
		}

		return Hop{
			TTL:   ttl,
			IP:    from.String(),
			RTTMs: float64(time.Since(sent).Microseconds()) / 1000.0,
			Final: r.Final,
		}, r.Final, nil
	}
}
