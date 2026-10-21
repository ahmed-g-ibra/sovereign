package traceroute

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestChecksumRFC1071(t *testing.T) {
	// A correctly checksummed packet sums to zero when re-checksummed.
	pkt := echoRequest(0x1234, 7, 16)
	if got := checksum(pkt); got != 0 {
		t.Errorf("re-checksum of a valid packet = %#04x, want 0", got)
	}
}

func TestChecksumHandlesOddLength(t *testing.T) {
	// Must not panic or read past the end on an odd number of bytes.
	if checksum([]byte{0x01, 0x02, 0x03}) == 0 {
		t.Error("expected a non-zero checksum for arbitrary odd-length input")
	}
}

func TestEchoRequestShape(t *testing.T) {
	pkt := echoRequest(0xbeef, 42, 24)
	if len(pkt) != icmpHeaderLen+24 {
		t.Fatalf("length %d, want %d", len(pkt), icmpHeaderLen+24)
	}
	if pkt[0] != TypeEchoRequest || pkt[1] != 0 {
		t.Errorf("type/code = %d/%d, want %d/0", pkt[0], pkt[1], TypeEchoRequest)
	}
	if got := binary.BigEndian.Uint16(pkt[6:8]); got != 42 {
		t.Errorf("sequence = %d, want 42", got)
	}
}

func TestParseEchoReply(t *testing.T) {
	pkt := echoRequest(1, 99, 8)
	pkt[0] = TypeEchoReply

	got, err := parseReply(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != 99 {
		t.Errorf("seq = %d, want 99", got.Seq)
	}
	if !got.Final {
		t.Error("an echo reply comes from the destination and must be final")
	}
}

// timeExceeded builds what a router sends back: its own ICMP header, then the
// IP header and first 8 bytes of the packet whose TTL expired.
func timeExceeded(originalSeq uint16, ihl int) []byte {
	original := echoRequest(1, originalSeq, 16)
	ip := make([]byte, ihl)
	ip[0] = byte(0x40 | ihl/4) // version 4, header length in words

	out := make([]byte, icmpHeaderLen)
	out[0] = TypeTimeExceeded
	out = append(out, ip...)
	out = append(out, original[:icmpHeaderLen]...)
	return out
}

func TestParseTimeExceededRecoversOurSequence(t *testing.T) {
	got, err := parseReply(timeExceeded(7, minIPv4HeaderLen))
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != 7 {
		t.Errorf("seq = %d, want 7", got.Seq)
	}
	if got.Final {
		t.Error("a Time Exceeded comes from an intermediate router, not the destination")
	}
}

func TestParseTimeExceededWithIPOptions(t *testing.T) {
	// Routers may quote an IP header carrying options, making it longer than
	// the 20-byte minimum. Reading a fixed offset would grab the wrong bytes.
	got, err := parseReply(timeExceeded(1234, 24))
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != 1234 {
		t.Errorf("seq = %d, want 1234 (IHL field ignored?)", got.Seq)
	}
}

func TestParseRejectsShortAndForeignPackets(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want error
	}{
		{"empty", nil, ErrShort},
		{"truncated header", []byte{11, 0, 0}, ErrShort},
		{"time exceeded without a full quote", append([]byte{11, 0, 0, 0, 0, 0, 0, 0},
			make([]byte, 10)...), ErrShort},
		{"someone else's protocol", []byte{9, 0, 0, 0, 0, 0, 0, 0}, ErrNotForUs},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseReply(c.in); !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestParseIgnoresQuotedNonEchoTraffic(t *testing.T) {
	// A Time Exceeded for somebody else's UDP packet must not be read as ours.
	pkt := timeExceeded(5, minIPv4HeaderLen)
	pkt[icmpHeaderLen+minIPv4HeaderLen] = 17 // quoted protocol is not an echo
	if _, err := parseReply(pkt); !errors.Is(err, ErrNotForUs) {
		t.Errorf("err = %v, want ErrNotForUs", err)
	}
}

// --- platform framing -------------------------------------------------------

// withIPHeader wraps a packet the way macOS and the BSDs deliver it.
func withIPHeader(payload []byte, ihl int) []byte {
	ip := make([]byte, ihl)
	ip[0] = byte(0x40 | ihl/4)
	return append(ip, payload...)
}

func TestParseHandlesBSDFramingWithLeadingIPHeader(t *testing.T) {
	// Regression: macOS delivers the IP header on a datagram ICMP socket,
	// Linux does not. Reading b[0] as an ICMP type on macOS yields 69.
	echo := echoRequest(1, 4242, 16)
	echo[0] = TypeEchoReply

	got, err := parseReply(withIPHeader(echo, minIPv4HeaderLen))
	if err != nil {
		t.Fatalf("BSD-framed echo reply rejected: %v", err)
	}
	if got.Seq != 4242 {
		t.Errorf("seq = %d, want 4242", got.Seq)
	}
	if !got.Final {
		t.Error("echo reply should be final")
	}
}

func TestParseHandlesBSDFramedTimeExceeded(t *testing.T) {
	got, err := parseReply(withIPHeader(timeExceeded(77, minIPv4HeaderLen),
		minIPv4HeaderLen))
	if err != nil {
		t.Fatalf("BSD-framed Time Exceeded rejected: %v", err)
	}
	if got.Seq != 77 || got.Final {
		t.Errorf("got seq=%d final=%v, want 77/false", got.Seq, got.Final)
	}
}

func TestParseStillHandlesLinuxFraming(t *testing.T) {
	// The strip must not eat the header on a platform that already removed it.
	got, err := parseReply(timeExceeded(55, minIPv4HeaderLen))
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != 55 {
		t.Errorf("seq = %d, want 55", got.Seq)
	}
}

func TestStripIPHeaderLeavesNonIPAlone(t *testing.T) {
	echo := echoRequest(1, 9, 24)
	if len(stripIPHeader(echo)) != len(echo) {
		t.Error("stripped bytes from a packet with no IP header")
	}
}
