from __future__ import annotations

import base64
import datetime as dt
import os
import secrets
import time

import psycopg
import pytest
import pytest_asyncio
from httpx import ASGITransport, AsyncClient

from sovereign_api import signing
from sovereign_api.app import create_app
from sovereign_api.db import make_pool, migrate
from sovereign_api.settings import Settings

DEFAULT_URL = "postgresql://sovereign:sovereign@localhost:55432/sovereign"
TEST_URL = os.environ.get("TEST_DATABASE_URL", DEFAULT_URL + "_test")


@pytest.fixture(scope="session")
def database_url() -> str:
    """Build the test database from scratch once per session."""
    admin_url = TEST_URL.rsplit("/", 1)[0] + "/postgres"
    dbname = TEST_URL.rsplit("/", 1)[1]
    with psycopg.connect(admin_url, autocommit=True) as conn:
        conn.execute(f'DROP DATABASE IF EXISTS "{dbname}" WITH (FORCE)')
        conn.execute(f'CREATE DATABASE "{dbname}"')
    return TEST_URL


@pytest_asyncio.fixture
async def app(database_url):
    settings = Settings(database_url=database_url)
    application = create_app(settings)

    # httpx's ASGI transport does not run lifespan events, so the pool is
    # wired up here instead of relying on the app's startup hook.
    pool = make_pool(database_url)
    await pool.open(wait=True, timeout=30)
    application.state.pool = pool

    async with pool.connection() as conn:
        await migrate(conn)
        # Every test starts from the same state. Targets survive because they
        # are reference data seeded by a migration.
        await conn.execute("TRUNCATE samples, probes, enrollment_tokens, "
                           "sample_rollup_5m, sample_rollup_1h CASCADE")
        await conn.execute("SELECT ensure_sample_partitions(2, 2)")

    try:
        yield application
    finally:
        await pool.close()


@pytest_asyncio.fixture
async def client(app):
    async with AsyncClient(transport=ASGITransport(app=app),
                           base_url="http://test") as c:
        yield c


@pytest_asyncio.fixture
async def enrollment_token(app) -> str:
    token = secrets.token_urlsafe(32)
    expires = dt.datetime.now(dt.timezone.utc) + dt.timedelta(hours=1)
    async with app.state.pool.connection() as conn:
        await conn.execute(
            "INSERT INTO enrollment_tokens (token_hash, max_uses, expires_at) "
            "VALUES (%s, %s, %s)",
            (signing.hash_token(token), 1, expires),
        )
    return token


class Agent:
    """A test double for the Go agent: holds a keypair and signs requests."""

    def __init__(self, client: AsyncClient, probe_id: str, private_key: str):
        self.client = client
        self.probe_id = probe_id
        self.private_key = private_key

    def headers(self, method: str, path: str, body: bytes,
                timestamp: int | None = None) -> dict[str, str]:
        ts = timestamp if timestamp is not None else int(time.time())
        return {
            signing.HEADER_PROBE: self.probe_id,
            signing.HEADER_TIMESTAMP: str(ts),
            signing.HEADER_SIGNATURE: signing.sign(self.private_key, method, path,
                                                   ts, body),
            "content-type": "application/json",
        }

    async def post(self, path: str, body: bytes, timestamp: int | None = None):
        return await self.client.post(
            path, content=body,
            headers=self.headers("POST", path, body, timestamp))

    async def get(self, path: str, timestamp: int | None = None):
        return await self.client.get(
            path, headers=self.headers("GET", path, b"", timestamp))


@pytest_asyncio.fixture
async def agent(client, enrollment_token) -> Agent:
    private, public = signing.generate_keypair()
    resp = await client.post("/v1/probes/register", json={
        "enrollment_token": enrollment_token,
        "public_key": public,
        "name": "test-muscat-1",
        "country": "OM",
        "city": "Muscat",
        "isp_claimed": "Omantel",
        "tier": "core",
    })
    assert resp.status_code == 201, resp.text
    return Agent(client, resp.json()["probe_id"], private)


def batch(samples: list[dict], sent_at: dt.datetime | None = None) -> bytes:
    import json
    sent = sent_at or dt.datetime.now(dt.timezone.utc)
    return json.dumps({"sent_at": sent.isoformat(), "samples": samples}).encode()


def sample(target_id: str = "aws-me-central-1", *, ts: dt.datetime | None = None,
           total_ms: float | None = 23.4, error: str | None = None,
           host_load1: float | None = 0.4) -> dict:
    when = ts or dt.datetime.now(dt.timezone.utc)
    out = {"target_id": target_id, "ts": when.isoformat(), "host_load1": host_load1}
    if error is not None:
        out["error"] = error
    else:
        out.update({"dns_ms": 2.1, "connect_ms": 10.2, "tls_ms": 11.1,
                    "total_ms": total_ms, "remote_ip": "3.5.50.169"})
    return out


def hop(ttl: int, ip: str | None = None, asn: int | None = None,
        rtt_ms: float | None = 5.0, final: bool = False) -> dict:
    if ip is None:
        return {"ttl": ttl, "timeout": True}
    return {"ttl": ttl, "ip": ip, "rtt_ms": rtt_ms, "asn": asn,
            "as_name": f"AS{asn}" if asn else None, "final": final}


def trace(target_id: str = "aws-me-central-1", *, ts: dt.datetime | None = None,
          hops: list[dict] | None = None, complete: bool = True,
          dest_ip: str = "3.5.50.14") -> dict:
    when = ts or dt.datetime.now(dt.timezone.utc)
    if hops is None:
        hops = [
            hop(1, "192.168.0.1"),
            hop(2, "81.161.90.253", 204170),
            hop(3, "5.102.177.197", 204170),
            hop(4),
            hop(5, "3.5.50.14", 16509, final=True),
        ]
    return {"target_id": target_id, "ts": when.isoformat(), "dest_ip": dest_ip,
            "complete": complete, "hops": hops}


def trace_batch(traces: list[dict], sent_at: dt.datetime | None = None) -> bytes:
    import json
    sent = sent_at or dt.datetime.now(dt.timezone.utc)
    return json.dumps({"sent_at": sent.isoformat(), "traces": traces}).encode()
