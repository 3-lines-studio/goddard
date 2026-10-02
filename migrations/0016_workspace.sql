CREATE SCHEMA IF NOT EXISTS workspace;

CREATE TABLE IF NOT EXISTS workspace.workspaces (
    id TEXT PRIMARY KEY DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    path TEXT NOT NULL,
    sandbox_kind TEXT NOT NULL DEFAULT 'ssh',
    sandbox_addr TEXT NOT NULL DEFAULT '',
    sandbox_user TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    CONSTRAINT workspaces_natural UNIQUE (owner_kind, owner_id)
);
