CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS memory_events (
    id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(128) NOT NULL,
    role VARCHAR(64) NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_memory_events_session_id ON memory_events(session_id);
CREATE INDEX IF NOT EXISTS idx_memory_events_role ON memory_events(role);
CREATE INDEX IF NOT EXISTS idx_memory_events_created_at ON memory_events(created_at);
