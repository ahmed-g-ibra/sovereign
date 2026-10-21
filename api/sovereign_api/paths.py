"""AS path derivation.

A traceroute's hop addresses are noisy: load balancers rotate them, and some
routers never answer. The sequence of *networks* is the stable, comparable
thing, and it is what tells you a route changed.
"""

from __future__ import annotations

import hashlib
from typing import Iterable


def as_path(asns: Iterable[int | None]) -> list[int]:
    """Collapse hop ASNs into the ordered sequence of networks crossed.

    Consecutive duplicates collapse because a single network contributing six
    hops is still one network in the path. Silent hops and unresolved
    addresses drop out: they carry no network identity, and leaving a gap
    marker in would make two identical paths compare unequal whenever a router
    happened not to answer.
    """
    out: list[int] = []
    for asn in asns:
        if not asn:
            continue
        if out and out[-1] == asn:
            continue
        out.append(asn)
    return out


def path_hash(path: list[int]) -> str:
    """A short, stable identifier for a path, for change detection.

    Hashing rather than comparing arrays keeps the index small and the
    comparison trivial in SQL. Truncated to 16 hex characters: this detects
    change, it does not defend against an adversary choosing collisions.
    """
    joined = ">".join(str(a) for a in path)
    return hashlib.sha256(joined.encode()).hexdigest()[:16]
