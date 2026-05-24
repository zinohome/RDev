package seed

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

//go:embed data
var dataFS embed.FS

// querier abstracts pgxpool.Pool for testability.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Loader reads seed YAML files and writes preset data to the database.
// All operations are idempotent: existing rows (matched by workspace_id + name)
// are left unchanged.
type Loader struct {
	db querier
}

// New creates a Loader backed by the given connection pool.
func New(db *pgxpool.Pool) *Loader {
	return &Loader{db: db}
}

// newWithQuerier is used by tests to inject a mock querier.
func newWithQuerier(q querier) *Loader {
	return &Loader{db: q}
}

// Load seeds all skills from data/skills/ into the given workspace.
// Agent definitions are parsed and logged for reference but are not inserted
// (agents require a runtime_id that must be provided by the operator).
// Safe to call multiple times — already-existing skills are skipped.
func (l *Loader) Load(ctx context.Context, workspaceID string) error {
	if workspaceID == "" {
		return fmt.Errorf("seed: workspaceID must not be empty")
	}

	skills, err := l.parseSkills()
	if err != nil {
		return fmt.Errorf("seed: parse skills: %w", err)
	}

	if err := l.insertSkills(ctx, workspaceID, skills); err != nil {
		return fmt.Errorf("seed: insert skills: %w", err)
	}

	agents, err := l.parseAgents()
	if err != nil {
		return fmt.Errorf("seed: parse agents: %w", err)
	}

	slog.Info("seed: agent templates available (not inserted — operator must create with runtime_id)",
		"count", len(agents),
		"agents", agentNames(agents),
	)

	return nil
}

// SkillDefs returns all skill definitions from the embedded data directory.
// Useful for introspection without a database connection.
func SkillDefs() ([]SkillYAML, error) {
	l := &Loader{}
	return l.parseSkills()
}

// AgentDefs returns all agent definitions from the embedded data directory.
func AgentDefs() ([]AgentYAML, error) {
	l := &Loader{}
	return l.parseAgents()
}

func (l *Loader) parseSkills() ([]SkillYAML, error) {
	return parseYAMLDir[SkillYAML](dataFS, "data/skills")
}

func (l *Loader) parseAgents() ([]AgentYAML, error) {
	return parseYAMLDir[AgentYAML](dataFS, "data/agents")
}

func (l *Loader) insertSkills(ctx context.Context, workspaceID string, skills []SkillYAML) error {
	inserted, skipped := 0, 0
	for _, s := range skills {
		if err := validateSkill(s); err != nil {
			return fmt.Errorf("invalid skill %q: %w", s.ID, err)
		}

		configJSON, err := marshalConfig(s.Config)
		if err != nil {
			return fmt.Errorf("skill %q: marshal config: %w", s.ID, err)
		}

		tag, err := l.db.Exec(ctx, `
			INSERT INTO skill (workspace_id, name, description, content, config)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (workspace_id, name) DO NOTHING`,
			workspaceID, s.Name, s.Description, s.Content, configJSON,
		)
		if err != nil {
			return fmt.Errorf("insert skill %q: %w", s.Name, err)
		}
		if tag.RowsAffected() > 0 {
			inserted++
			slog.Info("seed: inserted skill", "name", s.Name)
		} else {
			skipped++
			slog.Debug("seed: skill already exists, skipped", "name", s.Name)
		}
	}
	slog.Info("seed: skills done", "inserted", inserted, "skipped", skipped)
	return nil
}

// parseYAMLDir reads all *.yaml files from the given embedded directory
// and unmarshals them into T.
func parseYAMLDir[T any](fsys embed.FS, dir string) ([]T, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}

	var results []T
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var v T
		if err := yaml.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		results = append(results, v)
	}
	return results, nil
}

func validateSkill(s SkillYAML) error {
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	if s.Content == "" {
		return fmt.Errorf("content is required")
	}
	return nil
}

func marshalConfig(cfg map[string]any) ([]byte, error) {
	if len(cfg) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(cfg)
}

func agentNames(agents []AgentYAML) []string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	return names
}
