ALTER TABLE chat.projects
    ADD COLUMN owner_kind TEXT NOT NULL,
    ADD COLUMN owner_id TEXT NOT NULL;

ALTER TABLE chat.projects
    RENAME CONSTRAINT projects_slug_key TO projects_natural;
