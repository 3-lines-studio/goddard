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
	if len(applied) != 1 || applied[0] != "heimdall" {
		t.Fatalf("aplicó %v", applied)
	}
	want := []string{"audit", "environments", "logins", "secrets", "sessions", "tokens"}
	got := strings.Join(tables(t, db, "heimdall"), ",")
	if got != strings.Join(want, ",") {
		t.Fatalf("las tablas quedaron %s", got)
	}
	columns := []string{}
	rows, err := db.Query(
		`SELECT column_name || ':' || data_type FROM information_schema.columns
         WHERE table_schema = 'heimdall' AND table_name = 'secrets' ORDER BY ordinal_position`)
	if err != nil {
		t.Fatalf("no pude leer las columnas: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("no pude leer las columnas: %v", err)
		}
		columns = append(columns, column)
	}
	wanted := "project:text,env:text,name:text,value:bytea,updated_at:bigint"
	if strings.Join(columns, ",") != wanted {
		t.Fatalf("heimdall.secrets quedó %v", columns)
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
	if got := appliedVersions(t, db); len(got) != 1 {
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
	if got := len(results[0]) + len(results[1]); got != 1 {
		t.Fatalf("entre las dos aplicaron %d migraciones: %v %v", got, results[0], results[1])
	}
	if got := appliedVersions(t, db); len(got) != 1 {
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
