-- The transcript of the harness. A session is one scope — a conversation — and
-- there is only one live session per scope: the partial index is what says so.
CREATE SCHEMA IF NOT EXISTS axe;

CREATE TABLE IF NOT EXISTS axe.sessions (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    scope TEXT NOT NULL,
    title TEXT NOT NULL,
    turns INTEGER NOT NULL,
    seq BIGSERIAL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL,
    deleted_at BIGINT,
    PRIMARY KEY (id)
);

CREATE UNIQUE INDEX IF NOT EXISTS sessions_live ON axe.sessions (scope) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS sessions_scope ON axe.sessions (scope, updated_at DESC, seq DESC);

CREATE TABLE IF NOT EXISTS axe.entries (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    session_id TEXT NOT NULL REFERENCES axe.sessions (id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    entry JSONB NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    CONSTRAINT entries_natural UNIQUE (session_id, seq)
);

CREATE TABLE IF NOT EXISTS axe.resume (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    scope TEXT NOT NULL,
    session_id TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    CONSTRAINT resume_natural UNIQUE (scope)
);
