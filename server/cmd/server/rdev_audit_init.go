// rdev_audit_init.go initialises the audit PostgresSink and registers audit query routes.
// Route: GET /api/workspaces/{workspaceId}/audit-logs
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/extension"
	rdevaudit "github.com/zinohome/RDev/rdev/audit"
)

var (
	auditPool     *pgxpool.Pool
	auditPoolOnce sync.Once
)

func getAuditPool() *pgxpool.Pool {
	auditPoolOnce.Do(func() {
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			dbURL = "postgres://multica:multica@localhost:5432/multica?sslmode=disable"
		}
		pool, err := pgxpool.New(context.Background(), dbURL)
		if err != nil {
			return
		}
		auditPool = pool
		// Register the postgres sink so audit queries work.
		rdevaudit.RegisterSink(rdevaudit.NewPostgresSink(pool))
	})
	return auditPool
}

// auditLogEntry is the JSON shape returned to the frontend.
type auditLogEntry struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	ActorID       string `json:"actor_id"`
	ActorName     string `json:"actor_name"`
	ActorType     string `json:"actor_type"`
	Action        string `json:"action"`
	ResourceType  string `json:"resource_type"`
	ResourceID    string `json:"resource_id"`
	ResourceLabel string `json:"resource_label,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type auditLogResponse struct {
	Entries []auditLogEntry `json:"entries"`
	Total   int             `json:"total"`
}

const auditQuerySQL = `
SELECT
    ae.id::text,
    ae.workspace_id::text,
    COALESCE(ae.actor_id::text, '') AS actor_id,
    ae.actor_type,
    ae.action,
    COALESCE(ae.resource_type, '') AS resource_type,
    COALESCE(ae.resource_id, '') AS resource_id,
    ae.occurred_at,
    COALESCE(u.name, ag.name, 'System') AS actor_name,
    COALESCE(ae.metadata->>'resource_label', '') AS resource_label
FROM audit_event ae
LEFT JOIN "user" u ON ae.actor_type = 'member' AND ae.actor_id = u.id
LEFT JOIN agent ag ON ae.actor_type = 'agent' AND ae.actor_id = ag.id
WHERE ae.workspace_id = $1`

func handleWorkspaceAuditLogs(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "workspaceId")
	if workspaceID == "" {
		workspaceID = chi.URLParam(r, "id")
	}
	if workspaceID == "" {
		http.Error(w, `{"error":"workspace id required"}`, http.StatusBadRequest)
		return
	}

	pool := getAuditPool()
	if pool == nil {
		http.Error(w, `{"error":"audit service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	limit := 25
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	// Build dynamic WHERE clause additions.
	query := auditQuerySQL
	args := []any{workspaceID}
	idx := 2

	if action := r.URL.Query().Get("action"); action != "" && action != "all" {
		query += " AND ae.action = $" + strconv.Itoa(idx)
		args = append(args, action)
		idx++
	}
	if since := r.URL.Query().Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			query += " AND ae.occurred_at >= $" + strconv.Itoa(idx)
			args = append(args, t)
			idx++
		}
	}
	if until := r.URL.Query().Get("until"); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			query += " AND ae.occurred_at <= $" + strconv.Itoa(idx)
			args = append(args, t)
			idx++
		}
	}

	countQuery := `SELECT COUNT(*) FROM audit_event WHERE workspace_id = $1`
	var total int
	_ = pool.QueryRow(r.Context(), countQuery, workspaceID).Scan(&total)

	query += " ORDER BY ae.occurred_at DESC LIMIT $" + strconv.Itoa(idx) + " OFFSET $" + strconv.Itoa(idx+1)
	args = append(args, limit, offset)

	rows, err := pool.Query(r.Context(), query, args...)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries []auditLogEntry
	for rows.Next() {
		var e auditLogEntry
		var occurredAt time.Time
		if err := rows.Scan(
			&e.ID, &e.WorkspaceID, &e.ActorID, &e.ActorType, &e.Action,
			&e.ResourceType, &e.ResourceID, &occurredAt,
			&e.ActorName, &e.ResourceLabel,
		); err != nil {
			continue
		}
		e.CreatedAt = occurredAt.UTC().Format(time.RFC3339)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []auditLogEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(auditLogResponse{Entries: entries, Total: total})
}

func init() {
	extension.RegisterExtensionRoutes(func(r chi.Router) {
		// Workspace-scoped audit log endpoint (matches frontend API client).
		r.Get("/api/workspaces/{workspaceId}/audit-logs", handleWorkspaceAuditLogs)
		// Also register the rdev-native audit routes.
		rdevaudit.RegisterRoutes(r)
	})
	// Trigger pool + sink initialisation early (non-blocking).
	go getAuditPool()
}
