package seed

// SkillYAML mirrors the YAML structure of files under data/skills/.
type SkillYAML struct {
	ID          string         `yaml:"id"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Content     string         `yaml:"content"`
	Config      map[string]any `yaml:"config"`
}

// AgentYAML mirrors the YAML structure of files under data/agents/.
// Agents are loaded as reference templates; they are not inserted into the
// database directly because agent rows require a runtime_id (NOT NULL FK
// to agent_runtime). Callers can read AgentDefs() to bootstrap agents
// programmatically when a runtime is available.
type AgentYAML struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	Instructions  string   `yaml:"instructions"`
	AvatarEmoji   string   `yaml:"avatar_emoji"`
	Tier          string   `yaml:"tier"`
	DefaultSkills []string `yaml:"default_skills"`
}
