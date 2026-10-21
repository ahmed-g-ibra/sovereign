-- Path measurement.
--
-- Latency says a route is slow. The path says why, and -- more valuable -- it
-- says when the route changed. A peering change or a new region coming online
-- shows up here as a different AS path days before anyone announces anything.
--
-- Volume is modest: one trace per target per hour, roughly 15 hops each. That
-- is ~4,000 hop rows a day for 11 targets, so no partitioning is warranted.

CREATE TABLE traceroutes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    probe_id    uuid        NOT NULL REFERENCES probes(id) ON DELETE CASCADE,
    target_id   text        NOT NULL REFERENCES targets(id) ON DELETE RESTRICT,
    ts          timestamptz NOT NULL,

    dest_ip     inet,
    -- Complete means we reached the destination rather than exhausting TTL.
    complete    boolean     NOT NULL DEFAULT false,
    hop_count   integer     NOT NULL DEFAULT 0,
    -- How many hops actually answered. Routers often decline to send Time
    -- Exceeded, so a path with silent hops is normal, not broken.
    answered    integer     NOT NULL DEFAULT 0,

    -- The autonomous systems crossed, in order, with consecutive duplicates
    -- collapsed. This is the comparable part of a path: hop addresses change
    -- with load balancing while the sequence of networks stays put.
    as_path     integer[]   NOT NULL DEFAULT '{}',
    as_path_hash text       NOT NULL,

    received_at timestamptz NOT NULL DEFAULT now(),

    -- Same idempotency contract as samples: replay costs nothing.
    UNIQUE (probe_id, target_id, ts)
);

CREATE INDEX traceroutes_target_ts_idx ON traceroutes (target_id, ts DESC);
CREATE INDEX traceroutes_path_idx ON traceroutes (probe_id, target_id, ts DESC);

CREATE TABLE traceroute_hops (
    traceroute_id uuid     NOT NULL REFERENCES traceroutes(id) ON DELETE CASCADE,
    ttl           smallint NOT NULL,
    -- NULL means nothing answered at this TTL, which is recorded rather than
    -- skipped: the position in the path is itself information.
    hop_ip        inet,
    rtt_ms        real,
    asn           integer,
    as_name       text,
    final         boolean  NOT NULL DEFAULT false,

    PRIMARY KEY (traceroute_id, ttl)
);

CREATE INDEX traceroute_hops_asn_idx ON traceroute_hops (asn) WHERE asn IS NOT NULL;

-- Every point at which the AS path to a target changed, for a given probe.
--
-- Expressed as a view rather than computed at write time so that a corrected
-- or late-arriving trace is reflected automatically.
CREATE OR REPLACE VIEW path_changes AS
SELECT *
FROM (
    SELECT
        t.probe_id,
        t.target_id,
        t.ts,
        t.as_path,
        t.as_path_hash,
        lag(t.as_path_hash) OVER w AS previous_hash,
        lag(t.as_path)      OVER w AS previous_path,
        lag(t.ts)           OVER w AS previous_ts
    FROM traceroutes t
    WHERE t.as_path <> '{}'
    WINDOW w AS (PARTITION BY t.probe_id, t.target_id ORDER BY t.ts)
) x
WHERE previous_hash IS NOT NULL
  AND previous_hash IS DISTINCT FROM as_path_hash;
