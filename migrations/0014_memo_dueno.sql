ALTER TABLE memo.facts
    ADD COLUMN owner_kind TEXT NOT NULL,
    ADD COLUMN owner_id TEXT NOT NULL;

ALTER TABLE memo.facts DROP CONSTRAINT facts_natural;
ALTER TABLE memo.facts ADD CONSTRAINT facts_natural UNIQUE (owner_kind, owner_id, project, key);

DROP INDEX IF EXISTS memo.facts_project;
CREATE INDEX IF NOT EXISTS facts_project ON memo.facts (owner_kind, owner_id, project, fact_date DESC, key);

ALTER TABLE memo.revisions
    ADD COLUMN owner_kind TEXT NOT NULL,
    ADD COLUMN owner_id TEXT NOT NULL;

ALTER TABLE memo.revisions DROP CONSTRAINT revisions_natural;
ALTER TABLE memo.revisions ADD CONSTRAINT revisions_natural UNIQUE (owner_kind, owner_id, project, key, rev);
