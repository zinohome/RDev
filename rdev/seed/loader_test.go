package seed

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// mockQuerier records Exec calls for assertion in tests.
type mockQuerier struct {
	execCalls []execCall
	execErr   error
	// rowsAffected controls affected-row count per call index.
	// If shorter than the number of calls, the last entry is reused.
	rowsAffected []int64
}

type execCall struct {
	sql  string
	args []any
}

func (m *mockQuerier) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execErr != nil {
		return pgconn.CommandTag{}, m.execErr
	}
	m.execCalls = append(m.execCalls, execCall{sql: sql, args: args})

	affected := int64(1)
	idx := len(m.execCalls) - 1
	if idx < len(m.rowsAffected) {
		affected = m.rowsAffected[idx]
	} else if len(m.rowsAffected) > 0 {
		affected = m.rowsAffected[len(m.rowsAffected)-1]
	}

	var tag pgconn.CommandTag
	if affected > 0 {
		tag = pgconn.NewCommandTag("INSERT 0 1")
	} else {
		tag = pgconn.NewCommandTag("INSERT 0 0")
	}
	return tag, nil
}

func (m *mockQuerier) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	panic("QueryRow not implemented in mockQuerier")
}

func TestLoad_InsertsSkills(t *testing.T) {
	q := &mockQuerier{}
	l := newWithQuerier(q)

	err := l.Load(context.Background(), "workspace-123")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	skills, _ := SkillDefs()
	if len(q.execCalls) != len(skills) {
		t.Errorf("expected %d Exec calls (one per skill), got %d", len(skills), len(q.execCalls))
	}

	for i, call := range q.execCalls {
		if !strings.Contains(call.sql, "INSERT INTO skill") {
			t.Errorf("execCalls[%d]: expected INSERT INTO skill, got: %s", i, call.sql)
		}
		if !strings.Contains(call.sql, "ON CONFLICT") {
			t.Errorf("execCalls[%d]: missing ON CONFLICT clause", i)
		}
		if len(call.args) < 4 {
			t.Errorf("execCalls[%d]: expected at least 4 args, got %d", i, len(call.args))
		}
		if call.args[0] != "workspace-123" {
			t.Errorf("execCalls[%d]: args[0] = %v, want workspace-123", i, call.args[0])
		}
	}
}

func TestLoad_Idempotent(t *testing.T) {
	// Simulate all rows already existing (ON CONFLICT, 0 rows affected).
	q := &mockQuerier{rowsAffected: []int64{0}}
	l := newWithQuerier(q)

	if err := l.Load(context.Background(), "workspace-abc"); err != nil {
		t.Fatalf("first Load() error = %v", err)
	}
	firstCount := len(q.execCalls)

	// Second run must also succeed.
	if err := l.Load(context.Background(), "workspace-abc"); err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	if len(q.execCalls) != 2*firstCount {
		t.Errorf("expected %d total Exec calls after two runs, got %d", 2*firstCount, len(q.execCalls))
	}
}

func TestLoad_EmptyWorkspaceID(t *testing.T) {
	q := &mockQuerier{}
	l := newWithQuerier(q)

	if err := l.Load(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty workspaceID, got nil")
	}
	if len(q.execCalls) != 0 {
		t.Errorf("expected no DB calls for empty workspaceID, got %d", len(q.execCalls))
	}
}

func TestLoad_DBError(t *testing.T) {
	dbErr := errors.New("connection refused")
	q := &mockQuerier{execErr: dbErr}
	l := newWithQuerier(q)

	err := l.Load(context.Background(), "workspace-xyz")
	if err == nil {
		t.Fatal("expected error when DB fails, got nil")
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("error chain should wrap the DB error; got: %v", err)
	}
}

func TestSkillDefs_ParsesAllFiles(t *testing.T) {
	skills, err := SkillDefs()
	if err != nil {
		t.Fatalf("SkillDefs() error = %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("SkillDefs() returned no skills")
	}

	wantIDs := []string{
		"archon-workflow",
		"gitea-pr-helper",
		"code-review-internal",
		"frontend-design",
		"shadcn",
	}
	ids := make(map[string]bool, len(skills))
	for _, s := range skills {
		ids[s.ID] = true
	}
	for _, want := range wantIDs {
		if !ids[want] {
			t.Errorf("expected skill %q not found in SkillDefs()", want)
		}
	}
}

func TestAgentDefs_ParsesAllFiles(t *testing.T) {
	agents, err := AgentDefs()
	if err != nil {
		t.Fatalf("AgentDefs() error = %v", err)
	}
	if len(agents) == 0 {
		t.Fatal("AgentDefs() returned no agents")
	}

	wantIDs := []string{
		"rdev-omnipotent",
		"rdev-fullstack",
		"rdev-devops",
		"rdev-product",
		"rdev-codereview",
		"rdev-qa",
	}
	ids := make(map[string]bool, len(agents))
	for _, a := range agents {
		ids[a.ID] = true
	}
	for _, want := range wantIDs {
		if !ids[want] {
			t.Errorf("expected agent %q not found in AgentDefs()", want)
		}
	}
}

func TestSkillDefs_RequiredFieldsPresent(t *testing.T) {
	skills, err := SkillDefs()
	if err != nil {
		t.Fatalf("SkillDefs() error = %v", err)
	}
	for _, s := range skills {
		if s.ID == "" {
			t.Errorf("skill %+v: empty id", s)
		}
		if s.Name == "" {
			t.Errorf("skill %q: empty name", s.ID)
		}
		if s.Description == "" {
			t.Errorf("skill %q: empty description", s.ID)
		}
		if s.Content == "" {
			t.Errorf("skill %q: empty content", s.ID)
		}
	}
}

func TestAgentDefs_RequiredFieldsPresent(t *testing.T) {
	agents, err := AgentDefs()
	if err != nil {
		t.Fatalf("AgentDefs() error = %v", err)
	}
	for _, a := range agents {
		if a.ID == "" {
			t.Errorf("agent %+v: empty id", a)
		}
		if a.Name == "" {
			t.Errorf("agent %q: empty name", a.ID)
		}
		if a.Description == "" {
			t.Errorf("agent %q: empty description", a.ID)
		}
		if a.Instructions == "" {
			t.Errorf("agent %q: empty instructions", a.ID)
		}
	}
}
