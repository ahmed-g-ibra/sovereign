"""Unauthenticated read API."""

from __future__ import annotations

import re

from fastapi import APIRouter, HTTPException, Query, Request

router = APIRouter(tags=["public"])

_WINDOW = re.compile(r"^(\d{1,4})([hd])$")


def parse_window(window: str) -> str:
    """Accept '24h' or '7d' and return a Postgres interval literal.

    An allowlist rather than string interpolation: the value still goes in as a
    bound parameter, but refusing anything that is not N hours or N days also
    keeps the query planner away from pathological ranges.
    """
    m = _WINDOW.match(window)
    if not m:
        raise HTTPException(422, "window must look like 24h or 7d")
    n, unit = int(m.group(1)), m.group(2)
    if n < 1:
        raise HTTPException(422, "window must be at least 1")
    hours = n if unit == "h" else n * 24
    if hours > 30 * 24:
        # Raw samples are kept 30 days. Longer windows need percentiles merged
        # across rollups, which cannot be done by averaging them -- that needs
        # a t-digest column, and it is not built yet. Better a clear 422 than a
        # quietly wrong number.
        raise HTTPException(422, "windows longer than 30d are not supported yet")
    return f"{hours} hours"


@router.get("/healthz")
async def healthz(request: Request) -> dict:
    pool = request.app.state.pool
    async with pool.connection() as conn:
        await conn.execute("SELECT 1")
    return {"status": "ok"}


@router.get("/v1/targets")
async def list_targets(request: Request, status: str | None = None) -> dict:
    pool = request.app.state.pool
    async with pool.connection() as conn:
        cur = await conn.execute(
            """
            SELECT id, provider, region, country, city, host, port, status,
                   is_control, note
            FROM targets
            WHERE (%s::text IS NULL OR status = %s)
            ORDER BY provider, id
            """,
            (status, status),
        )
        return {"targets": await cur.fetchall()}


@router.get("/v1/probes")
async def list_probes(request: Request) -> dict:
    """Public probe list. No keys, no IPs: contributors are not the product."""
    pool = request.app.state.pool
    async with pool.connection() as conn:
        cur = await conn.execute(
            """
            SELECT id, name, country, city, asn, tier, enrolled_at,
                   last_seen_at, quarantined,
                   (last_seen_at > now() - interval '10 minutes') AS reporting
            FROM probes
            WHERE revoked_at IS NULL
            ORDER BY country, name
            """
        )
        return {"probes": await cur.fetchall()}


@router.get("/v1/matrix")
async def matrix(request: Request,
                 window: str = Query("24h"),
                 min_samples: int = Query(10, ge=1)) -> dict:
    """Latency per target, grouped by the country the probe sits in.

    Computed from raw samples rather than from the rollup tables, because
    percentiles cannot be averaged: merging twelve hourly p95 values does not
    give you the p95 of the period. The rollups exist for charts over time,
    where each bucket is read on its own.
    """
    pool = request.app.state.pool
    interval = parse_window(window)

    async with pool.connection() as conn:
        cur = await conn.execute(
            """
            SELECT
                s.target_id,
                t.provider,
                t.region,
                t.is_control,
                p.country AS probe_country,
                count(*)::int AS samples,
                count(*) FILTER (WHERE s.error IS NOT NULL)::int AS failures,
                round(((count(*) FILTER (WHERE s.error IS NOT NULL))::numeric
                       / count(*) * 100), 2) AS loss_pct,
                round(percentile_cont(0.50) WITHIN GROUP (ORDER BY s.total_ms)
                      FILTER (WHERE s.error IS NULL)::numeric, 1) AS p50_ms,
                round(percentile_cont(0.95) WITHIN GROUP (ORDER BY s.total_ms)
                      FILTER (WHERE s.error IS NULL)::numeric, 1) AS p95_ms
            FROM samples s
            JOIN probes  p ON p.id = s.probe_id
            JOIN targets t ON t.id = s.target_id
            WHERE s.ts > now() - %s::interval
              AND p.quarantined IS FALSE
              AND p.revoked_at IS NULL
            GROUP BY s.target_id, t.provider, t.region, t.is_control, p.country
            HAVING count(*) >= %s
            ORDER BY p50_ms NULLS LAST
            """,
            (interval, min_samples),
        )
        rows = await cur.fetchall()

    return {
        "window": window,
        "min_samples": min_samples,
        "note": "Percentiles over successful probes only. Quarantined probes excluded.",
        "rows": rows,
    }
