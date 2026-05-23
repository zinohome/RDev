package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Sink is the output interface for audit events.
type Sink interface {
	Write(ctx context.Context, e Event) error
	Query(ctx context.Context, q QueryParams) ([]Event, error)
}

// QueryParams filters for listing audit events.
type QueryParams struct {
	WorkspaceID string
	ActorType   string
	Action      string
	Since       *time.Time
	Until       *time.Time
	Limit       int
	Offset      int
}

var (
	globalSinkMu sync.RWMutex
	globalSink   Sink
)

// RegisterSink sets the active sink. Safe for concurrent use.
func RegisterSink(s Sink) {
	globalSinkMu.Lock()
	defer globalSinkMu.Unlock()
	globalSink = s
}

// Record writes an event to the registered sink, silently dropping it if none is set.
func Record(ctx context.Context, e Event) {
	globalSinkMu.RLock()
	s := globalSink
	globalSinkMu.RUnlock()
	if s == nil {
		return
	}
	if err := s.Write(ctx, e); err != nil {
		log.Printf("audit: write error: %v", err)
	}
}

// PostgresSink persists audit events to a postgres table.
type PostgresSink struct {
	pool *pgxpool.Pool
}

// NewPostgresSink creates a PostgresSink backed by the given connection pool.
func NewPostgresSink(pool *pgxpool.Pool) *PostgresSink {
	return &PostgresSink{pool: pool}
}

const insertSQL = `
INSERT INTO audit_event
    (id, workspace_id, actor_type, actor_id, action, resource_type, resource_id,
     client_ip, correlation_id, metadata, occurred_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`

// Write inserts a single audit event.
func (s *PostgresSink) Write(ctx context.Context, e Event) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("audit: marshal metadata: %w", err)
	}
	_, err = s.pool.Exec(ctx, insertSQL,
		e.ID, e.WorkspaceID, string(e.ActorType), e.ActorID,
		e.Action, e.ResourceType, e.ResourceID,
		e.ClientIP, e.CorrelationID, meta, e.OccurredAt,
	)
	return err
}

const selectBase = `
SELECT id, workspace_id, actor_type, actor_id, action, resource_type, resource_id,
       client_ip, correlation_id, metadata, occurred_at
FROM audit_event
WHERE %s
ORDER BY occurred_at DESC
LIMIT $%d OFFSET $%d`

// Query returns events matching the given filter, newest first.
func (s *PostgresSink) Query(ctx context.Context, q QueryParams) ([]Event, error) {
	args := []any{q.WorkspaceID}
	conditions := []string{"workspace_id = $1"}
	idx := 2

	if q.ActorType != "" {
		conditions = append(conditions, fmt.Sprintf("actor_type = $%d", idx))
		args = append(args, q.ActorType)
		idx++
	}
	if q.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", idx))
		args = append(args, q.Action)
		idx++
	}
	if q.Since != nil {
		conditions = append(conditions, fmt.Sprintf("occurred_at >= $%d", idx))
		args = append(args, *q.Since)
		idx++
	}
	if q.Until != nil {
		conditions = append(conditions, fmt.Sprintf("occurred_at <= $%d", idx))
		args = append(args, *q.Until)
		idx++
	}

	limit := q.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	sql := fmt.Sprintf(selectBase, strings.Join(conditions, " AND "), idx, idx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var metaBytes []byte
		if err := rows.Scan(
			&e.ID, &e.WorkspaceID, &e.ActorType, &e.ActorID,
			&e.Action, &e.ResourceType, &e.ResourceID,
			&e.ClientIP, &e.CorrelationID, &metaBytes, &e.OccurredAt,
		); err != nil {
			return nil, err
		}
		if metaBytes != nil {
			if err := json.Unmarshal(metaBytes, &e.Metadata); err != nil {
				return nil, fmt.Errorf("audit: unmarshal metadata: %w", err)
			}
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
