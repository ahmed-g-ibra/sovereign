-- Sovereign core schema.
--
-- Three ideas drive the shape of this:
--   1. A probe is an identity with a secret, not a trusted caller.
--   2. Samples are append-only and idempotent, because an agent that buffered
--      through an outage will replay batches it is not sure landed.
--   3. Raw samples are transient; rollups are the asset. Retention differs by
--      an order of magnitude, so they are different tables.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Enrollment
-- ---------------------------------------------------------------------------

-- A probe cannot enrol itself out of nowhere. An operator issues a token, the
-- agent spends it once, and gets an identity in return.
CREATE TABLE enrollment_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash  text        NOT NULL UNIQUE,
    note        text,
    max_uses    integer     NOT NULL DEFAULT 1 CHECK (max_uses > 0),
    uses        integer     NOT NULL DEFAULT 0 CHECK (uses >= 0),
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz
);

CREATE INDEX enrollment_tokens_live_idx
    ON enrollment_tokens (expires_at)
    WHERE revoked_at IS NULL;

-- ---------------------------------------------------------------------------
-- Probes
-- ---------------------------------------------------------------------------

CREATE TABLE probes (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                text        NOT NULL,
    -- Vantage point description. ASN is what aggregates should group by:
    -- a volunteer's claim about which ISP they are on is a hint, the observed
    -- AS number is evidence.
    country             text        NOT NULL,
    city                text,
    isp_claimed         text,
    asn                 integer,
    observed_ip         inet,

    -- Ed25519 public key, base64. The server never holds anything that could
    -- forge a sample: an attacker with a full database dump can read every
    -- measurement but cannot write one.
    public_key          text        NOT NULL UNIQUE,
    tier                text        NOT NULL DEFAULT 'community'
                        CHECK (tier IN ('core', 'vps', 'cloud', 'community')),

    enrolled_at         timestamptz NOT NULL DEFAULT now(),
    last_seen_at        timestamptz,
    last_host_load1     real,
    last_clock_skew_ms  integer,

    -- Methodology rule 4: a probe whose clock cannot be trusted is excluded
    -- from published aggregates until an operator clears it. The data is kept,
    -- never deleted, because the quarantine itself may turn out to be wrong.
    quarantined         boolean     NOT NULL DEFAULT false,
    quarantine_reason   text,
    revoked_at          timestamptz
);

CREATE INDEX probes_active_idx ON probes (last_seen_at DESC)
    WHERE revoked_at IS NULL;

-- ---------------------------------------------------------------------------
-- Targets: the control plane's copy of the target list
-- ---------------------------------------------------------------------------

-- The agent does not decide what to measure. It asks. That is the whole
-- difference between a script and a fleet: a region launch is an UPDATE here,
-- not a redeploy of every probe.
CREATE TABLE targets (
    id          text PRIMARY KEY,
    provider    text        NOT NULL,
    region      text        NOT NULL,
    country     text        NOT NULL,
    city        text,
    host        text        NOT NULL,
    port        integer     NOT NULL DEFAULT 443 CHECK (port BETWEEN 1 AND 65535),
    status      text        NOT NULL DEFAULT 'verify'
                CHECK (status IN ('live', 'verify', 'announced', 'rejected')),
    is_control  boolean     NOT NULL DEFAULT false,
    note        text,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX targets_live_idx ON targets (status) WHERE status = 'live';

-- ---------------------------------------------------------------------------
-- Samples
-- ---------------------------------------------------------------------------

-- Partitioned by day. Raw samples live 30 days; the rollups below live forever.
-- The primary key includes ts because a partitioned table's unique constraint
-- must contain the partition key, and it is also exactly the idempotency key
-- we want: one measurement per probe, per target, per instant.
CREATE TABLE samples (
    probe_id    uuid        NOT NULL REFERENCES probes(id) ON DELETE CASCADE,
    target_id   text        NOT NULL REFERENCES targets(id) ON DELETE RESTRICT,
    ts          timestamptz NOT NULL,

    dns_ms      real,
    connect_ms  real,
    tls_ms      real,
    total_ms    real,
    remote_ip   inet,
    error       text,

    -- Methodology rule 5: host load travels with the sample so a busy probe's
    -- data can be discarded after the fact rather than silently believed.
    host_load1  real,

    received_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (probe_id, target_id, ts)
) PARTITION BY RANGE (ts);

-- Reads are nearly always "this target, recent window, across probes".
CREATE INDEX samples_target_ts_idx ON samples (target_id, ts DESC);

-- A sample either succeeded or carries an error. Timings on a failed probe are
-- meaningless and must never be stored as if they were real.
ALTER TABLE samples ADD CONSTRAINT samples_failed_has_no_timings
    CHECK (error IS NULL OR total_ms IS NULL);

-- Creates daily partitions from (today - back) to (today + ahead). Idempotent,
-- so it is safe to call on every API start and from a scheduled job.
CREATE OR REPLACE FUNCTION ensure_sample_partitions(back integer DEFAULT 1,
                                                    ahead integer DEFAULT 7)
RETURNS integer LANGUAGE plpgsql AS $$
DECLARE
    d       date;
    created integer := 0;
    name    text;
BEGIN
    FOR d IN
        SELECT generate_series(current_date - back, current_date + ahead, '1 day')::date
    LOOP
        name := format('samples_%s', to_char(d, 'YYYYMMDD'));
        IF to_regclass(format('public.%I', name)) IS NULL THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF samples FOR VALUES FROM (%L) TO (%L)',
                name, d, d + 1);
            created := created + 1;
        END IF;
    END LOOP;
    RETURN created;
END;
$$;

-- Drops partitions wholly older than the retention window. Dropping a
-- partition is instant; DELETE across 16M rows is not. This is the entire
-- reason the table is partitioned.
CREATE OR REPLACE FUNCTION drop_old_sample_partitions(keep_days integer DEFAULT 30)
RETURNS integer LANGUAGE plpgsql AS $$
DECLARE
    r       record;
    dropped integer := 0;
    cutoff  date := current_date - keep_days;
BEGIN
    FOR r IN
        SELECT c.relname
        FROM pg_class c
        JOIN pg_inherits i ON i.inhrelid = c.oid
        JOIN pg_class p ON p.oid = i.inhparent
        WHERE p.relname = 'samples'
          AND c.relname ~ '^samples_[0-9]{8}$'
          AND to_date(right(c.relname, 8), 'YYYYMMDD') < cutoff
    LOOP
        EXECUTE format('DROP TABLE %I', r.relname);
        dropped := dropped + 1;
    END LOOP;
    RETURN dropped;
END;
$$;

SELECT ensure_sample_partitions(1, 7);
