CREATE TABLE IF NOT EXISTS zone_state (
    zone_id     TEXT NOT NULL,
    runtime_id  TEXT NOT NULL REFERENCES runtimes(id) ON DELETE CASCADE,
    snapshot    JSONB NOT NULL,
    tick_count  BIGINT NOT NULL,
    taken_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (zone_id, taken_at)
);
CREATE INDEX IF NOT EXISTS idx_zone_state_runtime ON zone_state(runtime_id, taken_at DESC);
