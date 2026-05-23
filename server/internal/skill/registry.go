package skill

// SkillDef defines a preset (builtin) skill. Fields align with the multica DB skills table.
type SkillDef struct {
	ID           string
	Name         string
	Description  string
	Instructions string
	IsBuiltin    bool
}

var builtinSkills []SkillDef

// RegisterBuiltinSkills batch-registers builtin skill definitions.
// Duplicate IDs are silently skipped (idempotent).
func RegisterBuiltinSkills(skills []SkillDef) {
	existing := map[string]bool{}
	for _, s := range builtinSkills {
		existing[s.ID] = true
	}
	for _, s := range skills {
		if !existing[s.ID] {
			builtinSkills = append(builtinSkills, s)
			existing[s.ID] = true
		}
	}
}

// BuiltinSkills returns a copy of all registered builtin skills.
func BuiltinSkills() []SkillDef {
	result := make([]SkillDef, len(builtinSkills))
	copy(result, builtinSkills)
	return result
}
