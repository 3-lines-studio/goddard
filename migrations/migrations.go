// Package migrations keeps the schema of every part of goddard in one ordered
// list, so the database is the repository and not somebody's laptop.
package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed *.sql
var files embed.FS

const lockKey = int64(0x676f6464617264)

const versions = `CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// Migration is one file, already parsed: NNNN_parte_tema.sql.
type Migration struct {
	Version int
	Name    string
	Body    string
}

// List is what the repository holds, oldest first, whether it ran or not.
func List() ([]Migration, error) {
	return list(files)
}

// Apply runs what has not run yet and returns their names. Each migration goes
// in its own transaction, so one that fails leaves nothing half done, and an
// advisory lock keeps two instances starting at once from stepping on each
// other. A migration that already ran and no longer says the same thing is an
// error instead of a quiet surprise.
func Apply(ctx context.Context, db *sql.DB) ([]string, error) {
	return apply(ctx, db, files)
}

// State is one migration as the database has it.
type State struct {
	Version   int
	Name      string
	Applied   bool
	AppliedAt time.Time
}

// Status says, for every migration the repository holds, oldest first, whether
// it ran and when. A database that never saw this package has them all
// pending, which is how an operator tells a fresh one from a half migrated
// one without running anything.
func Status(ctx context.Context, db *sql.DB) ([]State, error) {
	known, err := List()
	if err != nil {
		return nil, err
	}
	applied, err := appliedRows(ctx, db)
	if err != nil {
		return nil, err
	}
	states := make([]State, 0, len(known))
	for _, migration := range known {
		state := State{Version: migration.Version, Name: migration.Name}
		if at, ok := applied[migration.Version]; ok {
			state.Applied = true
			state.AppliedAt = at
		}
		states = append(states, state)
	}
	return states, nil
}

func appliedRows(ctx context.Context, db *sql.DB) (map[int]time.Time, error) {
	var there bool
	if err := db.QueryRowContext(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&there); err != nil {
		return nil, fmt.Errorf("no pude mirar public.schema_migrations: %w", err)
	}
	if !there {
		return map[int]time.Time{}, nil
	}
	rows, err := db.QueryContext(ctx, "SELECT version, applied_at FROM public.schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("no pude leer lo que ya corrió: %w", err)
	}
	defer rows.Close()
	applied := map[int]time.Time{}
	for rows.Next() {
		var version int
		var at time.Time
		if err := rows.Scan(&version, &at); err != nil {
			return nil, err
		}
		applied[version] = at
	}
	return applied, rows.Err()
}

func apply(ctx context.Context, db *sql.DB, fsys fs.FS) ([]string, error) {
	pending, err := list(fsys)
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		return nil, fmt.Errorf("no pude tomar el candado de las migraciones: %w", err)
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", lockKey)
	if _, err := conn.ExecContext(ctx, versions); err != nil {
		return nil, fmt.Errorf("no pude armar public.schema_migrations: %w", err)
	}
	applied, err := appliedChecksums(ctx, conn)
	if err != nil {
		return nil, err
	}
	for _, migration := range pending {
		known, ok := applied[migration.Version]
		if !ok {
			continue
		}
		if known != checksum(migration.Body) {
			return nil, fmt.Errorf("la migración %d («%s») ya corrió y ahora dice otra cosa", migration.Version, migration.Name)
		}
	}
	names := []string{}
	for _, migration := range pending {
		if _, ok := applied[migration.Version]; ok {
			continue
		}
		if err := run(ctx, conn, migration); err != nil {
			return nil, err
		}
		names = append(names, migration.Name)
	}
	return names, nil
}

func run(ctx context.Context, conn *sql.Conn, migration Migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("la migración %d («%s»): %w", migration.Version, migration.Name, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, migration.Body); err != nil {
		return fmt.Errorf("la migración %d («%s»): %w", migration.Version, migration.Name, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO public.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)",
		migration.Version, migration.Name, checksum(migration.Body),
	); err != nil {
		return fmt.Errorf("la migración %d («%s»): %w", migration.Version, migration.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("la migración %d («%s»): %w", migration.Version, migration.Name, err)
	}
	return nil
}

func appliedChecksums(ctx context.Context, conn *sql.Conn) (map[int]string, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version, checksum FROM public.schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("no pude leer lo que ya corrió: %w", err)
	}
	defer rows.Close()
	applied := map[int]string{}
	for rows.Next() {
		var version int
		var sum string
		if err := rows.Scan(&version, &sum); err != nil {
			return nil, err
		}
		applied[version] = sum
	}
	return applied, rows.Err()
}

func list(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	migrations := []Migration{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, name, err := parseName(entry.Name())
		if err != nil {
			return nil, err
		}
		body, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, Migration{Version: version, Name: name, Body: string(body)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	for index := 1; index < len(migrations); index++ {
		if migrations[index-1].Version == migrations[index].Version {
			return nil, fmt.Errorf("hay dos migraciones con el número %d", migrations[index].Version)
		}
	}
	return migrations, nil
}

func parseName(file string) (int, string, error) {
	digits, name, found := strings.Cut(strings.TrimSuffix(file, ".sql"), "_")
	if !found {
		return 0, "", fmt.Errorf("la migración «%s» no dice de qué parte es ni de qué tema", file)
	}
	version, err := strconv.Atoi(digits)
	if err != nil {
		return 0, "", fmt.Errorf("la migración «%s» no arranca con un número", file)
	}
	return version, name, nil
}

func checksum(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}
