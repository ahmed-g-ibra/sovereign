-- Rollups.
--
-- Raw samples are deleted after 30 days. These tables are what the site reads
-- and what makes the project worth more every month it runs.
--
-- Both resolutions are computed from RAW samples, never from each other:
-- a percentile of percentiles is not a percentile. Averaging twelve p95 values
-- produces a number that describes nothing, and it is the single most common
-- way monitoring dashboards end up lying.

CREATE TABLE sample_rollup_5m (
    probe_id   uuid        NOT NULL REFERENCES probes(id) ON DELETE CASCADE,
    target_id  text        NOT NULL REFERENCES targets(id) ON DELETE RESTRICT,
    bucket     timestamptz NOT NULL,

    samples    integer     NOT NULL,
    failures   integer     NOT NULL,
    loss_pct   real        NOT NULL,

    min_ms     real,
    p50_ms     real,
    p90_ms     real,
    p95_ms     real,
    p99_ms     real,
    max_ms     real,

    computed_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (probe_id, target_id, bucket)
);

CREATE INDEX sample_rollup_5m_target_bucket_idx
    ON sample_rollup_5m (target_id, bucket DESC);

CREATE TABLE sample_rollup_1h (
    LIKE sample_rollup_5m INCLUDING ALL
);

ALTER TABLE sample_rollup_1h
    ADD CONSTRAINT sample_rollup_1h_probe_fk
        FOREIGN KEY (probe_id) REFERENCES probes(id) ON DELETE CASCADE,
    ADD CONSTRAINT sample_rollup_1h_target_fk
        FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE RESTRICT;

-- Recompute every bucket that overlaps [from_ts, to_ts). Idempotent by design:
-- a late batch from a probe that buffered through an outage must correct the
-- bucket it belongs to, not append a second one.
CREATE OR REPLACE FUNCTION roll_up(resolution interval,
                                   from_ts timestamptz,
                                   to_ts timestamptz)
RETURNS integer LANGUAGE plpgsql AS $$
DECLARE
    target_table text;
    affected     integer;
BEGIN
    IF resolution = interval '5 minutes' THEN
        target_table := 'sample_rollup_5m';
    ELSIF resolution = interval '1 hour' THEN
        target_table := 'sample_rollup_1h';
    ELSE
        RAISE EXCEPTION 'unsupported rollup resolution: %', resolution;
    END IF;

    EXECUTE format($q$
        INSERT INTO %I AS r (probe_id, target_id, bucket, samples, failures,
                             loss_pct, min_ms, p50_ms, p90_ms, p95_ms, p99_ms,
                             max_ms, computed_at)
        SELECT
            probe_id,
            target_id,
            to_timestamp(floor(extract(epoch FROM ts) / %L) * %L) AS bucket,
            count(*)::int,
            count(*) FILTER (WHERE error IS NOT NULL)::int,
            (count(*) FILTER (WHERE error IS NOT NULL)::real / count(*)) * 100,
            -- Percentiles over successful probes only. A failed measurement
            -- counts toward loss and never toward timings.
            min(total_ms) FILTER (WHERE error IS NULL),
            percentile_cont(0.50) WITHIN GROUP (ORDER BY total_ms)
                FILTER (WHERE error IS NULL),
            percentile_cont(0.90) WITHIN GROUP (ORDER BY total_ms)
                FILTER (WHERE error IS NULL),
            percentile_cont(0.95) WITHIN GROUP (ORDER BY total_ms)
                FILTER (WHERE error IS NULL),
            percentile_cont(0.99) WITHIN GROUP (ORDER BY total_ms)
                FILTER (WHERE error IS NULL),
            max(total_ms) FILTER (WHERE error IS NULL),
            now()
        FROM samples
        WHERE ts >= %L AND ts < %L
        GROUP BY probe_id, target_id, 3
        ON CONFLICT (probe_id, target_id, bucket) DO UPDATE SET
            samples     = EXCLUDED.samples,
            failures    = EXCLUDED.failures,
            loss_pct    = EXCLUDED.loss_pct,
            min_ms      = EXCLUDED.min_ms,
            p50_ms      = EXCLUDED.p50_ms,
            p90_ms      = EXCLUDED.p90_ms,
            p95_ms      = EXCLUDED.p95_ms,
            p99_ms      = EXCLUDED.p99_ms,
            max_ms      = EXCLUDED.max_ms,
            computed_at = now()
    $q$, target_table,
         extract(epoch FROM resolution), extract(epoch FROM resolution),
         from_ts, to_ts);

    GET DIAGNOSTICS affected = ROW_COUNT;
    RETURN affected;
END;
$$;

-- What the public site reads: hourly data, quarantined probes excluded,
-- thin buckets excluded.
--
-- Methodology rules 2, 4 and 6 are enforced here rather than in application
-- code, so no future endpoint can forget them.
CREATE OR REPLACE VIEW published_rollup_1h AS
SELECT
    r.target_id,
    t.provider,
    t.region,
    t.country    AS target_country,
    p.country    AS probe_country,
    p.asn,
    r.bucket,
    r.samples,
    r.loss_pct,
    r.p50_ms,
    r.p90_ms,
    r.p95_ms,
    r.p99_ms
FROM sample_rollup_1h r
JOIN probes  p ON p.id = r.probe_id
JOIN targets t ON t.id = r.target_id
WHERE p.quarantined IS FALSE
  AND p.revoked_at IS NULL
  AND r.samples >= 10;
