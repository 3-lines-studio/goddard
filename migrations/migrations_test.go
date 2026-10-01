package migrations

import (
	"context"
	"database/sql"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const testLock = 0x676f6464544553

// testDB deja la base limpia. El candado es para que los paquetes que corren en
// paralelo no se pisen el schema.
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
	if err := db.Ping(); err != nil {
		t.Fatalf("no pude hablar con %s: %v", url, err)
	}
	t.Cleanup(func() { db.Close() })
	lock, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("no pude reservar una conexión: %v", err)
	}
	t.Cleanup(func() {
		lock.ExecContext(context.WithoutCancel(context.Background()), "SELECT pg_advisory_unlock($1)", testLock)
		lock.Close()
	})
	if _, err := lock.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", testLock); err != nil {
		t.Fatalf("no pude tomar el candado: %v", err)
	}
	reset(t, db)
	return db
}

func reset(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec("DROP SCHEMA IF EXISTS goddard, auth, chat, heimdall, axe, skill, memo, schedule, org CASCADE; DROP TABLE IF EXISTS public.schema_migrations")
	if err != nil {
		t.Fatalf("no pude limpiar: %v", err)
	}
}

func tables(t *testing.T, db *sql.DB, schema string) []string {
	t.Helper()
	rows, err := db.Query(
		"SELECT table_name FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name", schema)
	if err != nil {
		t.Fatalf("no pude leer las tablas: %v", err)
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("no pude leer las tablas: %v", err)
		}
		names = append(names, name)
	}
	return names
}

func appliedVersions(t *testing.T, db *sql.DB) []int {
	t.Helper()
	rows, err := db.Query("SELECT version FROM public.schema_migrations ORDER BY version")
	if err != nil {
		t.Fatalf("no pude leer las versiones: %v", err)
	}
	defer rows.Close()
	versions := []int{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			t.Fatalf("no pude leer las versiones: %v", err)
		}
		versions = append(versions, version)
	}
	return versions
}

func TestAplicaElEsquemaDeHeimdall(t *testing.T) {
	db := testDB(t)
	applied, err := Apply(context.Background(), db)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied) != len(pending(t)) || applied[0] != "heimdall" {
		t.Fatalf("aplicó %v", applied)
	}
	want := []string{"audit", "environments", "secrets", "tokens"}
	got := strings.Join(tables(t, db, "heimdall"), ",")
	if got != strings.Join(want, ",") {
		t.Fatalf("las tablas quedaron %s", got)
	}
	wanted := "project:text,env:text,name:text,value:bytea,updated_at:bigint,id:text,created_at:bigint,deleted_at:bigint,owner_kind:text,owner_id:text"
	if got := columnsOf(t, db, "heimdall", "secrets"); strings.Join(got, ",") != wanted {
		t.Fatalf("heimdall.secrets quedó %v", got)
	}
	wantedTokens := "id:text,name:text,project:text,env:text,keys:jsonb,hash:text,role:text,created_at:bigint,expires_at:bigint,last_used:bigint,updated_at:bigint,deleted_at:bigint,owner_kind:text,owner_id:text"
	if got := columnsOf(t, db, "heimdall", "tokens"); strings.Join(got, ",") != wantedTokens {
		t.Fatalf("heimdall.tokens quedó %v", got)
	}
}

