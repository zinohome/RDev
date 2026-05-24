package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// mockSink is a test double that records Write calls and returns preset Query results.
type mockSink struct {
	written []Event
	results []Event
	writeErr error
	queryErr error
}

func (m *mockSink) Write(_ context.Context, e Event) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.written = append(m.written, e)
	return nil
}

func (m *mockSink) Query(_ context.Context, _ QueryParams) ([]Event, error) {
	return m.results, m.queryErr
}

func resetSink(t *testing.T, s Sink) {
	t.Helper()
	prev := globalSink
	globalSink = s
	t.Cleanup(func() { globalSink = prev })
}

// ── RegisterSink / Record ────────────────────────────────────────────────────

func TestRecord_NoSink(t *testing.T) {
	resetSink(t, nil)
	// Must not panic.
	Record(context.Background(), Event{Action: "test"})
}

func TestRecord_WithSink(t *testing.T) {
	ms := &mockSink{}
	resetSink(t, ms)

	e := Event{
		ID:     uuid.New(),
		Action: "agent.task.started",
	}
	Record(context.Background(), e)

	if len(ms.written) != 1 {
		t.Fatalf("expected 1 event, got %d", len(ms.written))
	}
	if ms.written[0].Action != e.Action {
		t.Errorf("action mismatch: got %q", ms.written[0].Action)
	}
}

// ── Middleware ───────────────────────────────────────────────────────────────

func makeRequest(method, path string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	wsID := uuid.New()
	r = r.WithContext(WithWorkspaceID(r.Context(), wsID))
	return r
}

func TestMiddleware_SkipsGET(t *testing.T) {
	ms := &mockSink{}
	resetSink(t, ms)

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), makeRequest(http.MethodGet, "/api/issues"))

	if len(ms.written) != 0 {
		t.Errorf("GET should not be recorded, got %d events", len(ms.written))
	}
}

func TestMiddleware_RecordsPOST(t *testing.T) {
	ms := &mockSink{}
	resetSink(t, ms)

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), makeRequest(http.MethodPost, "/api/issues"))

	if len(ms.written) != 1 {
		t.Fatalf("expected 1 event, got %d", len(ms.written))
	}
	if !strings.HasPrefix(ms.written[0].Action, "http.post.") {
		t.Errorf("unexpected action: %q", ms.written[0].Action)
	}
}

func TestMiddleware_SkipsWithoutWorkspace(t *testing.T) {
	ms := &mockSink{}
	resetSink(t, ms)

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request without workspace in context.
	r := httptest.NewRequest(http.MethodDelete, "/api/something", nil)
	handler.ServeHTTP(httptest.NewRecorder(), r)

	if len(ms.written) != 0 {
		t.Errorf("missing workspace should skip recording")
	}
}

func TestMiddleware_SensitiveReadRecorded(t *testing.T) {
	ms := &mockSink{}
	resetSink(t, ms)

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := makeRequest(http.MethodGet, "/api/rdev/files/abc.txt")
	handler.ServeHTTP(httptest.NewRecorder(), r)

	if len(ms.written) != 1 {
		t.Fatalf("sensitive GET should be recorded, got %d", len(ms.written))
	}
	if ms.written[0].Action != ActionFileRead {
		t.Errorf("expected file.read action, got %q", ms.written[0].Action)
	}
}

func TestMiddleware_ClientIP(t *testing.T) {
	ms := &mockSink{}
	resetSink(t, ms)

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	r := makeRequest(http.MethodPost, "/api/foo")
	r.Header.Set("X-Real-IP", "1.2.3.4")
	handler.ServeHTTP(httptest.NewRecorder(), r)

	if len(ms.written) == 0 {
		t.Fatal("no event written")
	}
	if ms.written[0].ClientIP != "1.2.3.4" {
		t.Errorf("expected IP 1.2.3.4, got %q", ms.written[0].ClientIP)
	}
}

// ── QueryParams / parseQueryParams ───────────────────────────────────────────

func TestParseQueryParams_RequiresWorkspaceID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/rdev/audit/events", nil)
	_, err := parseQueryParams(r)
	if err == nil {
		t.Fatal("expected error for missing workspace_id")
	}
}

func TestParseQueryParams_FiltersPopulated(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	sinceStr := now.Add(-time.Hour).Format(time.RFC3339)
	untilStr := now.Format(time.RFC3339)

	url := "/api/rdev/audit/events?workspace_id=ws1&action=agent.task.started" +
		"&actor_type=agent&since=" + sinceStr + "&until=" + untilStr +
		"&limit=10&offset=5"

	r := httptest.NewRequest(http.MethodGet, url, nil)
	params, err := parseQueryParams(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if params.WorkspaceID != "ws1" {
		t.Errorf("workspace_id: %q", params.WorkspaceID)
	}
	if params.Action != "agent.task.started" {
		t.Errorf("action: %q", params.Action)
	}
	if params.ActorType != "agent" {
		t.Errorf("actor_type: %q", params.ActorType)
	}
	if params.Limit != 10 {
		t.Errorf("limit: %d", params.Limit)
	}
	if params.Offset != 5 {
		t.Errorf("offset: %d", params.Offset)
	}
	if params.Since == nil {
		t.Error("since should be set")
	}
	if params.Until == nil {
		t.Error("until should be set")
	}
}

func TestParseQueryParams_InvalidSince(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/events?workspace_id=ws&since=notadate", nil)
	_, err := parseQueryParams(r)
	if err == nil {
		t.Fatal("expected error for invalid since")
	}
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

func buildRouter() chi.Router {
	r := chi.NewRouter()
	RegisterRoutes(r)
	return r
}

func TestHandleListEvents_NoSink(t *testing.T) {
	resetSink(t, nil)
	r := buildRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/rdev/audit/events?workspace_id=ws1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestHandleListEvents_JSON(t *testing.T) {
	wsID := uuid.New()
	ms := &mockSink{results: []Event{
		{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			ActorType:   ActorUser,
			Action:      "http.post./api/issues",
			OccurredAt:  time.Now().UTC(),
			Metadata:    map[string]any{},
		},
	}}
	resetSink(t, ms)

	r := buildRouter()
	req := httptest.NewRequest(http.MethodGet,
		"/api/rdev/audit/events?workspace_id="+wsID.String(), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("unexpected content-type: %q", ct)
	}

	var events []Event
	if err := json.NewDecoder(rec.Body).Decode(&events); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

// ── CSV export ────────────────────────────────────────────────────────────────

func TestHandleExportCSV_ContentType(t *testing.T) {
	wsID := uuid.New()
	ms := &mockSink{results: []Event{
		{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			ActorType:   ActorAgent,
			Action:      "agent.task.completed",
			OccurredAt:  time.Now().UTC(),
			Metadata:    map[string]any{"key": "value"},
		},
	}}
	resetSink(t, ms)

	r := buildRouter()
	req := httptest.NewRequest(http.MethodGet,
		"/api/rdev/audit/events/export?workspace_id="+wsID.String(), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("expected text/csv content-type, got %q", ct)
	}

	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil {
		t.Fatalf("csv parse error: %v", err)
	}
	// header + 1 data row
	if len(rows) != 2 {
		t.Errorf("expected 2 rows (header+data), got %d", len(rows))
	}
	if rows[0][0] != "id" {
		t.Errorf("expected first header column 'id', got %q", rows[0][0])
	}
}

func TestHandleExportCSV_NoSink(t *testing.T) {
	resetSink(t, nil)
	r := buildRouter()
	req := httptest.NewRequest(http.MethodGet,
		"/api/rdev/audit/events/export?workspace_id=ws1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}
