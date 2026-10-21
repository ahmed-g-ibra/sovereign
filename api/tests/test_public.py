from __future__ import annotations

import datetime as dt

from tests.conftest import batch, sample


async def test_healthz(client):
    resp = await client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json() == {"status": "ok"}


async def test_targets_include_rejection_reasons(client):
    resp = await client.get("/v1/targets")
    targets = {t["id"]: t for t in resp.json()["targets"]}
    muscat = targets["oci-me-dcc-muscat-1"]
    assert muscat["status"] == "rejected"
    assert "no public endpoint" in muscat["note"]


async def test_probe_list_exposes_no_secrets(client, agent):
    resp = await client.get("/v1/probes")
    probe = resp.json()["probes"][0]
    assert probe["name"] == "test-muscat-1"
    assert "public_key" not in probe
    assert "observed_ip" not in probe


async def test_matrix_aggregates_by_probe_country(agent, client):
    now = dt.datetime.now(dt.timezone.utc)
    samples = [sample(ts=now - dt.timedelta(seconds=i), total_ms=float(10 + i))
               for i in range(20)]
    assert (await agent.post("/v1/samples", batch(samples))).status_code == 200

    resp = await client.get("/v1/matrix?window=1h&min_samples=10")
    rows = resp.json()["rows"]
    assert len(rows) == 1
    assert rows[0]["target_id"] == "aws-me-central-1"
    assert rows[0]["probe_country"] == "OM"
    assert rows[0]["samples"] == 20
    assert float(rows[0]["p50_ms"]) > 0


async def test_matrix_respects_min_samples(agent, client):
    await agent.post("/v1/samples", batch([sample()]))
    resp = await client.get("/v1/matrix?window=1h&min_samples=10")
    assert resp.json()["rows"] == []


async def test_matrix_excludes_quarantined_probes(agent, client, app):
    now = dt.datetime.now(dt.timezone.utc)
    samples = [sample(ts=now - dt.timedelta(seconds=i)) for i in range(15)]
    await agent.post("/v1/samples", batch(samples))

    assert (await client.get("/v1/matrix?window=1h&min_samples=10")).json()["rows"]

    async with app.state.pool.connection() as conn:
        await conn.execute("UPDATE probes SET quarantined = true")

    assert (await client.get("/v1/matrix?window=1h&min_samples=10")).json()["rows"] == []


async def test_long_windows_are_refused_rather_than_approximated(client):
    resp = await client.get("/v1/matrix?window=90d")
    assert resp.status_code == 422

    for bad in ["forever", "1w", "-5h", "0h"]:
        assert (await client.get(f"/v1/matrix?window={bad}")).status_code == 422


async def test_rollup_produces_buckets(agent, app):
    now = dt.datetime.now(dt.timezone.utc)
    samples = [sample(ts=now - dt.timedelta(seconds=i * 10), total_ms=float(20 + i))
               for i in range(12)]
    await agent.post("/v1/samples", batch(samples))

    async with app.state.pool.connection() as conn:
        await conn.execute("SELECT roll_up('5 minutes'::interval, %s, %s)",
                           (now - dt.timedelta(hours=1), now + dt.timedelta(minutes=1)))
        cur = await conn.execute(
            "SELECT samples, p50_ms, p95_ms, loss_pct FROM sample_rollup_5m")
        rows = await cur.fetchall()

    assert rows, "expected at least one 5-minute bucket"
    assert sum(r["samples"] for r in rows) == 12
    assert all(r["p50_ms"] is not None for r in rows)


async def test_published_data_carries_its_licence(agent, client):
    """A licence on a web page gets separated from the data the moment someone
    copies a JSON response into a notebook."""
    await agent.post("/v1/samples", batch([sample()]))

    for path in ("/v1/matrix?window=1h&min_samples=1", "/v1/paths",
                 "/v1/paths/changes"):
        body = (await client.get(path)).json()
        licence = body.get("license")
        assert licence, f"{path} publishes data with no licence"
        assert licence["name"] == "CC BY 4.0"
        assert "creativecommons.org" in licence["url"]
        assert "github.com" in licence["attribution"]
