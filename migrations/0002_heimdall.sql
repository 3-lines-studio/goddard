-- The vault. A secret belongs to an owner (a person or an organization), a
-- project and an environment, and the value is sealed by the app before it gets
-- here: the database never holds it in the clear.
CREATE SCHEMA IF NOT EXISTS heimdall;

CREATE TABLE IF NOT EXISTS heimdall.secrets (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    name TEXT NOT NULL,
    value BYTEA NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL,
    deleted_at BIGINT,
    PRIMARY KEY (owner_kind, owner_id, project, env, name),
    CONSTRAINT secrets_owner_kind CHECK (owner_kind IN ('org', 'user'))
);

CREATE TABLE IF NOT EXISTS heimdall.environments (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (owner_kind, owner_id, project, env),
    CONSTRAINT environments_owner_kind CHECK (owner_kind IN ('org', 'user'))
);

CREATE TABLE IF NOT EXISTS heimdall.tokens (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    keys JSONB,
    hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'agent' CHECK (role IN ('agent', 'admin')),
    created_at BIGINT NOT NULL,
    expires_at BIGINT,
    last_used BIGINT,
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    CONSTRAINT tokens_owner_kind CHECK (owner_kind IN ('org', 'user'))
);

CREATE INDEX IF NOT EXISTS tokens_hash ON heimdall.tokens (hash);

-- The audit is read newest first and a ULID does not keep the order inside the
-- same millisecond, so the sequence is what orders it.
CREATE TABLE IF NOT EXISTS heimdall.audit (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    at BIGINT NOT NULL,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    name TEXT,
    seq BIGINT GENERATED ALWAYS AS IDENTITY,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id)
);
