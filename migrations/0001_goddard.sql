-- Everything goddard shares. Every table of the app follows one convention: a
-- text ULID id minted by the database, created_at, updated_at and deleted_at in
-- seconds since the epoch with NULL meaning alive, and the natural key in its
-- own UNIQUE named <table>_natural instead of as the primary key.

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
