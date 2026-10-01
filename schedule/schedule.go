// Package schedule keeps the programmed tasks of goddard: prompts the app
// runs on their own, in a clean context, and the log of what each run
// answered. Port of jimmy's `src/schedule.rs`, with the directory of TOML
// files replaced by rows in the `schedule` schema of the shared database (see
// migrations/).
//
// A task says when it runs in one of three ways and only one: `when`, a date
// and a time it fires once, `at`, an hour it fires every day, or `every`, a
// period since the last run. The local hour is UTC plus an offset, and a task
// with none of the three never fires. What the app keeps of a task is its
// last runs, and that log is also its state: the last run is what `every`
// measures from and the one that tells `when` and `at` they already fired.
package schedule

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	Hour int64 = 3_600
	Day  int64 = 86_400
)

// Owner is who a task belongs to: the person, or the organization that owns
// the project it lives in. The empty owner is the project's own task, the one
// everybody who sees the project sees.
type Owner struct {
	Kind string
	ID   string
}

// The kinds of owner, the same two the rest of goddard knows.
const (
	KindUser = "user"
	KindOrg  = "org"
)

// Viewer is who is asking for the agenda: the person and the organizations
// they are in. The tasks of the project itself — the ones nobody owns — come
// along, which is what the empty owner means.
type Viewer struct {
	User string
	Orgs []string
}

// Task is one programmed task. It belongs to an owner inside a project, which
// is the project's owner unless it is one of nobody. Target is the chat a copy
// of the answer goes to, if any: the run is kept either way. Seen is the moment
// its log was last read, which is what the runs newer than it are counted
// against.
type Task struct {
	Owner   Owner
	Project string
	Name    string
	When    string
	At      string
	Every   string
	Target  string
	Prompt  string
	Silent  bool
	Paused  bool
	Seen    int64
}

// Run is one time a task ran: when, how long it took, whether it went well and
// what it answered.
type Run struct {
	TS   int64
	Date string
	MS   int64
	OK   bool
	Text string
}

// Entry is a task with its runs, what the list shows.
type Entry struct {
	Task   Task
	Runs   []Run
	Unread int
}

// Clock is the local moment a tick runs at: what Due needs and nothing else.
type Clock struct {
	Now  int64
	Date string
	Time string
}

// At is the local moment a Unix second falls on, with the hours of offset the
// app runs in.
func At(unix, offset int64) Clock {
	date, clock := LocalParts(unix, offset)
	return Clock{Now: unix, Date: date, Time: clock}
}

// Due says whether the task has to run now, given its last runs and the local
// date and time. A one-shot fires once, after its time has come; a daily fires
// the first tick of a date it has not run on; an interval fires once the
// period went by since the last run.
func Due(task Task, runs []Run, now int64, date, time string) bool {
	if task.When != "" {
		return len(runs) == 0 && date+"T"+time >= strings.ReplaceAll(strings.TrimSpace(task.When), " ", "T")
	}
	if task.At != "" {
		at, ok := HHMM(task.At)
		if !ok {
			return false
		}
		return time >= at && lastDate(runs) != date
	}
	if task.Every != "" {
		period, ok := Period(task.Every)
		if !ok {
			return false
		}
		last := lastRunOf(runs)
		return last == 0 || now-last >= period
	}
	return false
}

// HHMM reads an hour as `HH:MM`, the way a daily task writes it.
func HHMM(s string) (string, bool) {
	hour, minute, found := strings.Cut(strings.TrimSpace(s), ":")
	if !found {
		return "", false
	}
	h, err := strconv.ParseUint(strings.TrimSpace(hour), 10, 32)
	if err != nil || h > 23 {
		return "", false
	}
	m, err := strconv.ParseUint(strings.TrimSpace(minute), 10, 32)
	if err != nil || m > 59 {
		return "", false
	}
	return fmt.Sprintf("%02d:%02d", h, m), true
}