func TestAplicaElEsquemaDeAxe(t *testing.T) {
	db := testDB(t)
	if _, err := Apply(context.Background(), db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := []string{"entries", "resume", "sessions"}
	got := strings.Join(tables(t, db, "axe"), ",")
	if got != strings.Join(want, ",") {
		t.Fatalf("las tablas quedaron %s", got)
	}
	wantedSessions := "id:text,scope:text,title:text,turns:integer,updated_at:bigint,deleted_at:bigint,seq:bigint,created_at:bigint"
	if got := columnsOf(t, db, "axe", "sessions"); strings.Join(got, ",") != wantedSessions {
		t.Fatalf("axe.sessions quedó %v", got)
	}
	wantedEntries := "session_id:text,seq:bigint,entry:jsonb,id:text,created_at:bigint,updated_at:bigint,deleted_at:bigint"
	if got := columnsOf(t, db, "axe", "entries"); strings.Join(got, ",") != wantedEntries {
		t.Fatalf("axe.entries quedó %v", got)
	}
}

// pending es lo que el repositorio tiene para aplicar, que es lo que Apply
// aplica cuando la base está limpia.
func pending(t *testing.T) []Migration {
	t.Helper()
	list, err := List()
	if err != nil {
		t.Fatalf("no pude listar: %v", err)
	}
	return list
}

func columnsOf(t *testing.T, db *sql.DB, schema, table string) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT column_name || ':' || data_type FROM information_schema.columns
         WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`, schema, table)
	if err != nil {
		t.Fatalf("no pude leer las columnas de %s.%s: %v", schema, table, err)
	}
	defer rows.Close()
	columns := []string{}
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("no pude leer las columnas de %s.%s: %v", schema, table, err)
		}
		columns = append(columns, column)
	}
	return columns
}

func TestElRolEsUnEnum(t *testing.T) {
	db := testDB(t)
	if _, err := Apply(context.Background(), db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	insert := func(id, role string, keys any) error {
		_, err := db.Exec(
			`INSERT INTO heimdall.tokens (id, name, owner_kind, owner_id, project, env, keys, hash, role, created_at)
             VALUES ($1, $1, 'org', 'acme', 'bifrost', 'dev', $2, 'hash', $3, 0)`, id, keys, role)
		return err
	}
	if err := insert("t1", "admin", nil); err != nil {
		t.Fatalf("no aceptó admin: %v", err)
	}
	if err := insert("t2", "root", nil); err == nil {
		t.Fatal("aceptó un rol que no existe")
	}
	if err := insert("t3", "agent", "no es json"); err == nil {
		t.Fatal("aceptó claves que no son json")
	}
	if err := insert("t4", "agent", `["A","B"]`); err != nil {
		t.Fatalf("no aceptó claves json: %v", err)
	}
	var keys string
	if err := db.QueryRow("SELECT keys::text FROM heimdall.tokens WHERE id = 't4'").Scan(&keys); err != nil {
		t.Fatalf("no pude leer las claves: %v", err)
	}
	if keys != `["A", "B"]` {
		t.Fatalf("las claves quedaron %q", keys)
	}
	if _, err := db.Exec(
		`INSERT INTO heimdall.tokens (id, name, owner_kind, owner_id, project, env, hash, created_at) VALUES ('t5', 'sin rol', 'org', 'acme', 'bifrost', 'dev', 'hash', 0)`); err != nil {
		t.Fatalf("no aceptó el rol por defecto: %v", err)
	}
	var role string
	if err := db.QueryRow("SELECT role FROM heimdall.tokens WHERE id = 't5'").Scan(&role); err != nil {
		t.Fatalf("no pude leer el rol: %v", err)
	}
	if role != "agent" {
		t.Fatalf("el rol por defecto quedó %q", role)
	}
}

func TestNoAplicaDosVeces(t *testing.T) {
	db := testDB(t)
	if _, err := Apply(context.Background(), db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	applied, err := Apply(context.Background(), db)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("volvió a aplicar %v", applied)
	}
	if got := appliedVersions(t, db); len(got) != len(pending(t)) {
		t.Fatalf("quedaron %v", got)
	}
}

func TestUnaMigracionCambiadaSeQueja(t *testing.T) {
	db := testDB(t)
	if _, err := Apply(context.Background(), db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := db.Exec("UPDATE public.schema_migrations SET checksum = 'otro' WHERE version = 1"); err != nil {
		t.Fatalf("no pude ensuciar: %v", err)
	}
	_, err := Apply(context.Background(), db)
	if err == nil {
		t.Fatal("no se quejó")
	}
	if !strings.Contains(err.Error(), "ya corrió y ahora dice otra cosa") {
		t.Fatalf("dijo %q", err)
	}
}

func TestUnaMigracionQueFallaNoDejaNada(t *testing.T) {
	db := testDB(t)
	db.Exec("DROP TABLE IF EXISTS public.migracion_una; DROP TABLE IF EXISTS public.migracion_dos")
	t.Cleanup(func() {
		db.Exec("DROP TABLE IF EXISTS public.migracion_una; DROP TABLE IF EXISTS public.migracion_dos")
	})
	fake := fstest.MapFS{
		"0001_prueba_una.sql": {Data: []byte("CREATE TABLE public.migracion_una (x int)")},
		"0002_prueba_dos.sql": {Data: []byte("CREATE TABLE public.migracion_dos (y int); SELECT * FROM no_existe")},
	}
	_, err := apply(context.Background(), db, fake)
	if err == nil {
		t.Fatal("no se quejó")
	}
	if !strings.Contains(err.Error(), "0002") && !strings.Contains(err.Error(), "prueba_dos") {
		t.Fatalf("no dijo cuál falló: %q", err)
	}
	if got := tables(t, db, "public"); !contains(got, "migracion_una") {
		t.Fatalf("la primera no quedó: %v", got)
	}
	if got := tables(t, db, "public"); contains(got, "migracion_dos") {
		t.Fatalf("la que falló dejó la tabla: %v", got)
	}
	if got := appliedVersions(t, db); len(got) != 1 || got[0] != 1 {
		t.Fatalf("el libro quedó %v", got)
	}
}

func TestAplicaLasMigracionesEnOrden(t *testing.T) {
	db := testDB(t)
	db.Exec("DROP TABLE IF EXISTS public.migracion_una")
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS public.migracion_una") })
	fake := fstest.MapFS{
		"0002_prueba_dos.sql": {Data: []byte("INSERT INTO public.migracion_una VALUES (2)")},
		"0001_prueba_una.sql": {Data: []byte("CREATE TABLE public.migracion_una (x int)")},
	}
	applied, err := apply(context.Background(), db, fake)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied) != 2 || applied[0] != "prueba_una" || applied[1] != "prueba_dos" {
		t.Fatalf("aplicó %v", applied)
	}
	var value int
	if err := db.QueryRow("SELECT x FROM public.migracion_una").Scan(&value); err != nil {
		t.Fatalf("no pude leer: %v", err)
	}
	if value != 2 {
		t.Fatalf("quedó %d", value)
	}
}

func TestDosACorrerAlMismoTiempoNoSePisan(t *testing.T) {
	db := testDB(t)
	results := make([][]string, 2)
	errors := make([]error, 2)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], errors[index] = Apply(context.Background(), db)
		}(index)
	}
	wait.Wait()
	for index, err := range errors {
		if err != nil {
			t.Fatalf("la corrida %d falló: %v", index, err)
		}
	}
	if got := len(results[0]) + len(results[1]); got != len(pending(t)) {
		t.Fatalf("entre las dos aplicaron %d migraciones: %v %v", got, results[0], results[1])
	}
	if got := appliedVersions(t, db); len(got) != len(pending(t)) {
		t.Fatalf("el libro quedó %v", got)
	}
}

func TestUnNombreDeArchivoRaroEsUnError(t *testing.T) {
	cases := []struct {
		name  string
		files fstest.MapFS
	}{
		{"sin número", fstest.MapFS{"heimdall.sql": {Data: []byte("SELECT 1")}}},
		{"sin parte ni tema", fstest.MapFS{"0001.sql": {Data: []byte("SELECT 1")}}},
		{"número repetido", fstest.MapFS{
			"0001_heimdall.sql": {Data: []byte("SELECT 1")},
			"0001_axe.sql":      {Data: []byte("SELECT 1")},
		}},
	}
	for _, caso := range cases {
		t.Run(caso.name, func(t *testing.T) {
			if _, err := apply(context.Background(), testDB(t), caso.files); err == nil {
				t.Fatal("no se quejó")
			}
		})
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestStatusDiceQueFaltaTodoEnUnaBaseCruda(t *testing.T) {
	db := testDB(t)
	known, err := List()
	if err != nil {
		t.Fatal(err)
	}
	states, err := Status(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != len(known) {
		t.Fatalf("el estado trajo %d y el repositorio tiene %d", len(states), len(known))
	}
	for index, state := range states {
		if state.Version != known[index].Version || state.Name != known[index].Name {
			t.Fatalf("el estado %d no es la migración %d", index, known[index].Version)
		}
		if state.Applied {
			t.Fatalf("%04d_%s dice aplicada en una base cruda", state.Version, state.Name)
		}
	}
}

func TestStatusDiceQueCorrioYCuando(t *testing.T) {
	db := testDB(t)
	if _, err := Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	first, err := Status(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range first {
		if !state.Applied {
			t.Fatalf("%04d_%s quedó pendiente después de aplicarla", state.Version, state.Name)
		}
		if state.AppliedAt.IsZero() {
			t.Fatalf("%04d_%s no dice cuándo corrió", state.Version, state.Name)
		}
	}
	applied, err := Apply(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("la segunda corrida aplicó %v", applied)
	}
	second, err := Status(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first) {
		t.Fatalf("el estado cambió: %d contra %d", len(second), len(first))
	}
	for index := range first {
		if !second[index].AppliedAt.Equal(first[index].AppliedAt) {
			t.Fatalf("%04d_%s cambió de fecha", first[index].Version, first[index].Name)
		}
	}
}

func TestTodaTablaSigueLaConvencion(t *testing.T) {
	db := testDB(t)
	if _, err := Apply(context.Background(), db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := []string{"created_at", "deleted_at", "id", "updated_at"}
	rows, err := db.Query(
		`SELECT table_schema, table_name FROM information_schema.tables
         WHERE table_schema NOT IN ('pg_catalog', 'information_schema', 'public')
           AND table_type = 'BASE TABLE'
         ORDER BY table_schema, table_name`)
	if err != nil {
		t.Fatalf("no pude leer las tablas: %v", err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var schema, table string
		if err := rows.Scan(&schema, &table); err != nil {
			t.Fatalf("no pude leer la tabla: %v", err)
		}
		seen++
		names := []string{}
		for _, column := range columnsOf(t, db, schema, table) {
			names = append(names, strings.SplitN(column, ":", 2)[0])
		}
		for _, needed := range want {
			if !slices.Contains(names, needed) {
				t.Fatalf("%s.%s no tiene %s: %v", schema, table, needed, names)
			}
		}
	}
	if seen < len(pending(t)) {
		t.Fatalf("vi %d tablas, menos que migraciones", seen)
	}
}

func TestElIdEsUnUlid(t *testing.T) {
	db := testDB(t)
	if _, err := Apply(context.Background(), db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var ids []string
	rows, err := db.Query("SELECT goddard.ulid() FROM generate_series(1, 1000)")
	if err != nil {
		t.Fatalf("no pude pedir ids: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("no pude leer el id: %v", err)
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		if len(id) != 26 {
			t.Fatalf("el id %q mide %d", id, len(id))
		}
		for _, char := range id {
			if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", char) {
				t.Fatalf("el id %q tiene %q", id, char)
			}
		}
	}
	if len(slices.Compact(slices.Sorted(slices.Values(ids)))) != len(ids) {
		t.Fatal("mil ids y alguno se repitió")
	}
	var first, second string
	if err := db.QueryRow("SELECT goddard.ulid()").Scan(&first); err != nil {
		t.Fatalf("no pude pedir un id: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := db.QueryRow("SELECT goddard.ulid()").Scan(&second); err != nil {
		t.Fatalf("no pude pedir otro id: %v", err)
	}
	if !(first < second) {
		t.Fatalf("%q no es menor que %q", first, second)
	}
}
