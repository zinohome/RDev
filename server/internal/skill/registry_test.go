package skill

import "testing"

func TestRegisterBuiltinSkills_Idempotent(t *testing.T) {
	builtinSkills = nil

	RegisterBuiltinSkills([]SkillDef{
		{ID: "test-skill", Name: "Test Skill"},
	})
	RegisterBuiltinSkills([]SkillDef{
		{ID: "test-skill", Name: "Test Skill Duplicate"},
	})

	skills := BuiltinSkills()
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "Test Skill" {
		t.Errorf("first registration should win, got: %s", skills[0].Name)
	}
}

func TestBuiltinSkills_ReturnsCopy(t *testing.T) {
	builtinSkills = nil
	RegisterBuiltinSkills([]SkillDef{{ID: "s1", Name: "S1"}})

	skills := BuiltinSkills()
	skills[0].Name = "MUTATED"

	skills2 := BuiltinSkills()
	if skills2[0].Name == "MUTATED" {
		t.Error("BuiltinSkills should return a copy, not a reference")
	}
}
