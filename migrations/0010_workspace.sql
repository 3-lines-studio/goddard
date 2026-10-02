-- Where the projects of an owner live: the volume its sandbox mounts. It is the
-- only thing goddard knows of that machine. Where the machine is, who to be
-- there and the key to get in are three secrets of the owner in heimdall, under
-- the project `sandbox`; keeping them in a row too would be two places for one
-- string.
CREATE SCHEMA IF NOT EXISTS workspace;

CREATE TABLE IF NOT EXISTS workspace.workspaces (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    path TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    CONSTRAINT workspaces_natural UNIQUE (owner_kind, owner_id)
);
