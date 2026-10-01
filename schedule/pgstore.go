package schedule

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Keep is how many runs of a task the store holds: the log is also the state
// of the task, and nothing looks further back than the last twenty.
const Keep = 20

// MaxRunsPerHour is the ceiling of a task in an hour. A task that fires every
// minute is a mistake, and the store stops running it instead of letting it
// eat the day.
const MaxRunsPerHour = 6

// PgStore keeps the tasks and their runs in the schedule schema of the shared
// database (see migrations/), over the same pool as the rest of goddard.
type PgStore struct {
	db *sql.DB
}

func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

const taskColumns = `user_id, project, name, once_at, daily_at, every, target, prompt, silent, paused`

const runColumns = `run_ts, run_date, ms, ok, body`

// querier is what the store asks for rows, so one operation can read inside
// its own transaction or outside one.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Add writes a task, replacing the one with the same name of that user and
// project: editing a task and creating it are one gesture, the way writing the
// file was in jimmy.
func (s *PgStore) Add(ctx context.Context, task Task) error {
	if err := Validate(task); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO schedule.tasks (`+taskColumns+`, claimed_until)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULL)
		 ON CONFLICT (user_id, project, name) DO UPDATE SET
		     once_at = EXCLUDED.once_at,
		     daily_at = EXCLUDED.daily_at,
		     every = EXCLUDED.every,
		     target = EXCLUDED.target,
		     prompt = EXCLUDED.prompt,
		     silent = EXCLUDED.silent,
		     paused = EXCLUDED.paused,
		     claimed_until = NULL`,
		task.UserID, task.Project, task.Name, task.When, task.At, task.Every,
		task.Target, task.Prompt, task.Silent, task.Paused)
	if err != nil {
		return fmt.Errorf("no pude guardar la tarea %q: %w", task.Name, err)
	}
	return nil
}

// List is the tasks of that user in that project, by name, each one with its
// runs.
func (s *PgStore) List(ctx context.Context, userID, project string) ([]Entry, error) {
	tasks, err := tasksOf(ctx, s.db,
		`SELECT `+taskColumns+` FROM schedule.tasks WHERE user_id = $1 AND project = $2 ORDER BY name`,
		userID, project)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(tasks))
	for _, task := range tasks {
		runs, err := runsOf(ctx, s.db, task)
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{Task: task, Runs: runs})
	}
	return entries, nil
}

// Get is one task with its runs.
func (s *PgStore) Get(ctx context.Context, userID, project, name string) (Entry, error) {
	tasks, err := tasksOf(ctx, s.db,
		`SELECT `+taskColumns+` FROM schedule.tasks WHERE user_id = $1 AND project = $2 AND name = $3`,
		userID, project, name)
	if err != nil {
		return Entry{}, err
	}
	if len(tasks) == 0 {
		return Entry{}, fmt.Errorf("no existe la tarea %q", name)
	}
	runs, err := runsOf(ctx, s.db, tasks[0])
	if err != nil {
		return Entry{}, err
	}
	return Entry{Task: tasks[0], Runs: runs}, nil
}

// Pause stops a task from running without losing it.
func (s *PgStore) Pause(ctx context.Context, userID, project, name string, paused bool) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE schedule.tasks SET paused = $4 WHERE user_id = $1 AND project = $2 AND name = $3`,
		userID, project, name, paused)
	return touched(result, err, name)
}

// Remove is how a task stops existing.
func (s *PgStore) Remove(ctx context.Context, userID, project, name string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM schedule.tasks WHERE user_id = $1 AND project = $2 AND name = $3`,
		userID, project, name)
	return touched(result, err, name)
}

// Claim takes the tasks that are due right now and marks them, so the tick of
// another instance does not run them too. The mark is a lease: the store only
// hands over a task nobody holds, and an instance that dies mid-run leaves its
// task free for the first tick after the lease runs out. The work happens
// outside this transaction, so the claim is a moment and not the whole run.
func (s *PgStore) Claim(ctx context.Context, clock Clock, lease int64, limit int) ([]Entry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("no pude reclamar tareas: %w", err)
	}
	defer tx.Rollback()

	tasks, err := tasksOf(ctx, tx,
		`SELECT `+taskColumns+` FROM schedule.tasks
		 WHERE NOT paused AND (claimed_until IS NULL OR claimed_until < $1)
		 ORDER BY user_id, project, name
		 LIMIT $2
		 FOR UPDATE SKIP LOCKED`,
		clock.Now, limit)
	if err != nil {
		return nil, err
	}

	due := []Entry{}
	for _, task := range tasks {
		runs, err := runsOf(ctx, tx, task)
		if err != nil {
			return nil, err
		}
		if !Due(task, runs, clock.Now, clock.Date, clock.Time) {
			continue
		}
		if ranThisHour(runs, clock.Now) >= MaxRunsPerHour {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE schedule.tasks SET claimed_until = $4 WHERE user_id = $1 AND project = $2 AND name = $3`,
			task.UserID, task.Project, task.Name, clock.Now+lease); err != nil {
			return nil, fmt.Errorf("no pude reclamar la tarea %q: %w", task.Name, err)
		}
		due = append(due, Entry{Task: task, Runs: runs})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("no pude reclamar tareas: %w", err)
	}
	return due, nil
}

