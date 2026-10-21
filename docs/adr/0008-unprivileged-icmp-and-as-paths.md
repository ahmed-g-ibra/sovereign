# 8. Unprivileged ICMP, and the AS path as the unit of comparison

Date: 2026-10-22

## Status

Accepted

## Context

Path measurement needs per-packet TTL control and the ability to receive ICMP
Time Exceeded. The usual answers are a raw socket, which needs root, or
`golang.org/x/net/ipv4`, which breaks the zero-dependency rule in ADR 0003.

Separately, a traceroute produces a list of hop addresses, and hop addresses
are a poor thing to compare over time: load balancers rotate them between
probes, and plenty of routers never answer at all.

## Decision

**Datagram ICMP sockets, not raw.** `SOCK_DGRAM` with `IPPROTO_ICMP` needs no
privileges on macOS and, on Linux, only that the user's group falls inside
`net.ipv4.ping_group_range`. TTL is set with `syscall.SetsockoptInt` through
the `syscall.RawConn` the net package already exposes, so the standard library
is sufficient and the agent keeps zero dependencies.

**The AS path is the comparable unit.** Hops are stored for verification, but
change detection compares the ordered sequence of autonomous systems with
consecutive duplicates collapsed and silent hops dropped.

**Stop after five consecutive silent hops.** An interactive traceroute keeps
going because a person is watching. An hourly job against an unreachable
target would spend a full minute learning nothing.

## Consequences

- No volunteer is ever asked to run an unfamiliar binary as root.
- A probe that cannot open an ICMP socket logs once and carries on measuring
  latency. Path data is best-effort; latency is not.
- The kernel rewrites the echo identifier on a datagram socket, so replies are
  matched on sequence number instead, read out of the quoted packet inside a
  Time Exceeded message.
- **macOS and the BSDs include the IP header in what the socket returns and
  Linux does not.** Reading `b[0]` as an ICMP type on macOS yields 69, which is
  the IPv4 version and header-length nibbles. The parser detects and strips a
  leading IPv4 header. This cost an hour and was found by running against
  8.8.8.8 and getting "type 69" back; there are tests for both framings.
- Collapsing duplicates means a network contributing six hops counts once,
  so a path that genuinely leaves a network and returns is still recorded as
  two entries.
- Dropping silent hops from the AS path means a router that answers one hour
  and not the next does not register as a route change.
- Early stopping makes some traces incomplete by design. `complete` is stored
  so nobody mistakes a truncated path for a full one.
