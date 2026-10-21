# 7. At-least-once ingest with idempotent writes

Date: 2026-10-21

## Status

Accepted

## Context

Probes run on home connections, cheap VPS instances and volunteer hardware.
They will lose network access, be rebooted mid-upload, and see responses
disappear after the server has already committed the data.

Exactly-once delivery over an unreliable network needs coordination that is
disproportionate here. The alternatives were: drop data on failure (loses
precisely the outage we most want recorded), or buffer and risk duplicates.

## Decision

At-least-once delivery, made safe by idempotent writes.

- The agent writes samples to a disk spool *before* attempting any upload.
- Data is deleted only after the server acknowledges it.
- The server deduplicates on the natural key (probe, target, timestamp) with
  `ON CONFLICT DO NOTHING`, and reports how many rows were genuinely new.
- Rollups are recomputed over a window rather than incremented, so a late batch
  corrects its bucket instead of inflating it.

## Consequences

- Retrying is free and needs no coordination. The agent can be naive about
  failure, which is the right place for the simplicity to live.
- An agent offline for a fortnight replays on reconnect. The ingest endpoint
  creates whatever partitions that batch needs rather than rejecting data we
  asked the probe to keep.
- The spool is capped and drops oldest-first when full: an unbounded buffer
  fills a volunteer's disk, which is how you lose a volunteer.
- The response distinguishes accepted from duplicate counts, so a probe stuck
  in a replay loop is visible rather than silent.
- Verified end to end: killing the control plane mid-run, restarting the agent
  cold, and bringing the server back replayed 22 buffered samples with zero
  duplicate rows.
