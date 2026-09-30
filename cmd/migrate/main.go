// Command migrate is the schema of goddard, for whoever operates the service.
// It is not part of the user facing binary: nobody who asks for a skill should
// be able to change the database by doing it.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/3-lines-studio/goddard/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	ctx := context.Background()
	var err error
	switch {
	case len(os.Args) == 1:
		err = apply(ctx)
	case os.Args[1] == "status":
		err = status(ctx)
	case os.Args[1] == "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "migrate: no conozco «%s»\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func apply(ctx context.Context) error {
	db, err := database(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	applied, err := migrations.Apply(ctx, db)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		fmt.Println("no había ninguna pendiente")
		return nil
	}
	for _, name := range applied {
		fmt.Println("aplicada " + name)
	}
	return nil
}

func status(ctx context.Context) error {
	db, err := database(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	states, err := migrations.Status(ctx, db)
	if err != nil {
		return err
	}
	for _, state := range states {
		line := fmt.Sprintf("%04d_%s", state.Version, state.Name)
		if state.Applied {
			fmt.Printf("%-20s aplicada %s\n", line, state.AppliedAt.Format("2006-01-02 15:04"))
			continue
		}
		fmt.Printf("%-20s pendiente\n", line)
	}
	return nil
}

func database(ctx context.Context) (*sql.DB, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, errors.New("falta DATABASE_URL")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func usage() {
	fmt.Print(`migrate — el esquema de goddard, para el que opera el servicio

  migrate            aplica lo que falte
  migrate status     qué corrió y qué no

  DATABASE_URL    el Postgres compartido
`)
}
