//go:build !linux && !darwin

package traceroute

func newICMPConn() (conn, error) { return nil, ErrUnsupported }
