"""Probe enrolment: the only unsigned endpoint."""

from __future__ import annotations

from fastapi import APIRouter, HTTPException, Request
from psycopg.errors import UniqueViolation

from .. import signing
from ..schemas import RegisterRequest, RegisterResponse

router = APIRouter(tags=["enrolment"])


@router.post("/v1/probes/register", response_model=RegisterResponse, status_code=201)
async def register(body: RegisterRequest, request: Request) -> RegisterResponse:
    """Spend an enrolment token for a probe identity.

    The agent generates its own Ed25519 keypair and sends only the public half.
    Its private key never leaves the machine it runs on, which means this
    endpoint cannot leak a credential even if it is logged in full.
    """
    settings = request.app.state.settings
    pool = request.app.state.pool
    token_hash = signing.hash_token(body.enrollment_token)
    client_ip = request.client.host if request.client else None

    async with pool.connection() as conn:
        async with conn.transaction():
            # Claiming a use and checking the limit happen in one statement, so
            # two agents racing on a single-use token cannot both win: the
            # second one's WHERE clause no longer matches.
            cur = await conn.execute(
                """
                UPDATE enrollment_tokens
                   SET uses = uses + 1
                 WHERE token_hash = %s
                   AND revoked_at IS NULL
                   AND expires_at > now()
                   AND uses < max_uses
                RETURNING id
                """,
                (token_hash,),
            )
            if await cur.fetchone() is None:
                # One message for expired, spent, revoked and never-existed.
                raise HTTPException(403, "invalid or spent enrollment token")

            try:
                cur = await conn.execute(
                    """
                    INSERT INTO probes (name, country, city, isp_claimed,
                                        public_key, tier, observed_ip)
                    VALUES (%s, %s, %s, %s, %s, %s, %s)
                    RETURNING id
                    """,
                    (body.name, body.country.upper(), body.city, body.isp_claimed,
                     body.public_key, body.tier, client_ip),
                )
            except UniqueViolation:
                raise HTTPException(409, "that public key is already enrolled")

            probe = await cur.fetchone()

    return RegisterResponse(probe_id=str(probe["id"]),
                            config_ttl_s=settings.config_ttl_s)