// Finish writes the run of a task and frees it. A task deleted while it ran
// does not leave its run behind: the log of a task that is gone is not worth
// keeping.
func (s *PgStore) Finish(ctx context.Context, task Task, run Run) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("no pude guardar la corrida de %q: %w", task.Name, err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		`UPDATE schedule.tasks SET claimed_until = NULL WHERE user_id = $1 AND project = $2 AND name = $3`,
		task.UserID, task.Project, task.Name)
	if err != nil {
		return fmt.Errorf("no pude liberar la tarea %q: %w", task.Name, err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return nil
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schedule.runs (user_id, project, name, `+runColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		task.UserID, task.Project, task.Name, run.TS, run.Date, run.MS, run.OK, run.Text); err != nil {
		return fmt.Errorf("no pude guardar la corrida de %q: %w", task.Name, err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM schedule.runs
		 WHERE user_id = $1 AND project = $2 AND name = $3
		   AND id NOT IN (
		       SELECT id FROM schedule.runs
		       WHERE user_id = $1 AND project = $2 AND name = $3
		       ORDER BY id DESC LIMIT $4
		   )`,
		task.UserID, task.Project, task.Name, Keep); err != nil {
		return fmt.Errorf("no pude podar las corridas de %q: %w", task.Name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("no pude guardar la corrida de %q: %w", task.Name, err)
	}
	return nil
}

// Validate checks a task before it reaches the store. What a task does not
// say is when it runs: without a schedule it never fires, and a schedule
// nobody can read is a task that would sit there looking alive.
func Validate(task Task) error {
	if !validName(task.Name) {
		return fmt.Errorf("nombre inválido: %q (minúsculas, números, `-` y `.`)", task.Name)
	}
	if strings.TrimSpace(task.Prompt) == "" {
		return fmt.Errorf("la tarea %q no dice qué hacer", task.Name)
	}
	if task.When == "" && task.At == "" && task.Every == "" {
		return fmt.Errorf("la tarea %q no dice cuándo corre", task.Name)
	}
	if task.When != "" {
		if _, err := time.Parse("2006-01-02T15:04", strings.ReplaceAll(strings.TrimSpace(task.When), " ", "T")); err != nil {
			return fmt.Errorf("la tarea %q tiene una fecha que no entiendo: %q (YYYY-MM-DDTHH:MM)", task.Name, task.When)
		}
	}
	if task.At != "" {
		if _, ok := HHMM(task.At); !ok {
			return fmt.Errorf("la tarea %q tiene una hora que no existe: %q (HH:MM)", task.Name, task.At)
		}
	}
	if task.Every != "" {
		if _, ok := Period(task.Every); !ok {
			return fmt.Errorf("la tarea %q tiene un intervalo que no entiendo: %q (30m, 6h, 2d)", task.Name, task.Every)
		}
	}
	return nil
}

func validName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}

func tasksOf(ctx context.Context, from querier, query string, args ...any) ([]Task, error) {
	rows, err := from.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("no pude leer las tareas: %w", err)
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.UserID, &task.Project, &task.Name, &task.When, &task.At, &task.Every,
			&task.Target, &task.Prompt, &task.Silent, &task.Paused); err != nil {
			return nil, fmt.Errorf("no pude leer una tarea: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("no pude leer las tareas: %w", err)
	}
	return tasks, nil
}

// runsOf is the log of a task, oldest first: the last one is the run Due and
// the interval measure from.
func runsOf(ctx context.Context, from querier, task Task) ([]Run, error) {
	rows, err := from.QueryContext(ctx,
		`SELECT `+runColumns+` FROM schedule.runs
		 WHERE user_id = $1 AND project = $2 AND name = $3
		 ORDER BY id DESC LIMIT $4`,
		task.UserID, task.Project, task.Name, Keep)
	if err != nil {
		return nil, fmt.Errorf("no pude leer las corridas de %q: %w", task.Name, err)
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var run Run
		if err := rows.Scan(&run.TS, &run.Date, &run.MS, &run.OK, &run.Text); err != nil {
			return nil, fmt.Errorf("no pude leer una corrida: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("no pude leer las corridas de %q: %w", task.Name, err)
	}
	slices.Reverse(runs)
	return runs, nil
}

func ranThisHour(runs []Run, now int64) int {
	count := 0
	for _, run := range runs {
		if now-run.TS < Hour {
			count++
		}
	}
	return count
}

func touched(result sql.Result, err error, name string) error {
	if err != nil {
		return fmt.Errorf("no pude tocar la tarea %q: %w", name, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("no pude tocar la tarea %q: %w", name, err)
	}
	if affected == 0 {
		return fmt.Errorf("no existe la tarea %q", name)
	}
	return nil
}
