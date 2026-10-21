from __future__ import annotations

import datetime as dt
import json
import time

from tests.conftest import batch, sample


async def test_signed_batch_is_accepted(agent):
    body = batch([sample()])
    resp = await agent.post("/v1/samples", body)
    assert resp.status_code == 200, resp.text
    assert resp.json()["accepted"] == 1


async def test_replay_is_idempotent(agent):
    """The core guarantee for a buffering agent.

    An agent that uploaded successfully but never saw the response must be able
    to send the same batch again without creating duplicate measurements.
    """
    body = batch([sample(), sample(target_id="gcp-me-central1")])

    first = await agent.post("/v1/samples", body)
    assert first.json() == {"accepted": 2, "duplicates": 0,
                            "quarantined": False,
                            "clock_skew_ms": first.json()["clock_skew_ms"]}

    second = await agent.post("/v1/samples", body)
    assert second.json()["accepted"] == 0
    assert second.json()["duplicates"] == 2


async def test_unsigned_request_is_rejected(client):
    resp = await client.post("/v1/samples", content=batch([sample()]),
                             headers={"content-type": "application/json"})
    assert resp.status_code == 401


async def test_tampered_body_is_rejected(agent):
    """Sign one body, send another."""
    signed_body = batch([sample(total_ms=23.4)])
    headers = agent.headers("POST", "/v1/samples", signed_body)
    tampered = batch([sample(total_ms=1.0)])

    resp = await agent.client.post("/v1/samples", content=tampered, headers=headers)
    assert resp.status_code == 401


async def test_stale_timestamp_is_rejected(agent):
    old = int(time.time()) - 3600
    resp = await agent.post("/v1/samples", batch([sample()]), timestamp=old)
    assert resp.status_code == 401
    assert "window" in resp.json()["detail"]


async def test_signature_from_another_probe_is_rejected(agent, client,
                                                        enrollment_token, app):
    """A valid signature over the right bytes, but the wrong probe id."""
    from sovereign_api import signing
    other_private, other_public = signing.generate_keypair()

    body = batch([sample()])
    ts = int(time.time())
    resp = await client.post("/v1/samples", content=body, headers={
        signing.HEADER_PROBE: agent.probe_id,
        signing.HEADER_TIMESTAMP: str(ts),
        signing.HEADER_SIGNATURE: signing.sign(other_private, "POST",
                                               "/v1/samples", ts, body),
        "content-type": "application/json",
    })
    assert resp.status_code == 401


async def test_failed_sample_carrying_timings_is_refused(agent):
    """Methodology rule 3, enforced at the edge."""
    bad = sample()
    bad["error"] = "connect: timeout"
    resp = await agent.post("/v1/samples", batch([bad]))
    assert resp.status_code == 422


async def test_failed_sample_without_timings_is_stored(agent, app):
    failed = sample(error="connect: i/o timeout")
    resp = await agent.post("/v1/samples", batch([failed]))
    assert resp.status_code == 200
    assert resp.json()["accepted"] == 1

    async with app.state.pool.connection() as conn:
        cur = await conn.execute("SELECT error, total_ms FROM samples")
        row = await cur.fetchone()
    assert row["error"] == "connect: i/o timeout"
    assert row["total_ms"] is None


async def test_future_samples_are_refused(agent):
    future = dt.datetime.now(dt.timezone.utc) + dt.timedelta(hours=2)
    resp = await agent.post("/v1/samples", batch([sample(ts=future)]))
    assert resp.status_code == 422


async def test_buffered_replay_from_before_any_partition_is_accepted(agent, app):
    """An agent offline for a fortnight still gets its data in.

    This is the scenario the spool exists for, and it is also the one that
    breaks naive partitioning: the target partition does not exist yet.
    """
    old = dt.datetime.now(dt.timezone.utc) - dt.timedelta(days=12)
    resp = await agent.post("/v1/samples", batch([sample(ts=old)]))
    assert resp.status_code == 200, resp.text
    assert resp.json()["accepted"] == 1


async def test_clock_skew_quarantines_the_probe(agent, app):
    """Methodology rule 4.

    The batch is still stored -- the data may be fine and the quarantine may
    later be lifted -- but the probe stops contributing to published numbers.
    """
    stale_sent_at = dt.datetime.now(dt.timezone.utc) - dt.timedelta(seconds=30)
    body = batch([sample()], sent_at=stale_sent_at)
    resp = await agent.post("/v1/samples", body)

    assert resp.status_code == 200
    assert resp.json()["quarantined"] is True
    assert resp.json()["accepted"] == 1

    async with app.state.pool.connection() as conn:
        cur = await conn.execute(
            "SELECT quarantined, quarantine_reason, last_host_load1 FROM probes")
        row = await cur.fetchone()
    assert row["quarantined"] is True
    assert "clock skew" in row["quarantine_reason"]
    assert row["last_host_load1"] == 0.4


async def test_oversized_batch_is_refused(agent):
    body = batch([sample(ts=dt.datetime.now(dt.timezone.utc)
                         - dt.timedelta(seconds=i)) for i in range(5001)])
    resp = await agent.post("/v1/samples", body)
    assert resp.status_code == 413


async def test_config_lists_only_live_targets(agent):
    resp = await agent.get("/v1/config")
    assert resp.status_code == 200
    body = resp.json()
    ids = {t["id"] for t in body["targets"]}
    assert "aws-me-central-1" in ids
    # Rejected and announced targets must never reach an agent.
    assert "azure-qatarcentral-blob" not in ids
    assert "aws-sa-pending" not in ids
    assert body["config_ttl_s"] > 0
