# 2. PostgreSQL, not a dedicated time-series database

Date: 2026-10-21

## Status

Accepted

## Context

The probe fleet generates time-series data, and the reflex choice is a
time-series database — Timestream, InfluxDB, ClickHouse, Prometheus with Thanos.

Projected Phase 2 volume:

```
30 probes x 12 endpoints x 1 sample/min
  = 360 samples/min
  = ~518,000/day
  = ~16,000,000/month
```

## Decision

PostgreSQL on RDS, with daily partitions on the raw sample table and scheduled
rollups to 5-minute and hourly aggregates. Raw samples retained 30 days,
5-minute aggregates one year, hourly aggregates indefinitely.

Revisit at 100M rows/month.

## Consequences

- One database to operate, back up and pay for, rather than two.
- The application already needs relational data (probes, targets, users, the
  residency knowledge base). Those joins stay trivial.
- 16M rows/month is unremarkable for partitioned Postgres. The rollup jobs are
  the only real work, and they are a scheduled SQL statement.
- If volume grows an order of magnitude, the migration path is ClickHouse for
  raw samples with Postgres keeping the relational data — a change the rollup
  boundary already isolates.
- Reaching for ClickHouse at 360 writes/minute would signal an inability to size
  a system, which is the opposite of the point.
