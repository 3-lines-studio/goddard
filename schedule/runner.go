package schedule

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// Tick is how often the app looks for tasks to run.
	Tick = 60 * time.Second

	// Lease is how long a claim holds, in seconds. It is long on purpose: a
	// task the agent is working on is not up for grabs just because it takes a
	// while. An instance that dies mid-run leaves its task free after this.
	Lease int64 = 900

	// Limit is how many tasks one tick takes at most.
	Limit = 20
)

// Runner answers a task: it runs the prompt in a clean context, without the
// conversation it came from, and returns what the agent said. The app puts
// it together with the model and the tools it already has.
type Runner func(ctx context.Context, task Task) (string, error)

// Notifier is where a copy of the answer goes when the task names a target.
// The agenda lives in the web, so this is the backup the task asked for, and a
// target that does not answer is not a task that did not run.
type Notifier interface {
	Notify(ctx context.Context, target, text string) error
}

// Service runs the tasks that are due. It does not care how many instances are
// running: the store hands each task to one of them, and the one that dies
// before finishing it leaves it for the next tick.
type Service struct {
	store  *PgStore
	run    Runner
	notify Notifier
	offset int64
}

// NewService is a service over that store, running with the local hour the
// offset asks for. The offset is the same one the rest of the app uses.
func NewService(store *PgStore, run Runner, offset int64) *Service {
	return &Service{store: store, run: run, offset: offset}
}

// WithNotifier is where the answers with a target go.
func (s *Service) WithNotifier(notify Notifier) *Service {
	s.notify = notify
	return s
}

// Once runs a single pass over what is due at that moment, and returns what
// went wrong with the runs that could not be written down.
func (s *Service) Once(ctx context.Context, clock Clock) error {
	entries, err := s.store.Claim(ctx, clock, Lease, Limit)
	if err != nil {
		return err
	}
	failures := []error{}
	for _, entry := range entries {
		if _, err := s.runOne(ctx, entry, clock); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// RunNow runs a task with no wait, the way the run button of the web does. It
// runs it even if it already ran: it was asked for.
func (s *Service) RunNow(ctx context.Context, userID, project, name string) (Run, error) {
	entry, err := s.store.Get(ctx, userID, project, name)
	if err != nil {
		return Run{}, err
	}
	return s.runOne(ctx, entry, At(time.Now().Unix(), s.offset))
}

// Serve runs a pass every minute until the context is done. A pass that fails
// is not the end of the agenda — the next one finds the database again — so
// the error goes to report and the loop keeps its word.
func (s *Service) Serve(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(Tick)
	defer ticker.Stop()
	for {
		if err := s.Once(ctx, At(time.Now().Unix(), s.offset)); err != nil && report != nil {
			report(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runOne answers a task and writes the run down, stamped with the moment of
// the pass, the way jimmy stamped it with the moment of the tick.
func (s *Service) runOne(ctx context.Context, entry Entry, clock Clock) (Run, error) {
	started := time.Now()

	text, err := s.run(ctx, entry.Task)
	run := Run{TS: clock.Now, Date: clock.Date, OK: err == nil, Text: text}
	if err != nil {
		run.Text = err.Error()
	}
	run.MS = time.Since(started).Milliseconds()
	if warning := s.deliver(ctx, entry.Task, run.Text); warning != "" {
		if run.Text != "" {
			run.Text += "\n\n"
		}
		run.Text += warning
	}

	if err := s.store.Finish(ctx, entry.Task, run); err != nil {
		return run, err
	}
	return run, nil
}

// deliver sends the answer to the target of the task, if it has one, and
// answers with what to write down when it did not go out. A task with nowhere
// to report has nothing to say about it: what the store keeps is the run.
func (s *Service) deliver(ctx context.Context, task Task, text string) string {
	if task.Target == "" || strings.TrimSpace(text) == "" {
		return ""
	}
	if s.notify == nil {
		return fmt.Sprintf("⚠️ no salió a %s: no hay con qué avisar", task.Target)
	}
	if err := s.notify.Notify(ctx, task.Target, text); err != nil {
		return fmt.Sprintf("⚠️ no salió a %s: %v", task.Target, err)
	}
	return ""
}
