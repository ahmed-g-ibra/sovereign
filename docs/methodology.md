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

## What is not measured (yet)

- **Throughput.** Latency and bandwidth are different questions, and a bandwidth
  test from volunteer hardware is both expensive and easy to get wrong.
- **Azure per-region.** Azure publishes no consistent public per-region endpoint.
  Until Phase 2 deploys our own responders, Azure appears only as a global
  control.
- **Packet loss.** Needs ICMP or a sustained stream; Phase 1 work.

## Rules we hold ourselves to

1. **Percentiles, never averages.** p50, p90, p95, p99 and the sample count. An
   average over a bimodal distribution describes a state the network was never in.
2. **A silent probe is unknown, not down.** Missing data and a failed
   measurement are different rows, displayed differently.
3. **Failed probes never contribute timings.** They count toward loss only.
4. **Clock skew over 500 ms quarantines a probe's data.**
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

## Courtesy

Conservative intervals, no retries-on-retries, a real contact address on the
site, and a published list of exactly which endpoints we touch. We are measuring
networks, not stress-testing other people's services.
