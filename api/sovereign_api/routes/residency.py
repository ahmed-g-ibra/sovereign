"""Residency knowledge base and the advisor."""

from __future__ import annotations

import json
import uuid

from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import PlainTextResponse
from pydantic import BaseModel, Field, field_validator

from .. import advisor, knowledge
from .public import parse_window

router = APIRouter(tags=["residency"])


class AdviseRequest(BaseModel):
    user_countries: list[str] = Field(min_length=1, max_length=10)
    data_classes: list[str] = Field(default_factory=lambda: ["personal"],
                                    min_length=1, max_length=6)
    window: str = "7d"
    min_samples: int = Field(10, ge=1)

    @field_validator("user_countries")
    @classmethod
    def upper_two_letter(cls, v: list[str]) -> list[str]:
        for c in v:
            if len(c) != 2:
                raise ValueError(f"'{c}' is not a two-letter country code")
        return [c.upper() for c in v]

    @field_validator("data_classes")
    @classmethod
    def known_classes(cls, v: list[str]) -> list[str]:
        unknown = set(v) - set(advisor.DATA_CLASSES)
        if unknown:
            raise ValueError(
                f"unknown data class(es) {sorted(unknown)}; "
                f"expected any of {list(advisor.DATA_CLASSES)}")
        return v


@router.get("/v1/residency")
async def list_residency() -> dict:
    entries = knowledge.load_entries()
    return {
        "status": knowledge.overall_status(entries),
        "note": ("Entries marked draft were written from secondary sources and "
                 "have not been checked against the instrument."),
        "countries": [e.public() for e in sorted(entries.values(),
                                                 key=lambda x: x.country)],
    }


@router.get("/v1/residency/{country}")
async def get_residency(country: str) -> dict:
    entries = knowledge.load_entries()
    entry = entries.get(country.upper())
    if entry is None:
        raise HTTPException(404, f"no residency entry for '{country.upper()}'")
    out = entry.public()
    out["body"] = entry.body
    return out


async def _latency_by_target(pool, window: str, probe_countries: list[str],
                             min_samples: int) -> dict[str, dict]:
    """Measured latency keyed by target, from probes in the given countries.

    Falls back to every probe when none of the user's countries host one, which
    is the normal case today: there is one probe, in Oman.
    """
    interval = parse_window(window)
    async with pool.connection() as conn:
        cur = await conn.execute(
            """
            SELECT s.target_id,
                   count(*)::int AS samples,
                   round(((count(*) FILTER (WHERE s.error IS NOT NULL))::numeric
                          / count(*) * 100), 2) AS loss_pct,
                   round(percentile_cont(0.50) WITHIN GROUP (ORDER BY s.total_ms)
                         FILTER (WHERE s.error IS NULL)::numeric, 1) AS p50_ms,
                   round(percentile_cont(0.95) WITHIN GROUP (ORDER BY s.total_ms)
                         FILTER (WHERE s.error IS NULL)::numeric, 1) AS p95_ms
            FROM samples s
            JOIN probes p ON p.id = s.probe_id
            WHERE s.ts > now() - %s::interval
              AND p.quarantined IS FALSE
              AND p.revoked_at IS NULL
              AND (%s::text[] IS NULL OR p.country = ANY(%s::text[]))
            GROUP BY s.target_id
            HAVING count(*) >= %s
            """,
            (interval, probe_countries or None, probe_countries or None, min_samples),
        )
        rows = await cur.fetchall()
    return {r["target_id"]: {
        "samples": r["samples"],
        "loss_pct": float(r["loss_pct"]) if r["loss_pct"] is not None else None,
        "p50_ms": float(r["p50_ms"]) if r["p50_ms"] is not None else None,
        "p95_ms": float(r["p95_ms"]) if r["p95_ms"] is not None else None,
    } for r in rows}


@router.post("/v1/advise", status_code=201)
async def advise(body: AdviseRequest, request: Request) -> dict:
    pool = request.app.state.pool
    entries = knowledge.load_entries()
    rules = knowledge.load_rules()

    async with pool.connection() as conn:
        cur = await conn.execute(
            """
            SELECT id, provider, region, country, city
            FROM targets
            WHERE status = 'live' AND is_control IS FALSE
            ORDER BY id
            """
        )
        candidates = await cur.fetchall()

        # Prefer probes in the user's own countries; fall back to all of them
        # and say so in the evidence block.
        cur = await conn.execute(
            "SELECT DISTINCT country FROM probes WHERE quarantined IS FALSE "
            "AND revoked_at IS NULL")
        have = {r["country"] for r in await cur.fetchall()}

    probe_countries = [c for c in body.user_countries if c in have]
    latency = await _latency_by_target(pool, body.window, probe_countries,
                                       body.min_samples)

    record = advisor.advise(
        user_countries=body.user_countries,
        data_classes=body.data_classes,
        entries=entries,
        rules=rules,
        candidate_regions=[dict(c) for c in candidates],
        latency=latency,
        window=body.window,
        probe_countries=probe_countries,
    )
    if not probe_countries:
        record["evidence"]["note"] += (
            " No probe in the specified countries; measurements are from every "
            "available vantage point.")

    async with pool.connection() as conn:
        cur = await conn.execute(
            """
            INSERT INTO decision_records (inputs, record, kb_status)
            VALUES (%s, %s, %s) RETURNING id
            """,
            (json.dumps(record["inputs"]), json.dumps(record), record["status"]),
        )
        record_id = (await cur.fetchone())["id"]

    record["id"] = str(record_id)
    record["permalink"] = f"/v1/advise/{record_id}"
    return record


async def _load_record(request: Request, record_id: str) -> dict:
    try:
        rid = uuid.UUID(record_id)
    except ValueError:
        raise HTTPException(404, "no such decision record")
    async with request.app.state.pool.connection() as conn:
        cur = await conn.execute(
            "SELECT id, record FROM decision_records WHERE id = %s", (rid,))
        row = await cur.fetchone()
    if row is None:
        raise HTTPException(404, "no such decision record")
    record = row["record"]
    record["id"] = str(row["id"])
    return record


@router.get("/v1/advise/{record_id}")
async def get_record(record_id: str, request: Request) -> dict:
    """A stored record never changes.

    The latency data and the rules move on; the artifact someone pasted into a
    design review must keep saying what it said when they pasted it.
    """
    return await _load_record(request, record_id)


@router.get("/v1/advise/{record_id}/markdown", response_class=PlainTextResponse)
async def get_record_markdown(record_id: str, request: Request) -> str:
    record = await _load_record(request, record_id)
    return advisor.to_markdown(record, record_id=record["id"])
