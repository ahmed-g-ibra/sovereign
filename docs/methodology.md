# Measurement methodology

Published before the numbers, because a latency comparison nobody can audit is
marketing.

## What is measured

Every probe, against every target, records four timings:

| Timing | What it is |
|---|---|
| **DNS** | time for the probe's configured resolver to return an address |
| **TCP connect** | TCP handshake to port 443 of the resolved address |
| **TLS handshake** | full TLS handshake, measured separately from the connect |
| **Total** | the sum of the three |

They are reported separately on purpose. A blended number tells you something is
slow; the split tells you whether it is the ISP's resolver, the path, or the
endpoint.

## Path measurement

Once an hour, each probe traces the path to every target: ICMP echo requests
with the TTL raised one hop at a time, listening for the Time Exceeded
messages routers send when a packet expires.

Each hop records the responding address, the round-trip time, and the
autonomous system announcing that address, resolved through Team Cymru's
public DNS service and cached for a week.

**The AS path is what gets compared.** Hop addresses churn — load balancers
rotate them between probes, and many routers never answer — so change
detection uses the ordered sequence of networks crossed, with consecutive
duplicates collapsed and silent hops dropped. A path that stops crossing one
transit network and starts crossing another is a real event; a router
answering from a different address in the same network is not.

A trace stops after five consecutive silent hops and is marked incomplete.
That is recorded, not hidden: a truncated path must never be mistaken for a
full one.

Path measurement is best-effort. A probe that cannot open an ICMP socket keeps
measuring latency and contributes no path data.

## What is not measured (yet)

- **Throughput.** Latency and bandwidth are different questions, and a bandwidth
  test from volunteer hardware is both expensive and easy to get wrong.
- **Azure per-region.** Azure publishes no consistent public per-region endpoint.
  Until Phase 2 deploys our own responders, Azure appears only as a global
  control.
- **Packet loss as a dedicated measurement.** Loss is currently inferred from
  failed latency probes rather than measured with a sustained stream.
- **IPv6 paths.** Traces are IPv4 only.

## Rules we hold ourselves to

1. **Percentiles, never averages.** p50, p90, p95, p99 and the sample count. An
   average over a bimodal distribution describes a state the network was never in.
2. **A silent probe is unknown, not down.** Missing data and a failed
   measurement are different rows, displayed differently.
3. **Failed probes never contribute timings.** They count toward loss only.
4. **A probe whose clock cannot be trusted is quarantined.** The server
   compares the batch's `sent_at` against its own clock. That measurement
   includes network transit, so it is an upper bound on skew rather than skew
   itself, and the threshold is a deliberately loose 2 seconds: it only ever
   catches a probe that is genuinely far out. Tighter measurement needs the
   agent to query NTP itself, which is not built yet. Quarantined data is
   stored and excluded from published figures, never deleted -- the quarantine
   may turn out to be the thing that was wrong.
5. **Probe host load is recorded with every sample.** A busy VPS reports latency
   that belongs to its own CPU scheduler, and that data has to be discardable
   after the fact.
6. **No single probe represents a country.** Aggregates below a minimum sample
   count are not published.
7. **Sequential measurement per probe.** Parallel probes from one connection
   contend for the same uplink and measure each other.
8. **IPv4 preferred for consistency.** A host that silently switches between v4
   and v6 produces a step change that looks like a network event and is not one.

## Endpoint selection

**Phase 1** measures against each provider's existing public regional endpoints.
Free, immediate, and slightly out of our control: the vendor decides what sits
behind that name.

**Phase 2** replaces them with our own responder deployed into each region by
Terraform — a container that returns a timestamp. Identical software in every
region on every provider, no reliance on vendor endpoint behaviour, and no
traffic sent to anyone else's service.

Targets marked `verify` in `agent/targets.json` are documented patterns we have
not confirmed. Run `probe -verify` and prune what fails; an endpoint that does
not resolve is a finding worth recording, not a bug.

## Delivery guarantees

Ingest is **at-least-once with idempotent writes**. An agent that uploaded
successfully but never saw the response will send the same batch again; the
server deduplicates on (probe, target, timestamp), so a replay changes nothing.

This is what lets the agent treat the network as unreliable without a
distributed-transaction story. It buffers to disk before attempting any upload,
keeps data until the server acknowledges it, and replays on reconnect -- an
outage produces a gap in *upload* time, never in measurement time.

Rollups are recomputed rather than accumulated, for the same reason: a late
batch has to correct the bucket it belongs to, not append a second one.

## Courtesy

Conservative intervals, no retries-on-retries, a real contact address on the
site, and a published list of exactly which endpoints we touch. We are measuring
networks, not stress-testing other people's services.
