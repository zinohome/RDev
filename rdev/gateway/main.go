package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/rdev_002_gateway_init.up.sql
var migrationSQL string

func main() {
	port := os.Getenv("RDEV_GATEWAY_PORT")
	if port == "" {
		port = "8083"
	}

	dbURL := os.Getenv("RDEV_GATEWAY_DB_URL")
	if dbURL == "" {
		log.Fatal("RDEV_GATEWAY_DB_URL not set")
	}

	db, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("connect to DB: %v", err)
	}
	defer db.Close()

	if err := applyMigration(db); err != nil {
		log.Fatalf("apply migration: %v", err)
	}

	mr, err := NewModelRouter()
	if err != nil {
		log.Fatalf("init model router: %v", err)
	}

	h := newServer(db, mr)
	log.Printf("gateway listening on :%s", port)
	if err := http.ListenAndServe(":"+port, h); err != nil {
		log.Fatal(err)
	}
}

func applyMigration(db *pgxpool.Pool) error {
	_, err := db.Exec(context.Background(), migrationSQL)
	return err
}
