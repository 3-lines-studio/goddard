CREATE SCHEMA IF NOT EXISTS org;

CREATE TABLE IF NOT EXISTS org.orgs (
    id TEXT PRIMARY KEY DEFAULT goddard.ulid(),
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    CONSTRAINT orgs_natural UNIQUE (slug)
);

CREATE TABLE IF NOT EXISTS org.members (
    id TEXT PRIMARY KEY DEFAULT goddard.ulid(),
    org_id TEXT NOT NULL REFERENCES org.orgs (id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    CONSTRAINT members_natural UNIQUE (org_id, user_id)
);

CREATE INDEX IF NOT EXISTS members_user ON org.members (user_id);
