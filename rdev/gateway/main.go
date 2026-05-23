package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	port := os.Getenv("RDEV_GATEWAY_PORT")
	if port == "" {
		port = "8083"
	}

	var db *pgxpool.Pool
	if dbURL := os.Getenv("RDEV_GATEWAY_DB_URL"); dbURL != "" {
		var err error
		db, err = pgxpool.New(context.Background(), dbURL)
		if err != nil {
			log.Fatalf("connect to DB: %v", err)
		}
		defer db.Close()
	} else {
		log.Println("warn: RDEV_GATEWAY_DB_URL not set, auth will reject all requests unless RDEV_GATEWAY_NO_AUTH=1")
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
