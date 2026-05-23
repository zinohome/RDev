package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// SeedBundle is the interface that seed bundle implementations must satisfy.
type SeedBundle interface {
	Run() error
}

var seedBundles = map[string]SeedBundle{}

// RegisterSeedBundle registers a named seed bundle.
// Called by rdev/seed package in init() to register the "rdev" bundle.
func RegisterSeedBundle(name string, b SeedBundle) {
	seedBundles[name] = b
}

var cmdSeed = &cobra.Command{
	Use:   "seed",
	Short: "Run seed data bundles to initialize workspace defaults",
	Long: `Run seed data bundles to initialize workspace defaults.
Seed operations are idempotent — safe to run multiple times.

Example:
  multica seed --bundle=rdev`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bundle, _ := cmd.Flags().GetString("bundle")
		if bundle == "" {
			return fmt.Errorf("--bundle is required (e.g. --bundle=rdev)\n\nRegistered bundles: %v", registeredBundleNames())
		}
		b, ok := seedBundles[bundle]
		if !ok {
			return fmt.Errorf("unknown bundle %q (registered: %v)", bundle, registeredBundleNames())
		}
		fmt.Printf("Running seed bundle: %s\n", bundle)
		return b.Run()
	},
}

func init() {
	cmdSeed.Flags().String("bundle", "", "Seed bundle name (e.g. rdev)")
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
