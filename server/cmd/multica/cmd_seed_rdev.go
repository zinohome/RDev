package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	rdevseed "github.com/zinohome/RDev/rdev/seed"
)

func init() {
	RegisterSeedBundle("rdev", &rdevSeedBundle{})
}

// rdevSeedBundle seeds preset skills and agent templates for an RDev workspace.
// It requires DATABASE_URL and MULTICA_WORKSPACE_ID to be set in the environment.
type rdevSeedBundle struct{}

func (b *rdevSeedBundle) Run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("rdev seed: DATABASE_URL not set (direct DB access required for seed)")
	}

	workspaceID := os.Getenv("MULTICA_WORKSPACE_ID")
	if workspaceID == "" {
		return fmt.Errorf("rdev seed: MULTICA_WORKSPACE_ID not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("rdev seed: connect to DB: %w", err)
	}
	defer pool.Close()

	return rdevseed.New(pool).Load(ctx, workspaceID)
}
