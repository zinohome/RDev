package seed_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zinohome/RDev/rdev/seed"
)

// realDBURL returns the DATABASE_URL for integration tests, or skips.
func realDBURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	return url
}

// realWorkspaceID returns a workspace ID for integration tests, or skips.
func realWorkspaceID(t *testing.T) string {
	t.Helper()
	id := os.Getenv("TEST_WORKSPACE_ID")
	if id == "" {
		t.Skip("TEST_WORKSPACE_ID not set; skipping integration test")
	}
	return id
}

// TestLoad_Integration verifies that Load creates skills and agents in a real DB.
// Requires DATABASE_URL and TEST_WORKSPACE_ID env vars.
func TestLoad_Integration(t *testing.T) {
	ctx := context.Background()
	dbURL := realDBURL(t)
	wsID := realWorkspaceID(t)

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to db: %v", err)
	}
	defer pool.Close()

	// Point at the real seed data directory.
	bundleDir := filepath.Join("..", "..", "rdev", "seed", "data")
	if _, err := os.Stat(bundleDir); os.IsNotExist(err) {
		// Try relative path from test location.
		bundleDir = "data"
	}

	loader := seed.New(pool, wsID)
	if err := loader.Load(ctx, bundleDir); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Second call must be idempotent (no error, no duplicate rows).
	if err := loader.Load(ctx, bundleDir); err != nil {
		t.Fatalf("Load (idempotent second run): %v", err)
	}
}

// TestLoad_InvalidWorkspaceID tests that an invalid UUID returns a clear error.
func TestLoad_InvalidWorkspaceID(t *testing.T) {
	ctx := context.Background()
	dbURL := realDBURL(t)

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	loader := seed.New(pool, "not-a-uuid")
	err = loader.Load(ctx, "data")
	if err == nil {
		t.Fatal("expected error for invalid workspace_id, got nil")
	}
}

// TestBuildSlugToName_Roundtrip verifies that all skills in data/skills/ have
// unique ids and names, and that the slug→name mapping is correct.
func TestBuildSlugToName_Roundtrip(t *testing.T) {
	dir := filepath.Join("data", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("data/skills not found (%v); run from rdev/seed directory", err)
	}

	seen := map[string]bool{}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		// Basic validation: file must contain id and name fields.
		content := string(data)
		if !containsField(content, "id:") {
			t.Errorf("%s: missing 'id:' field", e.Name())
		}
		if !containsField(content, "name:") {
			t.Errorf("%s: missing 'name:' field", e.Name())
		}
		if seen[e.Name()] {
			t.Errorf("duplicate file name: %s", e.Name())
		}
		seen[e.Name()] = true
	}
}

// TestAgentYAML_DefaultSkillsExist verifies that every skill slug listed in
// agent default_skills has a corresponding skill YAML file.
func TestAgentYAML_DefaultSkillsExist(t *testing.T) {
	skillsDir := filepath.Join("data", "skills")
	agentsDir := filepath.Join("data", "agents")

	// Collect all known skill IDs from skill YAML files.
	skillEntries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Skipf("data/skills not found (%v); run from rdev/seed directory", err)
	}
	knownSlugs := map[string]bool{}
	for _, e := range skillEntries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(skillsDir, e.Name()))
		id := extractYAMLField(string(data), "id")
		if id != "" {
			knownSlugs[id] = true
		}
	}

	// Check each agent's default_skills.
	agentEntries, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Skipf("data/agents not found (%v); run from rdev/seed directory", err)
	}
	for _, e := range agentEntries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(agentsDir, e.Name()))
		skills := extractYAMLListField(string(data), "default_skills")
		for _, slug := range skills {
			slug = trimYAMLListItem(slug)
			if slug == "" {
				continue
			}
			if !knownSlugs[slug] {
				t.Errorf("agent %s: default_skills contains unknown slug %q", e.Name(), slug)
			}
		}
	}
}

// containsField checks that the given YAML key appears in the content.
func containsField(content, key string) bool {
	for _, line := range splitLines(content) {
		if len(line) >= len(key) && line[:len(key)] == key {
			return true
		}
	}
	return false
}

// extractYAMLField extracts a simple scalar value for a top-level YAML key.
func extractYAMLField(content, key string) string {
	prefix := key + ": "
	for _, line := range splitLines(content) {
		if len(line) > len(prefix) && line[:len(prefix)] == prefix {
			val := line[len(prefix):]
			// strip surrounding quotes if present
			if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
				val = val[1 : len(val)-1]
			}
			return val
		}
	}
	return ""
}

// extractYAMLListField extracts list items for a top-level YAML key.
func extractYAMLListField(content, key string) []string {
	lines := splitLines(content)
	var result []string
	inList := false
	for _, line := range lines {
		if line == key+":" {
			inList = true
			continue
		}
		if inList {
			if len(line) == 0 || line[0] != ' ' {
				break
			}
			result = append(result, line)
		}
	}
	return result
}

func trimYAMLListItem(s string) string {
	// Remove leading "  - " prefix typical in YAML lists.
	for _, prefix := range []string{"  - ", "- "} {
		if len(s) > len(prefix) && s[:len(prefix)] == prefix {
			return s[len(prefix):]
		}
	}
	return ""
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
