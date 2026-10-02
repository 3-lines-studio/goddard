-- Un turno del agente y lo que costó. La tabla no guarda ni el pedido ni la
-- respuesta: eso ya vive en chat.events y en axe.entries. Lo que hay acá son
-- números y claves para poder sumarlos por dueño, proyecto, modelo y tiempo.
CREATE SCHEMA IF NOT EXISTS metric;

CREATE TABLE IF NOT EXISTS metric.turns (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('org', 'user')),
    owner_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    user_id TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'web' CHECK (source IN ('web', 'telegram', 'slack', 'schedule')),
    model TEXT NOT NULL DEFAULT '',
    input INT NOT NULL DEFAULT 0,
    output INT NOT NULL DEFAULT 0,
    cached_input INT NOT NULL DEFAULT 0,
    ms BIGINT NOT NULL DEFAULT 0,
    outcome TEXT NOT NULL CHECK (outcome IN ('ok', 'failed', 'cancelled')),
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS turns_created_idx ON metric.turns (created_at);
CREATE INDEX IF NOT EXISTS turns_owner_idx ON metric.turns (owner_kind, owner_id, created_at);
