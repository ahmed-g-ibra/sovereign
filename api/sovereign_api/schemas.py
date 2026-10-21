"""Request and response models."""

from __future__ import annotations

import datetime as dt

from pydantic import BaseModel, Field, model_validator


class RegisterRequest(BaseModel):
    enrollment_token: str = Field(min_length=16)
    public_key: str = Field(min_length=10, description="Ed25519 public key, base64")
    name: str = Field(min_length=1, max_length=120)
    country: str = Field(min_length=2, max_length=2, description="ISO 3166-1 alpha-2")
    city: str | None = None
    isp_claimed: str | None = None
    tier: str = "community"


class RegisterResponse(BaseModel):
    probe_id: str
    config_ttl_s: int


class TargetOut(BaseModel):
    id: str
    provider: str
    region: str
    country: str
    city: str | None
    host: str
    port: int
    is_control: bool


class HopIn(BaseModel):
    ttl: int = Field(ge=1, le=255)
    ip: str | None = None
    rtt_ms: float | None = None
    asn: int | None = None
    as_name: str | None = None
    final: bool = False
    timeout: bool = False


class TraceIn(BaseModel):
    target_id: str
    ts: dt.datetime
    dest_ip: str | None = None
    host: str | None = None
    complete: bool = False
    hops: list[HopIn] = Field(default_factory=list, max_length=255)

    @model_validator(mode="after")
    def hops_have_distinct_ttls(self) -> "TraceIn":
        ttls = [h.ttl for h in self.hops]
        if len(set(ttls)) != len(ttls):
            raise ValueError("a trace cannot have two hops at the same TTL")
        return self


class TraceBatchIn(BaseModel):
    sent_at: dt.datetime
    traces: list[TraceIn] = Field(default_factory=list, max_length=500)


class ConfigOut(BaseModel):
    """What the control plane tells an agent to do.

    The agent holds no opinion about what to measure; it asks, caches for
    config_ttl_s, and asks again. A region launch is an UPDATE on one table.
    """

    interval_s: int
    timeout_s: int
    config_ttl_s: int
    # Much longer than interval_s: a path changes on the order of weeks, and
    # every trace puts ICMP at every router along the way.
    traceroute_interval_s: int
    targets: list[TargetOut]


class SampleIn(BaseModel):
    target_id: str
    ts: dt.datetime
    dns_ms: float | None = None
    connect_ms: float | None = None
    tls_ms: float | None = None
    total_ms: float | None = None
    remote_ip: str | None = None
    error: str | None = None
    host_load1: float | None = None

    @model_validator(mode="after")
    def failed_probes_carry_no_timings(self) -> "SampleIn":
        # Mirrors the CHECK constraint in the database. Enforced twice on
        # purpose: the constraint is the guarantee, this is the clear error
        # message that stops an agent bug from being discovered as a 500.
        if self.error is not None and self.total_ms is not None:
            raise ValueError("a failed sample must not carry a total_ms")
        return self


class BatchIn(BaseModel):
    sent_at: dt.datetime
    samples: list[SampleIn]


class BatchResponse(BaseModel):
    accepted: int
    duplicates: int
    quarantined: bool
    clock_skew_ms: int
