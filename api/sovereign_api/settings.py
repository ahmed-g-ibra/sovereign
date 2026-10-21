"""Configuration, read once from the environment."""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    database_url: str

    # How far a request's timestamp may be from ours before we reject it.
    # This is replay protection, not clock measurement.
    signature_window_s: int = 300

    # Server-side skew check. This is deliberately looser than the 500 ms in
    # the methodology because the measurement here includes network transit:
    # it is a conservative upper bound on skew, so it only ever quarantines a
    # probe that is genuinely far out. True skew measurement belongs in the
    # agent, against NTP, and is not built yet.
    clock_skew_quarantine_ms: int = 2000

    # Refuse absurd batches before touching the database.
    max_batch_samples: int = 5000

    # How long an agent may cache its config before asking again.
    config_ttl_s: int = 900

    @classmethod
    def from_env(cls) -> "Settings":
        url = os.environ.get("DATABASE_URL")
        if not url:
            raise RuntimeError("DATABASE_URL is not set")
        return cls(
            database_url=url,
            signature_window_s=int(os.environ.get("SIGNATURE_WINDOW_S", 300)),
            clock_skew_quarantine_ms=int(os.environ.get("CLOCK_SKEW_QUARANTINE_MS", 2000)),
            max_batch_samples=int(os.environ.get("MAX_BATCH_SAMPLES", 5000)),
            config_ttl_s=int(os.environ.get("CONFIG_TTL_S", 900)),
        )
