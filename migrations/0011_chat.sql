-- El slug de un proyecto nombra su carpeta en el workspace, y el workspace es
-- del dueño: la carpeta de «goddard» de un usuario y la de «goddard» de una
-- organización son dos carpetas. La clave natural pasa a ser del dueño, como
-- la de la memoria, la agenda y los secretos, que ya llevan el dueño adentro.
ALTER TABLE chat.projects DROP CONSTRAINT IF EXISTS projects_natural;

ALTER TABLE chat.projects
    ADD CONSTRAINT projects_natural UNIQUE (owner_kind, owner_id, slug);
