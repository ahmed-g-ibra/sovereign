// Package traceroute discovers the network path to a target by sending ICMP
// echo requests with increasing TTL and listening for the Time Exceeded
// messages that routers send back.
//
// Latency tells you a path is slow. The path tells you why -- which transit
// networks the traffic crosses, and when that changes. A region launch or a
// peering change shows up here days before anyone announces it.
//
// Everything is standard library, including the ICMP wire format below.
// ADR 0003 committed the agent to zero dependencies so that someone asked to
// run it on their home network can audit it in one sitting, and that is worth
// more than the hundred lines this costs.
package traceroute

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ICMP message types we care about (RFC 792).
const (
	TypeEchoReply    = 0
	TypeDestUnreach  = 3
	TypeEchoRequest  = 8
	TypeTimeExceeded = 11
	icmpHeaderLen    = 8
	minIPv4HeaderLen = 20
)

var (
	// ErrNotForUs means the packet is valid ICMP but belongs to another
	// conversation -- a different process pinging, or a stale reply.
	ErrNotForUs = errors.New("icmp: packet does not match any of our probes")
	// ErrShort means the packet was truncated below what the format requires.
	ErrShort = errors.New("icmp: packet too short")
)

// echoRequest builds an ICMP echo request carrying the given id and sequence.
//
// The payload is padding: we match replies on the sequence number, not on
// payload contents, because an unprivileged ICMP socket has its id rewritten
// by the kernel and cannot be relied upon.
func echoRequest(id, seq uint16, payloadLen int) []byte {
	if payloadLen < 0 {
		payloadLen = 0
	}
	b := make([]byte, icmpHeaderLen+payloadLen)
	b[0] = TypeEchoRequest
	b[1] = 0 // code
	// b[2:4] is the checksum, filled in below.
	binary.BigEndian.PutUint16(b[4:6], id)
	binary.BigEndian.PutUint16(b[6:8], seq)
	for i := icmpHeaderLen; i < len(b); i++ {
		b[i] = byte(i) // deterministic filler, so captures are readable
	}
	binary.BigEndian.PutUint16(b[2:4], checksum(b))
	return b
}

// checksum is the internet checksum from RFC 1071: the one's complement of the
// one's complement sum of 16-bit words.
func checksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i : i+2]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// stripIPHeader removes a leading IPv4 header if the platform left one on.
//
// Linux hands a datagram ICMP socket the ICMP message alone. macOS and the
// BSDs include the IP header, so the same code reading b[0] as an ICMP type
// gets 0x45 -- the IPv4 version and header-length nibbles -- and concludes it
// is looking at ICMP type 69. Found exactly that way, against 8.8.8.8.
//
// The check is safe because no ICMP packet can begin with 0x4n: those type
// numbers are unassigned, and type 4 (source quench) was deprecated long ago
// and would still need a different second nibble.
func stripIPHeader(b []byte) []byte {
	if len(b) < minIPv4HeaderLen {
		return b
	}
	if b[0]>>4 != 4 {
		return b
	}
	ihl := int(b[0]&0x0f) * 4
	if ihl < minIPv4HeaderLen || len(b) < ihl+icmpHeaderLen {
		return b
	}
	return b[ihl:]
}

// reply is what we managed to understand from an incoming ICMP packet.
type reply struct {
	Type uint8
	Seq  uint16
	// Final reports whether this came from the destination itself rather than
	// from a router along the way.
	Final bool
}

// parseReply extracts the sequence number of the probe a packet refers to.
//
// For an echo reply that is simply the sequence in its own header. For a Time
// Exceeded it is inside the quoted copy of the original datagram that the
// router includes: IP header, then the first 8 bytes of our ICMP message,
// which is exactly enough to reach the sequence field.
func parseReply(b []byte) (reply, error) {
	b = stripIPHeader(b)
	if len(b) < icmpHeaderLen {
		return reply{}, ErrShort
	}
	switch t := b[0]; t {
	case TypeEchoReply:
		return reply{Type: t, Seq: binary.BigEndian.Uint16(b[6:8]), Final: true}, nil

	case TypeTimeExceeded, TypeDestUnreach:
		quoted := b[icmpHeaderLen:]
		if len(quoted) < minIPv4HeaderLen+icmpHeaderLen {
			return reply{}, ErrShort
		}
		// The quoted IP header carries its own length in the low nibble of
		// byte 0, counted in 32-bit words. Routers may quote options.
		ihl := int(quoted[0]&0x0f) * 4
		if ihl < minIPv4HeaderLen || len(quoted) < ihl+icmpHeaderLen {
			return reply{}, ErrShort
		}
		inner := quoted[ihl:]
		if inner[0] != TypeEchoRequest {
			return reply{}, ErrNotForUs
		}
		return reply{
			Type:  t,
			Seq:   binary.BigEndian.Uint16(inner[6:8]),
			Final: t == TypeDestUnreach,
		}, nil

	default:
		return reply{}, fmt.Errorf("%w: type %d", ErrNotForUs, t)
	}
}
