CREATE SCHEMA IF NOT EXISTS goddard;

CREATE OR REPLACE FUNCTION goddard.now() RETURNS BIGINT
LANGUAGE sql STABLE AS $$
    SELECT floor(EXTRACT(EPOCH FROM now()))::BIGINT
$$;

CREATE OR REPLACE FUNCTION goddard.ulid() RETURNS TEXT
LANGUAGE plpgsql VOLATILE AS $$
DECLARE
    alphabet CONSTANT TEXT := '0123456789ABCDEFGHJKMNPQRSTVWXYZ';
    millis CONSTANT BIGINT := floor(EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::BIGINT;
    bytes CONSTANT BYTEA := uuid_send(gen_random_uuid());
    id TEXT := '';
    i INTEGER;
BEGIN
    FOR i IN REVERSE 9..0 LOOP
        id := id || substr(alphabet, ((millis >> (i * 5)) % 32)::INTEGER + 1, 1);
    END LOOP;
    FOR i IN 0..15 LOOP
        id := id || substr(alphabet, (get_byte(bytes, i) % 32) + 1, 1);
    END LOOP;
    RETURN id;
END;
$$;

CREATE SCHEMA IF NOT EXISTS auth;

CREATE TABLE IF NOT EXISTS auth.users (
    id TEXT PRIMARY KEY DEFAULT goddard.ulid(),
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT
);

CREATE TABLE IF NOT EXISTS auth.logins (
    id TEXT PRIMARY KEY DEFAULT goddard.ulid(),
    user_id TEXT NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
    hash TEXT NOT NULL,
    expires_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT
);

CREATE INDEX IF NOT EXISTS logins_hash ON auth.logins (hash);

CREATE TABLE IF NOT EXISTS auth.sessions (
    id TEXT PRIMARY KEY DEFAULT goddard.ulid(),
    user_id TEXT NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
    hash TEXT NOT NULL,
    expires_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT
);

CREATE INDEX IF NOT EXISTS sessions_hash ON auth.sessions (hash);
