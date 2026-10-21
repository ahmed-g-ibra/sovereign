from __future__ import annotations

import datetime as dt

from sovereign_api.paths import as_path, path_hash
from tests.conftest import hop, trace, trace_batch


# --- pure derivation --------------------------------------------------------

def test_as_path_collapses_consecutive_duplicates():
    # One network contributing six hops is still one network in the path.
    assert as_path([204170, 204170, 204170, 16509, 16509]) == [204170, 16509]


def test_as_path_drops_silent_and_unresolved_hops():
    # A router that does not answer carries no network identity. Leaving a gap
    # marker in would make two identical paths compare unequal.
    assert as_path([204170, None, 204170, 0, 16509]) == [204170, 16509]


def test_as_path_preserves_a_genuine_return_to_a_network():
    # Not a duplicate: the path leaves and comes back, which is real and worth
    # keeping.
    assert as_path([204170, 16509, 204170]) == [204170, 16509, 204170]


def test_as_path_of_nothing_is_empty():
    assert as_path([]) == []
    assert as_path([None, 0, None]) == []


def test_path_hash_is_stable_and_discriminating():
    a, b = [204170, 16509], [204170, 1299, 16509]
    assert path_hash(a) == path_hash(list(a))
    assert path_hash(a) != path_hash(b)
    assert len(path_hash(a)) == 16


# --- ingest -----------------------------------------------------------------

async def test_trace_is_stored_with_hops_and_derived_path(agent, app):
    resp = await agent.post("/v1/traceroutes", trace_batch([trace()]))
    assert resp.status_code == 200, resp.text
    assert resp.json()["accepted"] == 1

    async with app.state.pool.connection() as conn:
        cur = await conn.execute(
            "SELECT target_id, complete, hop_count, answered, as_path, "
            "as_path_hash, host(dest_ip) AS dest_ip FROM traceroutes")
        row = await cur.fetchone()
        cur = await conn.execute(
            "SELECT ttl, host(hop_ip) AS hop_ip, asn, final "
            "FROM traceroute_hops ORDER BY ttl")
        hops = await cur.fetchall()

    assert row["target_id"] == "aws-me-central-1"
    assert row["complete"] is True
    assert row["hop_count"] == 5
    assert row["answered"] == 4, "the silent hop should not count as answered"
    assert row["as_path"] == [204170, 16509]
    assert row["dest_ip"] == "3.5.50.14"

    assert len(hops) == 5
    assert hops[3]["hop_ip"] is None, "a silent hop keeps its place in the path"
    assert hops[4]["final"] is True


async def test_trace_replay_is_idempotent(agent, app):
    body = trace_batch([trace()])
    assert (await agent.post("/v1/traceroutes", body)).json()["accepted"] == 1

    second = await agent.post("/v1/traceroutes", body)
    assert second.json()["accepted"] == 0
    assert second.json()["duplicates"] == 1

    async with app.state.pool.connection() as conn:
        cur = await conn.execute("SELECT count(*) AS n FROM traceroute_hops")
        assert (await cur.fetchone())["n"] == 5, "replay duplicated hop rows"


async def test_unsigned_trace_upload_is_rejected(client):
    resp = await client.post("/v1/traceroutes", content=trace_batch([trace()]),
                             headers={"content-type": "application/json"})
    assert resp.status_code == 401


async def test_future_trace_is_refused(agent):
    future = dt.datetime.now(dt.timezone.utc) + dt.timedelta(hours=2)
    resp = await agent.post("/v1/traceroutes", trace_batch([trace(ts=future)]))
    assert resp.status_code == 422


async def test_duplicate_ttl_in_one_trace_is_refused(agent):
    bad = trace(hops=[hop(1, "10.0.0.1"), hop(1, "10.0.0.2")])
    assert (await agent.post("/v1/traceroutes", trace_batch([bad]))).status_code == 422


async def test_empty_batch_is_accepted(agent):
    resp = await agent.post("/v1/traceroutes", trace_batch([]))
    assert resp.status_code == 200
    assert resp.json()["accepted"] == 0


# --- reading ----------------------------------------------------------------

