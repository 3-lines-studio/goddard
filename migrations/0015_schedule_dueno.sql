ALTER TABLE schedule.tasks ADD COLUMN owner_kind TEXT NOT NULL, ADD COLUMN owner_id TEXT NOT NULL;
ALTER TABLE schedule.tasks DROP COLUMN user_id;
ALTER TABLE schedule.tasks ADD CONSTRAINT tasks_natural UNIQUE (owner_kind, owner_id, project, name);

ALTER TABLE schedule.runs ADD COLUMN owner_kind TEXT NOT NULL, ADD COLUMN owner_id TEXT NOT NULL;
ALTER TABLE schedule.runs DROP COLUMN user_id;

DROP INDEX IF EXISTS schedule.runs_task;
CREATE INDEX IF NOT EXISTS runs_task ON schedule.runs (owner_kind, owner_id, project, name, seq DESC);
