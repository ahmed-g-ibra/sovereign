"""The signed agent API: config and ingest."""

from __future__ import annotations

import datetime as dt

from fastapi import APIRouter, Depends, HTTPException, Request

from .. import paths
from ..auth import AuthedProbe, authenticate_probe
from ..schemas import (BatchIn, BatchResponse, ConfigOut, TargetOut,
                       TraceBatchIn)

router = APIRouter(tags=["agent"])


@router.get("/v1/config", response_model=ConfigOut)
async def get_config(request: Request,
                     probe: AuthedProbe = Depends(authenticate_probe)) -> ConfigOut:
    settings = request.app.state.settings
    pool = request.app.state.pool

    async with pool.connection() as conn:
        await conn.execute(
            "UPDATE probes SET last_seen_at = now() WHERE id = %s", (probe.id,)
        )
        cur = await conn.execute(
            """
            SELECT id, provider, region, country, city, host, port, is_control
            FROM targets
            WHERE status = 'live'
            ORDER BY id
            """
        )
        rows = await cur.fetchall()

    return ConfigOut(
        interval_s=60,
        timeout_s=5,
        config_ttl_s=settings.config_ttl_s,
        traceroute_interval_s=settings.traceroute_interval_s,
        targets=[TargetOut(**r) for r in rows],
    )


@router.post("/v1/samples", response_model=BatchResponse)
async def ingest(body: BatchIn, request: Request,
                 probe: AuthedProbe = Depends(authenticate_probe)) -> BatchResponse:
    settings = request.app.state.settings
    pool = request.app.state.pool

    if not body.samples:
        return BatchResponse(accepted=0, duplicates=0,
                             quarantined=probe.quarantined,
                             clock_skew_ms=probe.clock_skew_ms)

    if len(body.samples) > settings.max_batch_samples:
        raise HTTPException(413, f"batch exceeds {settings.max_batch_samples} samples")

    now = dt.datetime.now(dt.timezone.utc)
    skew_ms = int((now - body.sent_at).total_seconds() * 1000)
    quarantine = abs(skew_ms) > settings.clock_skew_quarantine_ms

    oldest = min(s.ts for s in body.samples)
    newest = max(s.ts for s in body.samples)
    if newest > now + dt.timedelta(minutes=5):
        # A sample from the future is a broken clock, not a measurement.
        raise HTTPException(422, "batch contains samples dated in the future")

    async with pool.connection() as conn:
        async with conn.transaction():
            # An agent that buffered through a two-week outage will replay
            # samples older than any partition we have. Create what the batch
            # needs rather than rejecting data we asked it to keep.
            days_back = max(1, (now.date() - oldest.date()).days + 1)
            await conn.execute("SELECT ensure_sample_partitions(%s, 1)", (days_back,))

            # One round trip for the whole batch, and ON CONFLICT DO NOTHING
            # makes replay free: an agent that never saw our 200 can resend the
            # same batch forever without corrupting anything. RETURNING counts
            # only the rows that were genuinely new.
            cur = await conn.execute(
                """
                INSERT INTO samples (probe_id, target_id, ts, dns_ms, connect_ms,
                                     tls_ms, total_ms, remote_ip, error, host_load1)
                SELECT %s, t.target_id, t.ts, t.dns_ms, t.connect_ms, t.tls_ms,
                       t.total_ms, t.remote_ip::inet, t.error, t.host_load1
                FROM unnest(
                    %s::text[], %s::timestamptz[], %s::real[], %s::real[],
                    %s::real[], %s::real[], %s::text[], %s::text[], %s::real[]
                ) AS t(target_id, ts, dns_ms, connect_ms, tls_ms, total_ms,
                       remote_ip, error, host_load1)
                ON CONFLICT (probe_id, target_id, ts) DO NOTHING
                RETURNING 1
                """,
                (
                    probe.id,
                    [s.target_id for s in body.samples],
                    [s.ts for s in body.samples],
                    [s.dns_ms for s in body.samples],
                    [s.connect_ms for s in body.samples],
                    [s.tls_ms for s in body.samples],
                    [s.total_ms for s in body.samples],
                    [s.remote_ip for s in body.samples],
                    [s.error for s in body.samples],
                    [s.host_load1 for s in body.samples],
                ),
            )
            accepted = len(await cur.fetchall())

            latest_load = next(
                (s.host_load1 for s in sorted(body.samples, key=lambda x: x.ts, reverse=True)
                 if s.host_load1 is not None), None)

            await conn.execute(
                """
                UPDATE probes
                   SET last_seen_at       = now(),
                       last_clock_skew_ms = %s,
                       last_host_load1    = COALESCE(%s, last_host_load1),
                       quarantined        = quarantined OR %s,
                       quarantine_reason  = CASE
                           WHEN %s THEN 'clock skew ' || %s || ' ms exceeds threshold'
                           ELSE quarantine_reason END
                 WHERE id = %s
                """,
                (skew_ms, latest_load, quarantine, quarantine, skew_ms, probe.id),
            )

    return BatchResponse(
        accepted=accepted,
        duplicates=len(body.samples) - accepted,
        quarantined=probe.quarantined or quarantine,
        clock_skew_ms=skew_ms,
    )


