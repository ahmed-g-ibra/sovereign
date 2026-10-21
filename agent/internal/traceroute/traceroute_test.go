package traceroute

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"
)

// fakeConn scripts one response per TTL so the runner can be exercised
// without touching the network.
type fakeConn struct {
	ttl       int
	responses map[int]fakeResponse // keyed by TTL
	written   []int                // TTLs actually probed, in order
	deadline  time.Time
	lastSeq   uint16
	served    bool
}

type fakeResponse struct {
	from    string
	final   bool
	silent  bool // nothing answers
	foreign bool // a reply to somebody else's probe arrives first
	err     error
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func (f *fakeConn) SetTTL(ttl int) error          { f.ttl = ttl; return nil }
func (f *fakeConn) SetDeadline(t time.Time) error { f.deadline = t; return nil }
func (f *fakeConn) Close() error                  { return nil }

func (f *fakeConn) WriteTo(b []byte, dst net.IP) error {
	f.written = append(f.written, f.ttl)
	f.lastSeq = binary.BigEndian.Uint16(b[6:8])
	f.served = false
	return nil
}

func (f *fakeConn) ReadFrom(b []byte) (int, net.IP, error) {
	r, ok := f.responses[f.ttl]
	if !ok || r.silent {
		return 0, nil, timeoutErr{}
	}
	if r.err != nil {
		return 0, nil, r.err
	}
	if r.foreign && !f.served {
		// A packet for a different conversation, which must be skipped rather
		// than mistaken for this hop's answer.
		f.served = true
		pkt := buildTimeExceeded(f.lastSeq + 9999)
		copy(b, pkt)
		return len(pkt), net.ParseIP("10.0.0.99"), nil
	}

	var pkt []byte
	if r.final {
		pkt = echoRequest(1, f.lastSeq, 8)
		pkt[0] = TypeEchoReply
	} else {
		pkt = buildTimeExceeded(f.lastSeq)
	}
	copy(b, pkt)
	return len(pkt), net.ParseIP(r.from), nil
}

func buildTimeExceeded(seq uint16) []byte {
	original := echoRequest(1, seq, 16)
	out := make([]byte, icmpHeaderLen)
	out[0] = TypeTimeExceeded
	ip := make([]byte, minIPv4HeaderLen)
	ip[0] = 0x45
	out = append(out, ip...)
	return append(out, original[:icmpHeaderLen]...)
}

func newTracer(responses map[int]fakeResponse) (*Tracer, *fakeConn) {
	fc := &fakeConn{responses: responses}
	return &Tracer{conn: fc, id: 1}, fc
}

func TestTraceWalksUntilTheDestinationAnswers(t *testing.T) {
	tr, fc := newTracer(map[int]fakeResponse{
		1: {from: "192.168.1.1"},
		2: {from: "10.52.0.1"},
		3: {from: "213.42.0.1"},
		4: {from: "3.5.50.169", final: true},
	})

	got, err := tr.Trace(context.Background(), "aws-me-central-1", "localhost",
		Options{MaxTTL: 10, HopTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete {
		t.Error("trace should be complete once the destination replies")
	}
	if len(got.Hops) != 4 {
		t.Fatalf("got %d hops, want 4", len(got.Hops))
	}
	if got.Hops[0].IP != "192.168.1.1" || got.Hops[3].IP != "3.5.50.169" {
		t.Errorf("unexpected path: %+v", got.Hops)
	}
	if !got.Hops[3].Final {
		t.Error("last hop should be marked final")
	}
	if fc.written[0] != 1 || fc.written[3] != 4 {
		t.Errorf("TTLs probed out of order: %v", fc.written)
	}
}

func TestSilentHopsAreRecordedNotFatal(t *testing.T) {
	// Routers that decline to send Time Exceeded are extremely common. A
	// traceroute that gave up at the first silence would be useless.
	tr, _ := newTracer(map[int]fakeResponse{
		1: {from: "192.168.1.1"},
		2: {silent: true},
		3: {silent: true},
		4: {from: "3.5.50.169", final: true},
	})

	got, err := tr.Trace(context.Background(), "t", "localhost",
		Options{MaxTTL: 10, HopTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hops) != 4 || !got.Complete {
		t.Fatalf("got %d hops, complete=%v", len(got.Hops), got.Complete)
	}
	if !got.Hops[1].Timeout || got.Hops[1].IP != "" {
		t.Errorf("silent hop recorded wrongly: %+v", got.Hops[1])
	}
	if got.Hops[3].IP != "3.5.50.169" {
		t.Error("the trace must continue past silent hops")
	}
}

func TestIncompleteTraceIsReportedHonestly(t *testing.T) {
	tr, _ := newTracer(map[int]fakeResponse{
		1: {from: "192.168.1.1"},
	})
	got, err := tr.Trace(context.Background(), "t", "localhost",
		Options{MaxTTL: 3, HopTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete {
		t.Error("never reached the destination, so Complete must be false")
	}
	if len(got.Hops) != 3 {
		t.Errorf("got %d hops, want MaxTTL=3", len(got.Hops))
	}
}

func TestForeignRepliesAreSkipped(t *testing.T) {
	// Another process on the machine is pinging. Its replies land on our
	// socket and must not be attributed to our hop.
	tr, _ := newTracer(map[int]fakeResponse{
		1: {from: "192.168.1.1", foreign: true},
		2: {from: "9.9.9.9", final: true},
	})
	got, err := tr.Trace(context.Background(), "t", "localhost",
		Options{MaxTTL: 5, HopTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if got.Hops[0].IP != "192.168.1.1" {
		t.Errorf("hop 1 = %q, want the real responder, not the foreign one",
			got.Hops[0].IP)
	}
}

func TestReadErrorsOtherThanTimeoutPropagate(t *testing.T) {
	boom := errors.New("socket exploded")
	tr, _ := newTracer(map[int]fakeResponse{1: {err: boom}})
	if _, err := tr.Trace(context.Background(), "t", "localhost",
		Options{MaxTTL: 2, HopTimeout: 20 * time.Millisecond}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestOptionDefaults(t *testing.T) {
	o := Options{}.withDefaults()
	if o.MaxTTL != 30 || o.HopTimeout != 2*time.Second || o.PayloadLen != 24 {
		t.Errorf("unexpected defaults: %+v", o)
	}
}

func TestTraceGivesUpAfterSustainedSilence(t *testing.T) {
	// An unreachable target must not cost MaxTTL x HopTimeout every hour.
	tr, fc := newTracer(map[int]fakeResponse{
		1: {from: "192.168.1.1"},
	})
	got, err := tr.Trace(context.Background(), "t", "localhost",
		Options{MaxTTL: 30, HopTimeout: 10 * time.Millisecond,
			MaxConsecutiveSilent: 4})
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete {
		t.Error("should not claim completion")
	}
	if len(fc.written) != 5 {
		t.Errorf("probed %d TTLs, want 1 answered + 4 silent", len(fc.written))
	}
}

func TestSilenceCounterResetsOnAnAnswer(t *testing.T) {
	// Scattered silence is normal and must not end the trace early.
	tr, _ := newTracer(map[int]fakeResponse{
		1: {from: "a"}, 2: {silent: true}, 3: {silent: true},
		4: {from: "b"}, 5: {silent: true}, 6: {silent: true},
		7: {from: "c", final: true},
	})
	got, err := tr.Trace(context.Background(), "t", "localhost",
		Options{MaxTTL: 30, HopTimeout: 10 * time.Millisecond,
			MaxConsecutiveSilent: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || len(got.Hops) != 7 {
		t.Errorf("complete=%v hops=%d, want true/7", got.Complete, len(got.Hops))
	}
}
