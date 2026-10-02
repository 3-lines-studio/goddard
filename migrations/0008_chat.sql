-- The threads: a project of an owner, the conversations in it, the events of
-- each conversation and the files that travel with a message. The slug of a
-- project is its directory in the workspace and never moves, not even after a
-- rename, which is why it is not the name.
CREATE SCHEMA IF NOT EXISTS chat;

CREATE TABLE IF NOT EXISTS chat.projects (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    CONSTRAINT projects_natural UNIQUE (slug)
);

CREATE TABLE IF NOT EXISTS chat.conversations (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    project_id TEXT NOT NULL REFERENCES chat.projects (id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'web' CHECK (source IN ('web', 'telegram', 'slack', 'schedule')),
    created_by TEXT NOT NULL,
    claimed_until BIGINT,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS conversations_project ON chat.conversations (project_id, updated_at DESC);

-- The body of an event is opaque to the store: the dialect is the app's.
CREATE TABLE IF NOT EXISTS chat.events (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    conversation_id TEXT NOT NULL REFERENCES chat.conversations (id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    at BIGINT NOT NULL,
    event JSONB NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    UNIQUE (conversation_id, seq)
);

-- The attachments live in the database and not in a directory, because any
-- instance has to serve any conversation.
CREATE TABLE IF NOT EXISTS chat.uploads (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    conversation_id TEXT NOT NULL REFERENCES chat.conversations (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    mime TEXT NOT NULL,
    bytes BYTEA NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id)
);
