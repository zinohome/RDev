package audit

import (
	"context"
	"net/http"
	"time"
)

// Event represents a single audit record.
type Event struct {
	WorkspaceID   string
	ActorType     string // user | agent | system
	ActorID       string
	Action        string
	ResourceType  string
	ResourceID    string
	Metadata      map[string]any
	OccurredAt    time.Time
	ClientIP      string
	CorrelationID string // optional; links related events in the same session
}

// Sink is the output interface for audit events.
// v1 provides only PostgresSink (implemented in rdev/audit/).
type Sink interface {
	Write(ctx context.Context, event Event) error
}

var sinks []Sink

// RegisterAuditSink registers an audit output target.
// Must be called before HTTP server starts to avoid missing early requests.
func RegisterAuditSink(s Sink) {
	sinks = append(sinks, s)
}

// Emit sends an audit event to all registered sinks.
// A single sink write failure does not affect other sinks.
func Emit(ctx context.Context, event Event) {
	for _, s := range sinks {
		_ = s.Write(ctx, event)
	}
}

// Middleware is an HTTP middleware that records API call audit events.
// workspaceIDFn extracts workspace ID from the request (avoids circular dep on handler).
func Middleware(workspaceIDFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			if len(sinks) == 0 {
				return
			}
			wsID := workspaceIDFn(r)
			if wsID == "" {
				return
			}
			Emit(r.Context(), Event{
				WorkspaceID: wsID,
				Action:      r.Method + " " + r.URL.Path,
				OccurredAt:  time.Now(),
				ClientIP:    r.RemoteAddr,
			})
		})
	}
}
