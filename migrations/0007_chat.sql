CREATE SCHEMA IF NOT EXISTS chat;

CREATE TABLE IF NOT EXISTS chat.projects (
    id TEXT PRIMARY KEY DEFAULT goddard.ulid(),
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT
);

CREATE TABLE IF NOT EXISTS chat.conversations (
    id TEXT PRIMARY KEY DEFAULT goddard.ulid(),
    project_id TEXT NOT NULL REFERENCES chat.projects (id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'web' CHECK (source IN ('web', 'telegram', 'slack', 'schedule')),
    created_by TEXT NOT NULL,
    claimed_until BIGINT,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT
);

CREATE INDEX IF NOT EXISTS conversations_project ON chat.conversations (project_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS chat.events (
    id TEXT PRIMARY KEY DEFAULT goddard.ulid(),
    conversation_id TEXT NOT NULL REFERENCES chat.conversations (id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    at BIGINT NOT NULL,
    event JSONB NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    UNIQUE (conversation_id, seq)
);

CREATE TABLE IF NOT EXISTS chat.uploads (
    id TEXT PRIMARY KEY DEFAULT goddard.ulid(),
    conversation_id TEXT NOT NULL REFERENCES chat.conversations (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    mime TEXT NOT NULL,
    bytes BYTEA NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT
);
