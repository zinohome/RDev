package audit

import (
	"context"
	"testing"
)

type testSink struct {
	events []Event
}

func (s *testSink) Write(_ context.Context, e Event) error {
	s.events = append(s.events, e)
	return nil
}

func TestEmit(t *testing.T) {
	sinks = nil

	ts := &testSink{}
	RegisterAuditSink(ts)

	Emit(context.Background(), Event{
		WorkspaceID: "ws-1",
		Action:      "GET /api/issues",
	})

	if len(ts.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(ts.events))
	}
	if ts.events[0].WorkspaceID != "ws-1" {
		t.Errorf("wrong workspace ID: %s", ts.events[0].WorkspaceID)
	}
}

func TestEmit_NoSinks(t *testing.T) {
	sinks = nil
	// Should not panic when no sinks registered
	Emit(context.Background(), Event{WorkspaceID: "ws-1", Action: "test"})
}

func TestEmit_MultipleSinks(t *testing.T) {
	sinks = nil

	ts1 := &testSink{}
	ts2 := &testSink{}
	RegisterAuditSink(ts1)
	RegisterAuditSink(ts2)

	Emit(context.Background(), Event{WorkspaceID: "ws-2", Action: "DELETE /api/something"})

	if len(ts1.events) != 1 || len(ts2.events) != 1 {
		t.Errorf("both sinks should receive the event: ts1=%d ts2=%d", len(ts1.events), len(ts2.events))
	}
}
