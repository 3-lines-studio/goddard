CREATE SCHEMA IF NOT EXISTS heimdall;

CREATE TABLE IF NOT EXISTS heimdall.secrets (
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    name TEXT NOT NULL,
    value BYTEA NOT NULL,
    updated_at BIGINT NOT NULL,
    PRIMARY KEY (project, env, name)
);

CREATE TABLE IF NOT EXISTS heimdall.tokens (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    keys TEXT,
    hash TEXT NOT NULL,
    admin BOOLEAN NOT NULL DEFAULT false,
    created_at BIGINT NOT NULL,
    expires_at BIGINT,
    last_used BIGINT
);

CREATE INDEX IF NOT EXISTS tokens_hash ON heimdall.tokens (hash);

CREATE TABLE IF NOT EXISTS heimdall.audit (
    id BIGSERIAL PRIMARY KEY,
    at BIGINT NOT NULL,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    name TEXT
);

CREATE TABLE IF NOT EXISTS heimdall.environments (
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (project, env)
);

CREATE TABLE IF NOT EXISTS heimdall.logins (
    hash TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS heimdall.sessions (
    hash TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL
);
