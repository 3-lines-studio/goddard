package migrations

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

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
	_, err := db.Exec("DROP SCHEMA IF EXISTS heimdall CASCADE; DROP TABLE IF EXISTS public.schema_migrations")
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
	want := []string{"audit", "environments", "logins", "secrets", "sessions", "tokens"}
	got := strings.Join(tables(t, db, "heimdall"), ",")
	if got != strings.Join(want, ",") {
		t.Fatalf("las tablas quedaron %s", got)
	}
	wanted := "project:text,env:text,name:text,value:bytea,updated_at:bigint"
	if got := columnsOf(t, db, "heimdall", "secrets"); strings.Join(got, ",") != wanted {
		t.Fatalf("heimdall.secrets quedó %v", got)
	}
	wantedTokens := "id:text,name:text,project:text,env:text,keys:jsonb,hash:text,role:text,created_at:bigint,expires_at:bigint,last_used:bigint"
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
	wantedSessions := "id:text,scope:text,title:text,turns:integer,updated_at:bigint,archived_at:bigint,seq:bigint"
	if got := columnsOf(t, db, "axe", "sessions"); strings.Join(got, ",") != wantedSessions {
		t.Fatalf("axe.sessions quedó %v", got)
	}
	wantedEntries := "session_id:text,seq:bigint,entry:jsonb"
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
			`INSERT INTO heimdall.tokens (id, name, project, env, keys, hash, role, created_at)
             VALUES ($1, $1, 'bifrost', 'dev', $2, 'hash', $3, 0)`, id, keys, role)
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
		`INSERT INTO heimdall.tokens (id, name, project, env, hash, created_at) VALUES ('t5', 'sin rol', 'bifrost', 'dev', 'hash', 0)`); err != nil {
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
