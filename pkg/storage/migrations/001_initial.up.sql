CREATE TABLE IF NOT EXISTS game_servers (
    id TEXT PRIMARY KEY,
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'joining',
    max_zones INTEGER NOT NULL DEFAULT 10,
    metadata JSONB DEFAULT '{}',
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS runtimes (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'creating',
    zone_count INTEGER NOT NULL DEFAULT 1,
    zone_size DOUBLE PRECISION NOT NULL DEFAULT 100,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    destroyed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS zones (
    id TEXT PRIMARY KEY,
    runtime_id TEXT NOT NULL REFERENCES runtimes(id),
    grid_x INTEGER NOT NULL,
    grid_y INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'unowned',
    server_id TEXT REFERENCES game_servers(id),
    size DOUBLE PRECISION NOT NULL DEFAULT 100,
    UNIQUE(runtime_id, grid_x, grid_y)
);

CREATE INDEX idx_zones_runtime ON zones(runtime_id);
CREATE INDEX idx_zones_server ON zones(server_id);
CREATE INDEX idx_game_servers_status ON game_servers(status);
