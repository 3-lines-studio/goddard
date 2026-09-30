CREATE SCHEMA IF NOT EXISTS axe;

CREATE TABLE IF NOT EXISTS axe.sessions (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    title TEXT NOT NULL,
    turns INTEGER NOT NULL,
    updated_at BIGINT NOT NULL,
    archived_at BIGINT,
    seq BIGSERIAL
);

CREATE UNIQUE INDEX IF NOT EXISTS sessions_live ON axe.sessions (scope) WHERE archived_at IS NULL;

CREATE INDEX IF NOT EXISTS sessions_scope ON axe.sessions (scope, updated_at DESC, seq DESC);

CREATE TABLE IF NOT EXISTS axe.entries (
    session_id TEXT NOT NULL REFERENCES axe.sessions (id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    entry JSONB NOT NULL,
    PRIMARY KEY (session_id, seq)
);

CREATE TABLE IF NOT EXISTS axe.resume (
    scope TEXT PRIMARY KEY,
    session_id TEXT NOT NULL
);
