package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/zinohome/RDev/rdev/seed"
)

// SeedBundle is the interface that seed bundle implementations must satisfy.
type SeedBundle interface {
	Run() error
}

var seedBundles = map[string]SeedBundle{}

// RegisterSeedBundle registers a named seed bundle.
func RegisterSeedBundle(name string, b SeedBundle) {
	seedBundles[name] = b
}

var cmdSeed = &cobra.Command{
	Use:   "seed",
	Short: "Run seed data bundles to initialize workspace defaults",
	Long: `Run seed data bundles to initialize workspace defaults.
Seed operations are idempotent — safe to run multiple times.

Requires DATABASE_URL and MULTICA_WORKSPACE_ID (or --workspace-id flag).

Example:
  DATABASE_URL=postgres://... MULTICA_WORKSPACE_ID=<uuid> multica seed --bundle=rdev`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bundle, _ := cmd.Flags().GetString("bundle")
		if bundle == "" {
			return fmt.Errorf("--bundle is required (e.g. --bundle=rdev)\n\nRegistered bundles: rdev%s", registeredSuffix())
		}

		// rdev bundle is handled directly via DB access.
		if bundle == "rdev" {
			return runRdevSeed(cmd)
		}

		b, ok := seedBundles[bundle]
		if !ok {
			return fmt.Errorf("unknown bundle %q (registered: rdev%s)", bundle, registeredSuffix())
		}
		fmt.Printf("Running seed bundle: %s\n", bundle)
		return b.Run()
	},
}

// runRdevSeed seeds the rdev bundle directly via pgx DB access.
// Reads DATABASE_URL and workspace-id from flags/environment.
func runRdevSeed(cmd *cobra.Command) error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is required for the rdev bundle")
	}

	workspaceID, _ := cmd.Flags().GetString("workspace-id")
	if workspaceID == "" {
		workspaceID = os.Getenv("MULTICA_WORKSPACE_ID")
	}
	if workspaceID == "" {
		return fmt.Errorf("--workspace-id flag or MULTICA_WORKSPACE_ID env var is required")
	}

	bundleDir, _ := cmd.Flags().GetString("data-dir")
	if bundleDir == "" {
		bundleDir = "./rdev/seed/data"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	fmt.Printf("Running seed bundle: rdev (workspace=%s, data=%s)\n", workspaceID, bundleDir)
	loader := seed.New(pool, workspaceID)
	return loader.Load(ctx, bundleDir)
}

func init() {
	cmdSeed.Flags().String("bundle", "", "Seed bundle name (e.g. rdev)")
	cmdSeed.Flags().String("workspace-id", "", "Target workspace UUID (env: MULTICA_WORKSPACE_ID)")
	cmdSeed.Flags().String("data-dir", "", "Path to seed data directory (default: ./rdev/seed/data)")
	rootCmd.AddCommand(cmdSeed)
}

func registeredBundleNames() []string {
	names := make([]string, 0, len(seedBundles))
	for k := range seedBundles {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// registeredSuffix returns extra registered bundle names for error messages.
func registeredSuffix() string {
	names := registeredBundleNames()
	if len(names) == 0 {
		return ""
	}
	return ", " + fmt.Sprintf("%v", names)
}
