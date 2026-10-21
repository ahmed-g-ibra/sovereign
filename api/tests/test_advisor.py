from __future__ import annotations

import datetime as dt

from tests.conftest import batch, sample


async def advise(client, **kwargs):
    body = {"user_countries": ["OM"], "data_classes": ["personal"]}
    body.update(kwargs)
    return await client.post("/v1/advise", json=body)


async def test_advice_lists_regions_with_verdicts(client):
    resp = await advise(client)
    assert resp.status_code == 201, resp.text
    rec = resp.json()

    assert rec["regions"], "no candidate regions returned"
    assert all(r["verdict"] in ("allowed", "conditional", "unknown", "blocked")
               for r in rec["regions"])
    # Controls are measurement references, not places to put a workload.
    assert "aws-eu-central-1" not in {r["target_id"] for r in rec["regions"]}


async def test_draft_knowledge_base_is_declared_loudly(client):
    rec = (await advise(client)).json()
    assert rec["status"] == "draft"
    assert any("DRAFT" in w for w in rec["warnings"])
    assert "not legal advice" in rec["disclaimer"]


async def test_every_jurisdiction_claim_carries_a_citation(client):
    rec = (await advise(client)).json()
    for j in rec["jurisdictions"]:
        assert j["citation"].startswith("knowledge/residency/")
        assert j["kb_status"] in ("draft", "reviewed", "verified")


async def test_health_data_in_uae_blocks_regions_outside_it(client):
    rec = (await advise(client, user_countries=["AE"],
                        data_classes=["health"])).json()
    by_country = {}
    for r in rec["regions"]:
        by_country.setdefault(r["country"], set()).add(r["verdict"])

    assert "blocked" not in by_country.get("AE", set()), \
        "UAE regions must satisfy a UAE localisation rule"
    for country, verdicts in by_country.items():
        if country != "AE":
            assert verdicts == {"blocked"}, f"{country} should be blocked, got {verdicts}"


async def test_unknown_jurisdiction_returns_unknown_not_a_guess(client):
    rec = (await advise(client, user_countries=["KW"])).json()
    assert rec["jurisdictions"][0]["verdict"] == "unknown"
    assert all(r["verdict"] in ("unknown", "blocked") for r in rec["regions"])


async def test_country_with_no_entry_is_skipped_not_invented(client):
    rec = (await advise(client, user_countries=["ZZ"])).json()
    assert rec["jurisdictions"] == []
    assert any("ZZ" in w for w in rec["warnings"])


async def test_bad_input_is_refused(client):
    assert (await advise(client, data_classes=["astrology"])).status_code == 422
    assert (await advise(client, user_countries=["OMN"])).status_code == 422
    assert (await advise(client, user_countries=[])).status_code == 422


async def test_latency_evidence_is_attached_when_it_exists(agent, client):
    now = dt.datetime.now(dt.timezone.utc)
    samples = [sample(ts=now - dt.timedelta(seconds=i), total_ms=float(20 + i))
               for i in range(15)]
    await agent.post("/v1/samples", batch(samples))

    rec = (await advise(client, min_samples=10)).json()
    measured = [r for r in rec["regions"] if r["latency"]]
    assert measured, "expected measured latency to be attached"
    assert measured[0]["target_id"] == "aws-me-central-1"
    assert measured[0]["latency"]["p50_ms"] > 0
    assert rec["evidence"]["regions_with_measurements"] == 1


async def test_no_measurements_is_stated_not_hidden(client):
    rec = (await advise(client)).json()
    assert rec["evidence"]["regions_with_measurements"] == 0
    assert any("No latency measurements" in w for w in rec["warnings"])


async def test_blocked_regions_rank_last(client):
    rec = (await advise(client, user_countries=["AE"],
                        data_classes=["health"])).json()
    verdicts = [r["verdict"] for r in rec["regions"]]
    assert verdicts == sorted(verdicts, key=lambda v: v == "blocked"), \
        "blocked regions should sort to the bottom"


async def test_record_is_permanent_and_unchanging(client, app):
    rec = (await advise(client)).json()
    record_id = rec["id"]

    # The world moves on: the knowledge base could change, new data arrives.
    fetched = (await client.get(f"/v1/advise/{record_id}")).json()
    assert fetched["generated_at"] == rec["generated_at"]
    assert fetched["regions"] == rec["regions"]
    assert fetched["status"] == rec["status"]


async def test_missing_record_is_404(client):
    assert (await client.get("/v1/advise/not-a-uuid")).status_code == 404
    assert (await client.get(
        "/v1/advise/00000000-0000-0000-0000-000000000000")).status_code == 404


async def test_markdown_export_is_pasteable(client):
    rec = (await advise(client)).json()
    resp = await client.get(f"/v1/advise/{rec['id']}/markdown")
    assert resp.status_code == 200
    text = resp.text

    assert text.startswith("# Cloud region decision record")
    assert "## Candidate regions" in text
    assert "| Region | Provider |" in text
    assert "not legal advice" in text
    assert "DRAFT" in text


async def test_residency_endpoints(client):
    listing = (await client.get("/v1/residency")).json()
    assert listing["status"] == "draft"
    assert {c["country"] for c in listing["countries"]} >= {"OM", "AE", "SA"}

    oman = (await client.get("/v1/residency/om")).json()
    assert oman["country"] == "OM"
    assert oman["open_questions"]
    assert "Royal Decree" in oman["instrument"]
    assert "body" in oman

    assert (await client.get("/v1/residency/ZZ")).status_code == 404


async def test_unreachable_target_with_total_loss_does_not_break_ranking(agent, client):
    """Regression: a target can have samples and no percentile.

    If every probe against it failed, p95 is NULL. Found by running the
    advisor against real data, where AWS Bahrain had 100% loss from the only
    vantage point.
    """
    now = dt.datetime.now(dt.timezone.utc)
    failed = [sample(target_id="aws-me-south-1", ts=now - dt.timedelta(seconds=i),
                     error="connect: i/o timeout") for i in range(12)]
    good = [sample(target_id="aws-me-central-1", ts=now - dt.timedelta(seconds=i),
                   total_ms=float(20 + i)) for i in range(12)]
    assert (await agent.post("/v1/samples", batch(failed + good))).status_code == 200

    resp = await advise(client, min_samples=10)
    assert resp.status_code == 201, resp.text
    rec = resp.json()

    by_id = {r["target_id"]: r for r in rec["regions"]}
    dead = by_id["aws-me-south-1"]
    assert dead["latency"]["samples"] == 12
    assert dead["latency"]["loss_pct"] == 100.0
    assert dead["latency"]["p95_ms"] is None

    # The unreachable one must not outrank a working region.
    order = [r["target_id"] for r in rec["regions"]]
    assert order.index("aws-me-central-1") < order.index("aws-me-south-1")
