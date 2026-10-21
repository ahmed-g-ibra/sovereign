//go:build linux || darwin

package traceroute

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// icmpConn is a real ICMP socket.
//
// It is deliberately a SOCK_DGRAM socket rather than SOCK_RAW: the datagram
// form needs no root on macOS and, on Linux, needs only that the user's group
// falls inside net.ipv4.ping_group_range. Requiring sudo would mean asking
// volunteers to run an unfamiliar binary as root, which is a request most
// people should refuse.
//
// The kernel rewrites the echo identifier on a datagram ICMP socket, so
// replies are matched on the sequence number instead. See parseReply.
type icmpConn struct {
	pc net.PacketConn
}

func newICMPConn() (conn, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_ICMP)
	if err != nil {
		return nil, fmt.Errorf("%w: %v (on Linux this needs "+
			"net.ipv4.ping_group_range to include your group)", ErrUnsupported, err)
	}
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{}); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("bind icmp socket: %w", err)
	}

	// Hand the descriptor to the net package so reads participate in the
	// runtime poller and honour deadlines. FilePacketConn duplicates the
	// descriptor, so ours is closed either way.
	f := os.NewFile(uintptr(fd), "icmp")
	pc, err := net.FilePacketConn(f)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("wrap icmp socket: %w", err)
	}
	return &icmpConn{pc: pc}, nil
}

// SetTTL controls how many hops a probe survives, which is the entire
// mechanism of a traceroute: TTL n elicits a reply from the nth router.
func (c *icmpConn) SetTTL(ttl int) error {
	sc, ok := c.pc.(syscall.Conn)
	if !ok {
		return fmt.Errorf("socket does not expose a raw descriptor")
	}
	rc, err := sc.SyscallConn()
	if err != nil {
		return err
	}
	var setErr error
	if err := rc.Control(func(fd uintptr) {
		setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP,
			syscall.IP_TTL, ttl)
	}); err != nil {
		return err
	}
	return setErr
}

func (c *icmpConn) SetDeadline(t time.Time) error { return c.pc.SetDeadline(t) }

func (c *icmpConn) WriteTo(b []byte, dst net.IP) error {
	// A datagram ICMP socket addresses peers as UDP addresses with no port.
	_, err := c.pc.WriteTo(b, &net.UDPAddr{IP: dst})
	return err
}

func (c *icmpConn) ReadFrom(b []byte) (int, net.IP, error) {
	n, addr, err := c.pc.ReadFrom(b)
	if err != nil {
		return n, nil, err
	}
	switch a := addr.(type) {
	case *net.UDPAddr:
		return n, a.IP, nil
	case *net.IPAddr:
		return n, a.IP, nil
	default:
		return n, nil, fmt.Errorf("unexpected address type %T", addr)
	}
}

func (c *icmpConn) Close() error { return c.pc.Close() }