// Period reads an interval as a number and a unit: `s`, `m`, `h` or `d`.
func Period(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) < 2 {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(runes[:len(runes)-1])), 10, 64)
	if err != nil {
		return 0, false
	}
	var factor int64
	switch runes[len(runes)-1] {
	case 's':
		factor = 1
	case 'm':
		factor = 60
	case 'h':
		factor = Hour
	case 'd':
		factor = Day
	default:
		return 0, false
	}
	return value * factor, true
}

// LocalParts is the day and the hour a moment falls on, in the local time the
// offset asks for.
func LocalParts(now, offset int64) (string, string) {
	local := now + offset*Hour
	days := floorDiv(local, Day)
	secs := floorMod(local, Day)
	year, month, day := CivilFromDays(days)
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day),
		fmt.Sprintf("%02d:%02d", secs/Hour, secs%Hour/60)
}

// CivilFromDays is the calendar date of a day count since the epoch, in UTC.
// The stdlib would need a location to do this, and the port is the same
// arithmetic jimmy wrote.
func CivilFromDays(days int64) (int64, int64, int64) {
	z := days + 719_468
	era := floorDiv(z, 146_097)
	doe := floorMod(z, 146_097)
	yoe := (doe - doe/1_460 + doe/36_524 - doe/146_096) / 365
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	day := doy - (153*mp+2)/5 + 1
	month := mp - 9
	if mp < 10 {
		month = mp + 3
	}
	year := yoe + era*400
	if month <= 2 {
		year++
	}
	return year, month, day
}

// Render is the agenda as the model reads it: one line per task with its
// schedule, and under it the last run, which is what says whether it is
// working.
func Render(entries []Entry) string {
	if len(entries) == 0 {
		return "no hay tareas"
	}
	lines := make([]string, 0, 2*len(entries))
	for _, entry := range entries {
		lines = append(lines, head(entry.Task), "  "+lastRun(entry.Runs))
	}
	return strings.Join(lines, "\n")
}

// Show is one task as the model reads it: what it says, when it runs and what
// its last runs answered.
func Show(entry Entry) string {
	lines := []string{head(entry.Task), "prompt: " + entry.Task.Prompt}
	if len(entry.Runs) == 0 {
		return strings.Join(append(lines, "todavía no corrió"), "\n")
	}
	lines = append(lines, "corridas:")
	for _, run := range entry.Runs {
		lines = append(lines, fmt.Sprintf(" %s · %s · %d ms\n %s", run.Date, verdict(run.OK), run.MS, run.Text))
	}
	return strings.Join(lines, "\n")
}

func head(task Task) string {
	parts := []string{task.Name, scheduleOf(task)}
	if task.Target != "" {
		parts = append(parts, "avisa a "+task.Target)
	}
	if task.Paused {
		parts = append(parts, "pausada")
	}
	return strings.Join(parts, " · ")
}

func scheduleOf(task Task) string {
	switch {
	case task.When != "":
		return "una vez " + task.When
	case task.At != "":
		return "todos los días " + task.At
	case task.Every != "":
		return "cada " + task.Every
	}
	return "sin horario"
}

func lastRun(runs []Run) string {
	if len(runs) == 0 {
		return "todavía no corrió"
	}
	run := runs[len(runs)-1]
	return fmt.Sprintf("última %s · %s · %s", run.Date, verdict(run.OK), firstLine(run.Text))
}

func verdict(ok bool) string {
	if ok {
		return "ok"
	}
	return "falló"
}

// firstLine is what one run said, cut down: the list is an index, and a run
// that answered with an essay would push the rest of the agenda out of sight.
func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	runes := []rune(line)
	if len(runes) > 200 {
		return string(runes[:200]) + "…"
	}
	return line
}

func lastRunOf(runs []Run) int64 {
	if len(runs) == 0 {
		return 0
	}
	return runs[len(runs)-1].TS
}

func lastDate(runs []Run) string {
	if len(runs) == 0 {
		return ""
	}
	return runs[len(runs)-1].Date
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func floorMod(a, b int64) int64 {
	r := a % b
	if r != 0 && (r < 0) != (b < 0) {
		r += b
	}
	return r
}
