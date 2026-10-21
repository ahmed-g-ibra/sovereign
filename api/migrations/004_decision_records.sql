-- Decision records.
--
-- The advisor's output is the artifact an architect pastes into a design
-- review, so it needs to be stable and citable: the same URL must show the
-- same answer next month, even after the latency data and the rules have moved
-- on. Storing the rendered record rather than recomputing it is the point.

CREATE TABLE decision_records (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at  timestamptz NOT NULL DEFAULT now(),
    inputs      jsonb       NOT NULL,
    record      jsonb       NOT NULL,
    -- Status of the knowledge base at the time. A record produced from draft
    -- entries must keep saying so for as long as it exists.
    kb_status   text        NOT NULL
);

CREATE INDEX decision_records_created_idx ON decision_records (created_at DESC);
