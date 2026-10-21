"""Probe authentication."""

from __future__ import annotations

import time
import uuid

from fastapi import HTTPException, Request

from . import signing
from .settings import Settings


class AuthedProbe:
    def __init__(self, row: dict, clock_skew_ms: int):
        self.id: uuid.UUID = row["id"]
        self.name: str = row["name"]
        self.quarantined: bool = row["quarantined"]
        self.clock_skew_ms = clock_skew_ms


async def authenticate_probe(request: Request) -> AuthedProbe:
    """Verify the signature on an agent request.

    Failures are deliberately indistinguishable from the outside: a missing
    probe, a revoked probe and a bad signature all return the same 401, so the
    endpoint cannot be used to enumerate valid probe ids.
    """
    settings: Settings = request.app.state.settings

    probe_id_raw = request.headers.get(signing.HEADER_PROBE)
    ts_raw = request.headers.get(signing.HEADER_TIMESTAMP)
    sig = request.headers.get(signing.HEADER_SIGNATURE)
    if not probe_id_raw or not ts_raw or not sig:
        raise HTTPException(401, "unauthenticated")

    try:
        probe_id = uuid.UUID(probe_id_raw)
        timestamp = int(ts_raw)
    except ValueError:
        raise HTTPException(401, "unauthenticated")

    now = int(time.time())
    skew = now - timestamp
    if abs(skew) > settings.signature_window_s:
        # Replay protection. Separated from the quarantine check below because
        # they answer different questions: this one is "is this request fresh",
        # that one is "can I trust this probe's timestamps at all".
        raise HTTPException(401, "timestamp outside acceptable window")

    body = await request.body()

    pool = request.app.state.pool
    async with pool.connection() as conn:
        cur = await conn.execute(
            """
            SELECT id, name, public_key, quarantined, revoked_at
            FROM probes WHERE id = %s
            """,
            (probe_id,),
        )
        row = await cur.fetchone()

    if row is None or row["revoked_at"] is not None:
        raise HTTPException(401, "unauthenticated")

    if not signing.verify(row["public_key"], sig, request.method,
                          request.url.path, timestamp, body):
        raise HTTPException(401, "unauthenticated")

    return AuthedProbe(row, clock_skew_ms=skew * 1000)
