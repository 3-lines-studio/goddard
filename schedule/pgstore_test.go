package schedule

import (
	"context"
	"database/sql"
	"os"
	"slices"
	"testing"

	"github.com/3-lines-studio/goddard/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const testLock = 0x676f6464544553

// testDB deja la base del schema schedule limpia y con las migraciones
// aplicadas. El candado es el mismo que usan las otras partes de goddard para
// que los paquetes que corren en paralelo no se pisen el schema compartido.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("sin TEST_DATABASE_URL no hay Postgres contra el que correr")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("no pude abrir %s: %v", url, err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatalf("no pude hablar con %s: %v", url, err)
	}
	lock, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("no pude reservar una conexión: %v", err)
	}
	t.Cleanup(func() {
		lock.ExecContext(context.WithoutCancel(t.Context()), "SELECT pg_advisory_unlock($1)", testLock)
		lock.Close()
	})
	if _, err := lock.ExecContext(t.Context(), "SELECT pg_advisory_lock($1)", testLock); err != nil {
		t.Fatalf("no pude tomar el candado: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS goddard, auth, chat, heimdall, axe, skill, memo, schedule, org CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatalf("no pude limpiar: %v", err)
	}
	if _, err := migrations.Apply(t.Context(), db); err != nil {
		t.Fatalf("no pude migrar: %v", err)
	}
	return db
}

func testStore(t *testing.T) (*PgStore, *sql.DB) {
	t.Helper()
	db := testDB(t)
	return NewPgStore(db), db
}

func unaTarea(name string) Task {
	return Task{
		Owner:   Owner{Kind: KindUser, ID: "berti"},
		Project: "goddard",
		Name:    name,
		At:      "09:00",
		Prompt:  "hacé algo",
	}
}

// deBerti es la agenda que ve una persona: la suya y la de nadie más.
func deBerti() Viewer {
	return Viewer{User: "berti"}
}

func clk(now int64, date, clock string) Clock {
	return Clock{Now: now, Date: date, Time: clock}
}

func reloj() Clock {
	return clk(1_000, "2026-09-14", "10:00")
}

// corridasDe siembra el log de una tarea con las corridas que le pasen, de la
// más vieja a la más nueva, todas en la misma hora del reloj de los tests.
func corridasDe(t *testing.T, db *sql.DB, task Task, cantidad int) {
	t.Helper()
	for index := 0; index < cantidad; index++ {
		_, err := db.ExecContext(t.Context(),
			`INSERT INTO schedule.runs (owner_kind, owner_id, project, name, run_ts, run_date, ms, ok, body)
			 VALUES ($1, $2, $3, $4, $5, $6, 10, true, 'ok')`,
			task.Owner.Kind, task.Owner.ID, task.Project, task.Name, int64(900+index), "2026-09-14")
		if err != nil {
			t.Fatalf("no pude sembrar una corrida de %q: %v", task.Name, err)
		}
	}
}

func TestAddYListDanLaMismaTarea(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()

	task := unaTarea("recordatorio-tests")
	task.Every = "6h"
	task.Target = "123456789"
	task.Prompt = "Avisá que corra los tests"
	task.Silent = true
	task.Paused = true
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}

	entries, err := store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("quedaron %d tareas", len(entries))
	}
	if got := entries[0].Task; got != task {
		t.Fatalf("la tarea volvió %+v", got)
	}
	if len(entries[0].Runs) != 0 {
		t.Fatalf("una tarea nueva ya trae corridas: %d", len(entries[0].Runs))
	}
}