async def test_paths_endpoint_returns_hops_in_order(agent, client):
    await agent.post("/v1/traceroutes", trace_batch([trace()]))

    body = (await client.get("/v1/paths?window=1h")).json()
    # Addresses come back bare, not as inet with a /32 suffix.
    assert body["traces"][0]["dest_ip"] == "3.5.50.14"
    assert body["traces"][0]["hops"][0]["ip"] == "192.168.0.1"
    assert len(body["traces"]) == 1
    t = body["traces"][0]
    assert t["as_path"] == [204170, 16509]
    assert [h["ttl"] for h in t["hops"]] == [1, 2, 3, 4, 5]
    assert t["hops"][1]["as_name"] == "AS204170"
    assert t["probe_country"] == "OM"


async def test_paths_can_be_filtered_by_target(agent, client):
    now = dt.datetime.now(dt.timezone.utc)
    await agent.post("/v1/traceroutes", trace_batch([
        trace(ts=now),
        trace(target_id="gcp-me-central1", ts=now - dt.timedelta(seconds=1)),
    ]))
    body = (await client.get("/v1/paths?target_id=gcp-me-central1")).json()
    assert len(body["traces"]) == 1
    assert body["traces"][0]["target_id"] == "gcp-me-central1"


async def test_path_change_is_detected(agent, client):
    """The output the whole feature exists for."""
    now = dt.datetime.now(dt.timezone.utc)

    before = trace(ts=now - dt.timedelta(hours=2), hops=[
        hop(1, "81.161.90.253", 204170),
        hop(2, "3.5.50.14", 16509, final=True)])
    after = trace(ts=now, hops=[
        hop(1, "81.161.90.253", 204170),
        hop(2, "80.231.1.1", 6762),          # suddenly via a transit carrier
        hop(3, "3.5.50.14", 16509, final=True)])

    await agent.post("/v1/traceroutes", trace_batch([before]))
    await agent.post("/v1/traceroutes", trace_batch([after]))

    changes = (await client.get("/v1/paths/changes?window=1d")).json()["changes"]
    assert len(changes) == 1
    c = changes[0]
    assert c["previous_path"] == [204170, 16509]
    assert c["as_path"] == [204170, 6762, 16509]
    assert c["target_id"] == "aws-me-central-1"


async def test_an_unchanged_path_is_not_a_change(agent, client):
    now = dt.datetime.now(dt.timezone.utc)
    await agent.post("/v1/traceroutes", trace_batch([
        trace(ts=now - dt.timedelta(hours=1)),
        trace(ts=now),
    ]))
    changes = (await client.get("/v1/paths/changes?window=1d")).json()["changes"]
    assert changes == [], "the same path twice is not a change"


async def test_hop_address_churn_without_an_as_change_is_not_a_change(agent, client):
    """Load balancers rotate hop addresses constantly. Only networks count."""
    now = dt.datetime.now(dt.timezone.utc)
    first = trace(ts=now - dt.timedelta(hours=1), hops=[
        hop(1, "5.102.177.196", 204170),
        hop(2, "3.5.50.14", 16509, final=True)])
    second = trace(ts=now, hops=[
        hop(1, "5.102.177.199", 204170),   # different router, same network
        hop(2, "3.5.50.77", 16509, final=True)])

    await agent.post("/v1/traceroutes", trace_batch([first]))
    await agent.post("/v1/traceroutes", trace_batch([second]))

    assert (await client.get("/v1/paths/changes?window=1d")).json()["changes"] == []


async def test_quarantined_probes_are_excluded_from_paths(agent, client, app):
    await agent.post("/v1/traceroutes", trace_batch([trace()]))
    assert (await client.get("/v1/paths")).json()["traces"]

    async with app.state.pool.connection() as conn:
        await conn.execute("UPDATE probes SET quarantined = true")

    assert (await client.get("/v1/paths")).json()["traces"] == []


async def test_config_tells_the_agent_how_often_to_trace(agent):
    cfg = (await agent.get("/v1/config")).json()
    assert cfg["traceroute_interval_s"] > cfg["interval_s"], \
        "tracing must be rarer than latency sampling"
