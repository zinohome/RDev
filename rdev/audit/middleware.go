package audit

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type contextKey string

const (
	ContextKeyWorkspaceID contextKey = "audit_workspace_id"
	ContextKeyActorType   contextKey = "audit_actor_type"
	ContextKeyActorID     contextKey = "audit_actor_id"
)

// WithWorkspaceID injects the workspace UUID into the request context for auditing.
func WithWorkspaceID(ctx context.Context, wsID uuid.UUID) context.Context {
	return context.WithValue(ctx, ContextKeyWorkspaceID, wsID)
}

// WithActor injects actor identity into the request context for auditing.
func WithActor(ctx context.Context, actorType ActorType, actorID *uuid.UUID) context.Context {
	ctx = context.WithValue(ctx, ContextKeyActorType, actorType)
	return context.WithValue(ctx, ContextKeyActorID, actorID)
}

var writeMethods = map[string]bool{
	http.MethodPost:   true,
	http.MethodPut:    true,
	http.MethodPatch:  true,
	http.MethodDelete: true,
}

// sensitiveReadPaths records GET requests to these path prefixes as file.read events.
var sensitiveReadPaths = []string{
	"/api/rdev/files/",
}

// Middleware records an audit event for write operations (and sensitive reads).
// It reads workspace_id and actor from context values set by WithWorkspaceID / WithActor.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		ctx := r.Context()
		wsID, _ := ctx.Value(ContextKeyWorkspaceID).(uuid.UUID)
		if wsID == uuid.Nil {
			return
		}

		method := r.Method
		isSensitiveRead := false
		if method == http.MethodGet {
			for _, prefix := range sensitiveReadPaths {
				if strings.HasPrefix(r.URL.Path, prefix) {
					isSensitiveRead = true
					break
				}
			}
		}

		if !writeMethods[method] && !isSensitiveRead {
			return
		}

		action := buildAction(method, r, isSensitiveRead)

		e := Event{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			Action:      action,
			ClientIP:    clientIP(r),
			OccurredAt:  time.Now(),
			Metadata:    map[string]any{},
		}

		if at, ok := ctx.Value(ContextKeyActorType).(ActorType); ok && at != "" {
			e.ActorType = at
		} else {
			e.ActorType = ActorSystem
		}
		if aid, ok := ctx.Value(ContextKeyActorID).(*uuid.UUID); ok {
			e.ActorID = aid
		}

		Record(ctx, e)
	})
}

// buildAction produces "http.{method}.{route_pattern}" or "file.read" for sensitive GETs.
func buildAction(method string, r *http.Request, sensitiveRead bool) string {
	if sensitiveRead {
		return ActionFileRead
	}
	pattern := routePattern(r)
	return "http." + strings.ToLower(method) + "." + pattern
}

// routePattern returns the chi route pattern, falling back to the raw URL path.
func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	return r.URL.Path
}

// clientIP extracts the real client IP, preferring proxy headers over RemoteAddr.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.SplitN(fwd, ",", 2)[0])
	}
	return r.RemoteAddr
}