func TestAddReemplazaLaTarea(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()

	task := unaTarea("recordatorio-tests")
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}
	task.At = "15:00"
	task.Prompt = "otra cosa"
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schedule.tasks`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("quedaron %d filas", rows)
	}
	entries, err := store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Task.At != "15:00" || entries[0].Task.Prompt != "otra cosa" {
		t.Fatalf("la tarea quedó %+v", entries[0].Task)
	}
}

func TestCadaQuienVeSuAgenda(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()

	if err := store.Add(ctx, unaTarea("de-berti")); err != nil {
		t.Fatal(err)
	}
	delProyecto := unaTarea("del-proyecto")
	delProyecto.Owner = Owner{}
	if err := store.Add(ctx, delProyecto); err != nil {
		t.Fatal(err)
	}
	deAna := unaTarea("de-ana")
	deAna.Owner = Owner{Kind: KindUser, ID: "ana"}
	if err := store.Add(ctx, deAna); err != nil {
		t.Fatal(err)
	}
	deLaOrg := unaTarea("de-la-org")
	deLaOrg.Owner = Owner{Kind: KindOrg, ID: "acme"}
	if err := store.Add(ctx, deLaOrg); err != nil {
		t.Fatal(err)
	}
	otroProyecto := unaTarea("de-otro-proyecto")
	otroProyecto.Project = "jimmy"
	if err := store.Add(ctx, otroProyecto); err != nil {
		t.Fatal(err)
	}

	names := []string{}
	entries, err := store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		names = append(names, entry.Task.Name)
	}
	if !slices.Equal(names, []string{"de-berti", "del-proyecto"}) {
		t.Fatalf("la agenda de berti trajo %v", names)
	}

	names = []string{}
	entries, err = store.List(ctx, Viewer{User: "berta", Orgs: []string{"acme"}}, "goddard")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		names = append(names, entry.Task.Name)
	}
	if !slices.Equal(names, []string{"de-la-org", "del-proyecto"}) {
		t.Fatalf("la agenda de un miembro de la org trajo %v", names)
	}

	entries, err = store.List(ctx, Viewer{}, "goddard")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Task.Name != "del-proyecto" {
		t.Fatalf("la agenda del proyecto trajo %v", entries)
	}
}

func TestListAllTraeTodosLosProyectos(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	for _, donde := range []string{"goddard", "picsel"} {
		task := unaTarea("memoria")
		task.Project = donde
		if err := store.Add(ctx, task); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := store.ListAll(ctx, deBerti())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Task.Project != "goddard" || entries[1].Task.Project != "picsel" {
		t.Fatalf("la agenda quedó %+v", entries)
	}
	ajena, err := store.ListAll(ctx, Viewer{User: "otro"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ajena) != 0 {
		t.Fatalf("la agenda ajena quedó %+v", ajena)
	}
}

func TestLoQueNadieLeyoEsLoUltimo(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()
	task := unaTarea("memoria")
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}
	corridasDe(t, db, task, 3)

	entry, err := store.Get(ctx, deBerti(), "goddard", "memoria")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Unread != 3 {
		t.Fatalf("dijo %d sin leer", entry.Unread)
	}

	if err := store.MarkRead(ctx, deBerti(), "goddard", "memoria"); err != nil {
		t.Fatal(err)
	}
	entry, err = store.Get(ctx, deBerti(), "goddard", "memoria")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Unread != 0 {
		t.Fatalf("después de leer quedaron %d", entry.Unread)
	}

	if err := store.Finish(ctx, task, Run{TS: 9_999, Date: "2026-09-14", MS: 1, OK: true, Text: "nueva"}); err != nil {
		t.Fatal(err)
	}
	entry, err = store.Get(ctx, deBerti(), "goddard", "memoria")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Unread != 1 {
		t.Fatalf("la corrida nueva quedó en %d", entry.Unread)
	}
	if err := store.MarkRead(ctx, deBerti(), "goddard", "fantasma"); err == nil {
		t.Fatal("leer una tarea que no existe tendría que fallar")
	}
}

func TestMarcarTodoLeido(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()
	tareas := []Task{unaTarea("memoria"), unaTarea("limpieza")}
	tareas[1].Project = "picsel"
	for _, task := range tareas {
		if err := store.Add(ctx, task); err != nil {
			t.Fatal(err)
		}
		corridasDe(t, db, task, 2)
	}
	if err := store.MarkAllRead(ctx, deBerti()); err != nil {
		t.Fatal(err)
	}
	entries, err := store.ListAll(ctx, deBerti())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("la agenda quedó con %d tareas", len(entries))
	}
	for _, entry := range entries {
		if entry.Unread != 0 {
			t.Fatalf("%s quedó con %d sin leer", entry.Task.Name, entry.Unread)
		}
	}
}

func TestValidateRechazaLoQueNoSePuedeCorrer(t *testing.T) {
	sinHorario := unaTarea("sin-horario")
	sinHorario.At = ""
	if err := Validate(sinHorario); err == nil {
		t.Fatal("una tarea sin horario pasó")
	}
	sinPrompt := unaTarea("sin-prompt")
	sinPrompt.Prompt = "  "
	if err := Validate(sinPrompt); err == nil {
		t.Fatal("una tarea sin prompt pasó")
	}
	mayusculas := unaTarea("Recordatorio")
	if err := Validate(mayusculas); err == nil {
		t.Fatal("un nombre con mayúsculas pasó")
	}

	store, _ := testStore(t)
	if err := store.Add(t.Context(), sinHorario); err == nil {
		t.Fatal("el store guardó una tarea sin horario")
	}
}

func TestPauseYRemove(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	task := unaTarea("recordatorio-tests")
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}

	if err := store.Pause(ctx, deBerti(), "goddard", "recordatorio-tests", true); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	if !entries[0].Task.Paused {
		t.Fatal("la tarea no quedó pausada")
	}

	if err := store.Remove(ctx, deBerti(), "goddard", "recordatorio-tests"); err != nil {
		t.Fatal(err)
	}
	entries, err = store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("quedaron %d tareas", len(entries))
	}
	if err := store.Pause(ctx, deBerti(), "goddard", "recordatorio-tests", true); err == nil {
		t.Fatal("pausar lo que no existe no falló")
	}
	if err := store.Remove(ctx, deBerti(), "goddard", "recordatorio-tests"); err == nil {
		t.Fatal("borrar lo que no existe no falló")
	}
}

func TestUnMiembroDeLaOrgEscribeLaAgendaDeLaOrg(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()

	deLaOrg := unaTarea("del-equipo")
	deLaOrg.Owner = Owner{Kind: KindOrg, ID: "acme"}
	if err := store.Add(ctx, deLaOrg); err != nil {
		t.Fatal(err)
	}

	berta := Viewer{User: "berta", Orgs: []string{"acme"}}
	if err := store.Pause(ctx, berta, "goddard", "del-equipo", true); err != nil {
		t.Fatalf("un miembro de la org no pudo pausarla: %v", err)
	}
	if entry, err := store.Get(ctx, berta, "goddard", "del-equipo"); err != nil || !entry.Task.Paused {
		t.Fatalf("la tarea quedó %+v (%v)", entry.Task, err)
	}
	if err := store.Remove(ctx, Viewer{User: "ajeno"}, "goddard", "del-equipo"); err == nil {
		t.Fatal("alguien de afuera la borró")
	}
	if err := store.Pause(ctx, berta, "goddard", "del-equipo", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(ctx, berta, "goddard", "del-equipo"); err != nil {
		t.Fatalf("un miembro de la org no pudo borrarla: %v", err)
	}
}

func TestClaimTraeLoQueEstaDue(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()

	due := unaTarea("daily")
	if err := store.Add(ctx, due); err != nil {
		t.Fatal(err)
	}
	temprano := unaTarea("mas-tarde")
	temprano.At = "23:00"
	if err := store.Add(ctx, temprano); err != nil {
		t.Fatal(err)
	}
	pausada := unaTarea("pausada")
	pausada.Paused = true
	if err := store.Add(ctx, pausada); err != nil {
		t.Fatal(err)
	}

	entries, err := store.Claim(ctx, reloj(), 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Task.Name != "daily" {
		t.Fatalf("reclamó %v", entries)
	}

	otra, err := store.Claim(ctx, reloj(), 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(otra) != 0 {
		t.Fatalf("la reclamó dos veces: %v", otra)
	}
}

func TestClaimNoRepiteLoQueYaCorrio(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()

	unaVez := unaTarea("una-vez")
	unaVez.At = ""
	unaVez.When = "2026-09-14T09:00"
	if err := store.Add(ctx, unaVez); err != nil {
		t.Fatal(err)
	}
	entries, err := store.Claim(ctx, reloj(), 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("la tarea de una vez no salió: %v", entries)
	}
	if err := store.Finish(ctx, entries[0].Task, Run{TS: 1_000, Date: "2026-09-14", MS: 10, OK: true, Text: "ok"}); err != nil {
		t.Fatal(err)
	}

	otra, err := store.Claim(ctx, reloj(), 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(otra) != 0 {
		t.Fatalf("la de una vez volvió a salir: %v", otra)
	}
}

func TestElLeaseSeVence(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	if err := store.Add(ctx, unaTarea("daily")); err != nil {
		t.Fatal(err)
	}

	if entries, err := store.Claim(ctx, reloj(), 300, 10); err != nil || len(entries) != 1 {
		t.Fatalf("el primer claim dio %v, %v", entries, err)
	}
	if entries, err := store.Claim(ctx, clk(1_200, "2026-09-14", "10:03"), 300, 10); err != nil || len(entries) != 0 {
		t.Fatalf("el lease venció antes de tiempo: %v, %v", entries, err)
	}
	if entries, err := store.Claim(ctx, clk(1_400, "2026-09-14", "10:07"), 300, 10); err != nil || len(entries) != 1 {
		t.Fatalf("el lease vencido no la soltó: %v, %v", entries, err)
	}
}

func TestDosReplicasNoCorrenLaMismaTarea(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()
	otraReplica := NewPgStore(db)
	if err := store.Add(ctx, unaTarea("daily")); err != nil {
		t.Fatal(err)
	}

	primera, err := store.Claim(ctx, reloj(), 300, 10)
	if err != nil {
		t.Fatal(err)
	}
	segunda, err := otraReplica.Claim(ctx, reloj(), 300, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(primera) != 1 || len(segunda) != 0 {
		t.Fatalf("la reclamaron las dos: %d y %d", len(primera), len(segunda))
	}
}

func TestElTopePorHora(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()
	task := unaTarea("cada-minuto")
	task.At = ""
	task.Every = "1m"
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}
	corridasDe(t, db, task, MaxRunsPerHour)

	entries, err := store.Claim(ctx, reloj(), 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("corrió %d veces en la misma hora", len(entries))
	}

	viejo := clk(1_000+Hour, "2026-09-14", "11:00")
	entries, err = store.Claim(ctx, viejo, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("pasada la hora no la corrió: %v", entries)
	}
}

func TestFinishGuardaLaCorridaYPoda(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()
	task := unaTarea("daily")
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}

	run := Run{TS: 1_000, Date: "2026-09-14", MS: 25, OK: false, Text: "se rompió"}
	if err := store.Finish(ctx, task, run); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries[0].Runs) != 1 || entries[0].Runs[0] != run {
		t.Fatalf("la corrida quedó %+v", entries[0].Runs)
	}

	for index := 0; index < Keep+5; index++ {
		if err := store.Finish(ctx, task, Run{TS: int64(1_100 + index), Date: "2026-09-14", MS: 1, OK: true, Text: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err = store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries[0].Runs) != Keep {
		t.Fatalf("quedaron %d corridas", len(entries[0].Runs))
	}
	if entries[0].Runs[0].Text != "se rompió" && entries[0].Runs[0].TS == 1_000 {
		t.Fatal("la poda se llevó la corrida que no correspondía")
	}
	var claimed sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT claimed_until FROM schedule.tasks WHERE name = 'daily'`).Scan(&claimed); err != nil {
		t.Fatal(err)
	}
	if claimed.Valid {
		t.Fatal("la tarea quedó reclamada después de terminar")
	}
}

func TestFinishDeUnaTareaBorradaNoDejaCorrida(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()
	task := unaTarea("efimera")
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}
	entries, err := store.Claim(ctx, reloj(), 300, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("no la reclamó: %v", entries)
	}
	if err := store.Remove(ctx, deBerti(), "goddard", "efimera"); err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, task, Run{TS: 1_000, Date: "2026-09-14", MS: 5, OK: true, Text: "tarde"}); err != nil {
		t.Fatal(err)
	}

	var runs int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schedule.runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("quedaron %d corridas de una tarea borrada", runs)
	}
}
