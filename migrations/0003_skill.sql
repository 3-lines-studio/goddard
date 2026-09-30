CREATE SCHEMA IF NOT EXISTS skill;

CREATE TABLE IF NOT EXISTS skill.skills (
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('system', 'org', 'user')),
    owner_id TEXT NOT NULL,
    role TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    body TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    PRIMARY KEY (owner_kind, owner_id, name)
);
