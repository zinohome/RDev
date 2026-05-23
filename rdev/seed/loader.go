package seed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

// skillDef is the parsed form of a skills/*.yaml file.
type skillDef struct {
	ID          string         `yaml:"id"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Content     string         `yaml:"content"`
	Config      map[string]any `yaml:"config"`
}

// agentDef is the parsed form of an agents/*.yaml file.
type agentDef struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	Instructions  string   `yaml:"instructions"`
	Visibility    string   `yaml:"visibility"`
	DefaultSkills []string `yaml:"default_skills"`
	Model         string   `yaml:"model"`
}

// Loader reads YAML seed files and writes them to the database idempotently.
type Loader struct {
	db          *pgxpool.Pool
	workspaceID string
}

// New creates a Loader targeting the given workspace.
func New(db *pgxpool.Pool, workspaceID string) *Loader {
	return &Loader{db: db, workspaceID: workspaceID}
}

// Load reads all YAML files under bundleDir/skills and bundleDir/agents and
// inserts any that do not already exist in the workspace (idempotent).
func (l *Loader) Load(ctx context.Context, bundleDir string) error {
	wsUUID, err := parseUUID(l.workspaceID)
	if err != nil {
		return fmt.Errorf("invalid workspace_id %q: %w", l.workspaceID, err)
	}

	if err := l.loadSkills(ctx, wsUUID, filepath.Join(bundleDir, "skills")); err != nil {
		return fmt.Errorf("load skills: %w", err)
	}
	if err := l.loadAgents(ctx, wsUUID, bundleDir); err != nil {
		return fmt.Errorf("load agents: %w", err)
	}
	return nil
}

func (l *Loader) loadSkills(ctx context.Context, wsUUID pgtype.UUID, dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return err
	}
	for _, f := range files {
		var def skillDef
		if err := parseYAML(f, &def); err != nil {
			return fmt.Errorf("parse %s: %w", f, err)
		}
		if err := l.upsertSkill(ctx, wsUUID, def); err != nil {
			return fmt.Errorf("upsert skill %q: %w", def.Name, err)
		}
	}
	return nil
}

func (l *Loader) upsertSkill(ctx context.Context, wsUUID pgtype.UUID, def skillDef) error {
	_, err := l.db.Exec(ctx, `
		INSERT INTO skill (workspace_id, name, description, content, config, created_by)
		VALUES ($1, $2, $3, $4, NULL, NULL)
		ON CONFLICT (workspace_id, name) DO NOTHING
	`, wsUUID, def.Name, def.Description, def.Content)
	return err
}

// loadAgents seeds agents and wires up their default skills.
// Skills are matched by YAML "id" slug → skill name → DB UUID.
func (l *Loader) loadAgents(ctx context.Context, wsUUID pgtype.UUID, bundleDir string) error {
	// Build slug→name map from skill YAML files so agent default_skills slugs
	// (e.g. "archon-workflow") can be resolved to DB skill names ("Archon Workflow").
	slugToName, err := buildSlugToName(filepath.Join(bundleDir, "skills"))
	if err != nil {
		return fmt.Errorf("build slug→name map: %w", err)
	}

	// Fetch current DB skill name→id for this workspace.
	skillIDs, err := l.skillIDsByName(ctx, wsUUID)
	if err != nil {
		return fmt.Errorf("fetch skill ids: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(bundleDir, "agents", "*.yaml"))
	if err != nil {
		return err
	}
	for _, f := range files {
		var def agentDef
		if err := parseYAML(f, &def); err != nil {
			return fmt.Errorf("parse %s: %w", f, err)
		}
		agentID, err := l.upsertAgent(ctx, wsUUID, def)
		if err != nil {
			return fmt.Errorf("upsert agent %q: %w", def.Name, err)
		}
		if agentID == "" {
			continue // already existed, skip skill attachment
		}
		if err := l.attachSkills(ctx, agentID, def.DefaultSkills, slugToName, skillIDs); err != nil {
			return fmt.Errorf("attach skills for agent %q: %w", def.Name, err)
		}
	}
	return nil
}

// upsertAgent inserts the agent if it does not already exist (matched by name
// in workspace). Returns the new agent's UUID string, or "" if it already existed.
func (l *Loader) upsertAgent(ctx context.Context, wsUUID pgtype.UUID, def agentDef) (string, error) {
	visibility := def.Visibility
	if visibility == "" {
		visibility = "workspace"
	}

	var agentID pgtype.UUID
	err := l.db.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, instructions,
			avatar_url, runtime_mode, runtime_config, runtime_id,
			visibility, max_concurrent_tasks, owner_id,
			custom_env, custom_args, mcp_config, model, thinking_level
		) VALUES (
			$1, $2, $3, $4,
			NULL, 'local', '{}', NULL,
			$5, 1, NULL,
			NULL, NULL, NULL, NULLIF($6, ''), NULL
		)
		ON CONFLICT (workspace_id, name) DO NOTHING
		RETURNING id
	`, wsUUID, def.Name, def.Description, def.Instructions, visibility, def.Model,
	).Scan(&agentID)

	if err == pgx.ErrNoRows {
		return "", nil // ON CONFLICT DO NOTHING: agent already exists
	}
	if err != nil {
		return "", err
	}
	return uuidToString(agentID), nil
}

// attachSkills wires default_skills slugs (e.g. "archon-workflow") to the agent.
// Slugs that don't resolve to a known skill in this workspace are silently skipped.
func (l *Loader) attachSkills(ctx context.Context, agentID string, slugs []string, slugToName map[string]string, skillIDs map[string]string) error {
	aUUID, err := parseUUID(agentID)
	if err != nil {
		return err
	}
	for _, slug := range slugs {
		name, ok := slugToName[slug]
		if !ok {
			continue // unknown slug, skip
		}
		sID, ok := skillIDs[name]
		if !ok {
			continue // skill not yet seeded in this workspace, skip
		}
		sUUID, err := parseUUID(sID)
		if err != nil {
			return err
		}
		if _, err := l.db.Exec(ctx,
			`INSERT INTO agent_skill (agent_id, skill_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			aUUID, sUUID,
		); err != nil {
			return err
		}
	}
	return nil
}

// skillIDsByName returns a map of skill name → UUID string for the workspace.
func (l *Loader) skillIDsByName(ctx context.Context, wsUUID pgtype.UUID) (map[string]string, error) {
	rows, err := l.db.Query(ctx, `SELECT id, name FROM skill WHERE workspace_id = $1`, wsUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var id pgtype.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		m[name] = uuidToString(id)
	}
	return m, rows.Err()
}

// buildSlugToName reads skill YAML files and returns a map from skill "id"
// slug to skill display name (e.g. "archon-workflow" → "Archon Workflow").
func buildSlugToName(dir string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(files))
	for _, f := range files {
		var def skillDef
		if err := parseYAML(f, &def); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		m[def.ID] = def.Name
	}
	return m, nil
}

// parseYAML reads a YAML file and decodes it into dst.
func parseYAML(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, dst)
}

// parseUUID converts a UUID string to pgtype.UUID.
func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return u, err
	}
	return u, nil
}

// uuidToString converts a pgtype.UUID to its canonical hyphenated string form.
func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
