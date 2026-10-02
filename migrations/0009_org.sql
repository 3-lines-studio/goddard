-- Who shares what. An organization owns projects, and its people are in it with
-- a role.
CREATE SCHEMA IF NOT EXISTS org;

CREATE TABLE IF NOT EXISTS org.orgs (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    CONSTRAINT orgs_natural UNIQUE (slug)
);

CREATE TABLE IF NOT EXISTS org.members (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    org_id TEXT NOT NULL REFERENCES org.orgs (id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    CONSTRAINT members_natural UNIQUE (org_id, user_id)
);

CREATE INDEX IF NOT EXISTS members_user ON org.members (user_id);
