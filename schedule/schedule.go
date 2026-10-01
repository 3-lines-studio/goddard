// Package schedule keeps the programmed tasks of goddard: prompts the service
// runs on their own, in a clean context, and the log of what each run
// answered. Port of jimmy's `src/schedule.rs`, with the directory of TOML
// files replaced by rows in the `schedule` schema of the shared database (see
// migrations/).
//
// A task says when it runs in one of three ways and only one: `when`, a date
// and a time it fires once, `at`, an hour it fires every day, or `every`, a
// period since the last run. The local hour is UTC plus an offset, and a task
// with none of the three never fires. What the service keeps of a task is its
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

// Task is one programmed task. It belongs to a user or to a project, and
// Target is the chat a copy of the answer goes to, if any: the run is kept
// either way.
type Task struct {
	Name   string
	When   string
	At     string
	Every  string
	Target string
	Prompt string
	Silent bool
	Paused bool
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
	Task Task
	Runs []Run
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
		last := lastRun(runs)
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

func lastRun(runs []Run) int64 {
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
