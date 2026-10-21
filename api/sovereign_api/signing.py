"""Request signing shared by the agent and the API.

Ed25519, not HMAC. With a shared secret the server must store something that
can impersonate any probe; here it stores only public keys, so a full database
dump lets an attacker read every measurement and forge none.

Canonical string, newline-joined:

    v1
    <HTTP method, uppercase>
    <path, no query string>
    <unix timestamp, seconds>
    <lowercase hex sha256 of the raw body>

Signature: Ed25519 over those bytes, base64, sent as
``X-Sovereign-Signature: v1=<base64>``.

The scheme version is inside the signed material, so a future v2 cannot be
downgraded to v1 by rewriting only the header.

Any change here must be mirrored in agent/internal/client/sign.go. The
cross-language fixture in tests/test_signing.py exists to catch it when it
is not.
"""

from __future__ import annotations

import base64
import hashlib

from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives.asymmetric.ed25519 import (
    Ed25519PrivateKey,
    Ed25519PublicKey,
)

SCHEME = "v1"

HEADER_PROBE = "X-Sovereign-Probe"
HEADER_TIMESTAMP = "X-Sovereign-Timestamp"
HEADER_SIGNATURE = "X-Sovereign-Signature"


def body_digest(body: bytes) -> str:
    return hashlib.sha256(body).hexdigest()


def canonical_string(method: str, path: str, timestamp: int, body: bytes) -> bytes:
    return "\n".join(
        [SCHEME, method.upper(), path, str(timestamp), body_digest(body)]
    ).encode()


def sign(private_key_b64: str, method: str, path: str, timestamp: int,
         body: bytes) -> str:
    """Sign as an agent would. Used by tests and by the enrollment helper."""
    key = Ed25519PrivateKey.from_private_bytes(base64.b64decode(private_key_b64))
    sig = key.sign(canonical_string(method, path, timestamp, body))
    return f"{SCHEME}={base64.b64encode(sig).decode()}"


def verify(public_key_b64: str, signature: str, method: str, path: str,
           timestamp: int, body: bytes) -> bool:
    scheme, _, raw = signature.partition("=")
    if scheme != SCHEME or not raw:
        return False
    try:
        key = Ed25519PublicKey.from_public_bytes(base64.b64decode(public_key_b64))
        key.verify(base64.b64decode(raw), canonical_string(method, path, timestamp, body))
        return True
    except (InvalidSignature, ValueError):
        # ValueError covers malformed base64 and wrong-length keys. Both are
        # "this request is not authentic", and neither deserves a distinct
        # error message to whoever sent it.
        return False


def generate_keypair() -> tuple[str, str]:
    """Return (private_b64, public_b64). Used by tests; agents do this in Go."""
    private = Ed25519PrivateKey.generate()
    from cryptography.hazmat.primitives.serialization import (
        Encoding, NoEncryption, PrivateFormat, PublicFormat,
    )
    priv = private.private_bytes(Encoding.Raw, PrivateFormat.Raw, NoEncryption())
    pub = private.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw)
    return base64.b64encode(priv).decode(), base64.b64encode(pub).decode()


def hash_token(token: str) -> str:
    """Enrollment tokens are stored hashed.

    Plain SHA-256 rather than argon2: these are 256 bits of
    ``secrets.token_urlsafe`` output, not user-chosen passwords. There is no
    dictionary to attack, so a slow KDF would buy nothing.
    """
    return hashlib.sha256(token.encode()).hexdigest()