@router.post("/v1/traceroutes", response_model=BatchResponse)
async def ingest_traces(body: TraceBatchIn, request: Request,
                        probe: AuthedProbe = Depends(authenticate_probe)) -> BatchResponse:
    """Store path measurements.

    Same contract as sample ingest: at-least-once delivery made safe by
    deduplicating on (probe, target, timestamp), so a buffered agent can
    replay freely.
    """
    settings = request.app.state.settings
    pool = request.app.state.pool

    if not body.traces:
        return BatchResponse(accepted=0, duplicates=0,
                             quarantined=probe.quarantined,
                             clock_skew_ms=probe.clock_skew_ms)

    now = dt.datetime.now(dt.timezone.utc)
    skew_ms = int((now - body.sent_at).total_seconds() * 1000)
    quarantine = abs(skew_ms) > settings.clock_skew_quarantine_ms

    accepted = 0
    async with pool.connection() as conn:
        async with conn.transaction():
            for trace in body.traces:
                if trace.ts > now + dt.timedelta(minutes=5):
                    raise HTTPException(422, "trace dated in the future")

                path = paths.as_path(h.asn for h in trace.hops)
                cur = await conn.execute(
                    """
                    INSERT INTO traceroutes (probe_id, target_id, ts, dest_ip,
                                             complete, hop_count, answered,
                                             as_path, as_path_hash)
                    VALUES (%s, %s, %s, %s::inet, %s, %s, %s, %s, %s)
                    ON CONFLICT (probe_id, target_id, ts) DO NOTHING
                    RETURNING id
                    """,
                    (probe.id, trace.target_id, trace.ts, trace.dest_ip,
                     trace.complete, len(trace.hops),
                     sum(1 for h in trace.hops if h.ip),
                     path, paths.path_hash(path)),
                )
                row = await cur.fetchone()
                if row is None:
                    continue  # already held; replay is free

                accepted += 1
                if trace.hops:
                    await conn.execute(
                        """
                        INSERT INTO traceroute_hops (traceroute_id, ttl, hop_ip,
                                                     rtt_ms, asn, as_name, final)
                        SELECT %s, h.ttl, h.ip::inet, h.rtt, h.asn, h.as_name, h.final
                        FROM unnest(%s::smallint[], %s::text[], %s::real[],
                                    %s::integer[], %s::text[], %s::boolean[])
                             AS h(ttl, ip, rtt, asn, as_name, final)
                        """,
                        (row["id"],
                         [h.ttl for h in trace.hops],
                         [h.ip for h in trace.hops],
                         [h.rtt_ms for h in trace.hops],
                         [h.asn or None for h in trace.hops],
                         [h.as_name for h in trace.hops],
                         [h.final for h in trace.hops]),
                    )

            await conn.execute(
                """
                UPDATE probes
                   SET last_seen_at = now(),
                       quarantined  = quarantined OR %s
                 WHERE id = %s
                """,
                (quarantine, probe.id),
            )

    return BatchResponse(
        accepted=accepted,
        duplicates=len(body.traces) - accepted,
        quarantined=probe.quarantined or quarantine,
        clock_skew_ms=skew_ms,
    )
