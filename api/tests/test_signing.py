"""Signing scheme tests, including the cross-language fixture.

The vector below is also asserted by agent/internal/client/sign_test.go. If
someone changes the canonical string on one side only, exactly one of those
two tests fails, which is the entire point of duplicating it.
"""

from __future__ import annotations

import base64

from sovereign_api import signing

# A deterministic key: 32 bytes of 0x01. Never used for anything real.
FIXTURE_PRIVATE_KEY = base64.b64encode(bytes([1] * 32)).decode()
FIXTURE_TIMESTAMP = 1761000000
FIXTURE_BODY = b'{"hello":"gulf"}'
FIXTURE_PATH = "/v1/samples"
FIXTURE_SIGNATURE = "v1=0wNien+Fk3IJTqkQYdRioglwjDdtftdZqab+leID/sMgk+tYj6kYwaZXMt/YP/GWqiE9lwlByykDxjwgN/EOCQ=="


def test_canonical_string_shape():
    canonical = signing.canonical_string("post", FIXTURE_PATH, FIXTURE_TIMESTAMP,
                                         FIXTURE_BODY)
    lines = canonical.decode().split("\n")
    assert lines[0] == "v1"
    assert lines[1] == "POST", "method must be upper-cased"
    assert lines[2] == FIXTURE_PATH
    assert lines[3] == str(FIXTURE_TIMESTAMP)
    assert lines[4] == signing.body_digest(FIXTURE_BODY)
    assert len(lines) == 5


def test_sign_verify_roundtrip():
    private, public = signing.generate_keypair()
    sig = signing.sign(private, "POST", FIXTURE_PATH, FIXTURE_TIMESTAMP, FIXTURE_BODY)
    assert sig.startswith("v1=")
    assert signing.verify(public, sig, "POST", FIXTURE_PATH, FIXTURE_TIMESTAMP,
                          FIXTURE_BODY)


def test_every_signed_field_is_load_bearing():
    private, public = signing.generate_keypair()
    sig = signing.sign(private, "POST", FIXTURE_PATH, FIXTURE_TIMESTAMP, FIXTURE_BODY)

    # Change any one element and the signature must stop verifying.
    assert not signing.verify(public, sig, "GET", FIXTURE_PATH, FIXTURE_TIMESTAMP,
                              FIXTURE_BODY)
    assert not signing.verify(public, sig, "POST", "/v1/config", FIXTURE_TIMESTAMP,
                              FIXTURE_BODY)
    assert not signing.verify(public, sig, "POST", FIXTURE_PATH,
                              FIXTURE_TIMESTAMP + 1, FIXTURE_BODY)
    assert not signing.verify(public, sig, "POST", FIXTURE_PATH, FIXTURE_TIMESTAMP,
                              FIXTURE_BODY + b" ")


def test_wrong_key_does_not_verify():
    private, _ = signing.generate_keypair()
    _, other_public = signing.generate_keypair()
    sig = signing.sign(private, "POST", FIXTURE_PATH, FIXTURE_TIMESTAMP, FIXTURE_BODY)
    assert not signing.verify(other_public, sig, "POST", FIXTURE_PATH,
                              FIXTURE_TIMESTAMP, FIXTURE_BODY)


def test_malformed_signatures_are_rejected_not_raised():
    _, public = signing.generate_keypair()
    for bad in ["", "v1=", "garbage", "v2=abc", "v1=!!!notbase64!!!",
                "v1=" + base64.b64encode(b"short").decode()]:
        assert signing.verify(public, bad, "POST", FIXTURE_PATH,
                              FIXTURE_TIMESTAMP, FIXTURE_BODY) is False


def test_cross_language_fixture():
    """Known-answer test shared with the Go agent."""
    sig = signing.sign(FIXTURE_PRIVATE_KEY, "POST", FIXTURE_PATH,
                       FIXTURE_TIMESTAMP, FIXTURE_BODY)
    # Printed by `pytest -s -k fixture` when it needs regenerating.
    print("\nFIXTURE SIGNATURE:", sig)
    assert sig == FIXTURE_SIGNATURE

