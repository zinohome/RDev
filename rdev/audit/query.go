package audit

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts the audit query endpoints onto the given router.
// Intended to be called by server/internal/extension.RegisterExtensionRoutes during init.
//
//	extension.RegisterExtensionRoutes(func(r chi.Router) {
//	    audit.RegisterRoutes(r)
//	})
func RegisterRoutes(r chi.Router) {
	r.Route("/api/rdev/audit", func(r chi.Router) {
		r.Get("/events", handleListEvents)
		r.Get("/events/export", handleExportCSV)
	})
}

// handleListEvents lists audit events with optional filters.
//
// GET /api/rdev/audit/events?workspace_id=&action=&actor_type=&since=&until=&limit=50&offset=0
func handleListEvents(w http.ResponseWriter, r *http.Request) {
	if globalSink == nil {
		http.Error(w, "audit sink not configured", http.StatusServiceUnavailable)
		return
	}

	q, err := parseQueryParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	events, err := globalSink.Query(r.Context(), q)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(events); err != nil {
		http.Error(w, "encode failed", http.StatusInternalServerError)
	}
}

// handleExportCSV exports matching audit events as a CSV download.
//
// GET /api/rdev/audit/events/export?...same params as /events...
func handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if globalSink == nil {
		http.Error(w, "audit sink not configured", http.StatusServiceUnavailable)
		return
	}

	q, err := parseQueryParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Export is typically large; raise the default limit ceiling.
	if q.Limit <= 0 {
		q.Limit = 10000
	}

	events, err := globalSink.Query(r.Context(), q)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit_events.csv"`)

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"id", "workspace_id", "actor_type", "actor_id",
		"action", "resource_type", "resource_id",
		"client_ip", "occurred_at", "metadata",
	})

	for _, e := range events {
		meta, _ := json.Marshal(e.Metadata)
		actorID := ""
		if e.ActorID != nil {
			actorID = e.ActorID.String()
		}
		_ = cw.Write([]string{
			e.ID.String(),
			e.WorkspaceID.String(),
			string(e.ActorType),
			actorID,
			e.Action,
			e.ResourceType,
			e.ResourceID,
			e.ClientIP,
			e.OccurredAt.UTC().Format(time.RFC3339),
			string(meta),
		})
	}
	cw.Flush()
}

func parseQueryParams(r *http.Request) (QueryParams, error) {
	q := r.URL.Query()

	wsID := q.Get("workspace_id")
	if wsID == "" {
		return QueryParams{}, fmt.Errorf("workspace_id is required")
	}

	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	params := QueryParams{
		WorkspaceID: wsID,
		ActorType:   q.Get("actor_type"),
		Action:      q.Get("action"),
		Limit:       limit,
		Offset:      offset,
	}

	if s := q.Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return QueryParams{}, fmt.Errorf("invalid since: %w", err)
		}
		params.Since = &t
	}
	if s := q.Get("until"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return QueryParams{}, fmt.Errorf("invalid until: %w", err)
		}
		params.Until = &t
	}

	return params, nil
}
