from __future__ import annotations

import datetime as dt
import secrets

import pytest

from sovereign_api import signing
from tests.conftest import Agent


async def test_register_returns_an_identity(client, enrollment_token):
    _, public = signing.generate_keypair()
    resp = await client.post("/v1/probes/register", json={
        "enrollment_token": enrollment_token,
        "public_key": public,
        "name": "muscat-home",
        "country": "OM",
    })
    assert resp.status_code == 201
    assert resp.json()["probe_id"]


async def test_private_key_is_never_sent_or_stored(app, client, enrollment_token):
    private, public = signing.generate_keypair()
    await client.post("/v1/probes/register", json={
        "enrollment_token": enrollment_token,
        "public_key": public,
        "name": "muscat-home",
        "country": "OM",
    })
    async with app.state.pool.connection() as conn:
        cur = await conn.execute("SELECT public_key FROM probes")
        rows = await cur.fetchall()
    stored = [r["public_key"] for r in rows]
    assert stored == [public]
    assert private not in stored, "the server must never hold a signing key"


async def test_bad_token_is_rejected(client):
    _, public = signing.generate_keypair()
    resp = await client.post("/v1/probes/register", json={
        "enrollment_token": secrets.token_urlsafe(32),
        "public_key": public,
        "name": "impostor",
        "country": "OM",
    })
    assert resp.status_code == 403


async def test_single_use_token_cannot_be_spent_twice(client, enrollment_token):
    for expected in (201, 403):
        _, public = signing.generate_keypair()
        resp = await client.post("/v1/probes/register", json={
            "enrollment_token": enrollment_token,
            "public_key": public,
            "name": "twice",
            "country": "OM",
        })
        assert resp.status_code == expected


async def test_expired_token_is_rejected(app, client):
    token = secrets.token_urlsafe(32)
    async with app.state.pool.connection() as conn:
        await conn.execute(
            "INSERT INTO enrollment_tokens (token_hash, max_uses, expires_at) "
            "VALUES (%s, 1, %s)",
            (signing.hash_token(token),
             dt.datetime.now(dt.timezone.utc) - dt.timedelta(seconds=1)),
        )
    _, public = signing.generate_keypair()
    resp = await client.post("/v1/probes/register", json={
        "enrollment_token": token, "public_key": public,
        "name": "late", "country": "OM",
    })
    assert resp.status_code == 403


async def test_duplicate_public_key_is_refused(app, client):
    """Two probes sharing a key would make their measurements indistinguishable."""
    _, public = signing.generate_keypair()
    tokens = []
    for _ in range(2):
        token = secrets.token_urlsafe(32)
        async with app.state.pool.connection() as conn:
            await conn.execute(
                "INSERT INTO enrollment_tokens (token_hash, max_uses, expires_at) "
                "VALUES (%s, 1, %s)",
                (signing.hash_token(token),
                 dt.datetime.now(dt.timezone.utc) + dt.timedelta(hours=1)),
            )
        tokens.append(token)

    first = await client.post("/v1/probes/register", json={
        "enrollment_token": tokens[0], "public_key": public,
        "name": "a", "country": "OM"})
    assert first.status_code == 201

    second = await client.post("/v1/probes/register", json={
        "enrollment_token": tokens[1], "public_key": public,
        "name": "b", "country": "OM"})
    assert second.status_code == 409
