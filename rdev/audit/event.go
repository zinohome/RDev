package audit

import (
	"time"

	"github.com/google/uuid"
)

type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
)

type Event struct {
	ID            uuid.UUID      `json:"id"`
	WorkspaceID   uuid.UUID      `json:"workspace_id"`
	ActorType     ActorType      `json:"actor_type"`
	ActorID       *uuid.UUID     `json:"actor_id,omitempty"`
	Action        string         `json:"action"`
	ResourceType  string         `json:"resource_type,omitempty"`
	ResourceID    string         `json:"resource_id,omitempty"`
	ClientIP      string         `json:"client_ip,omitempty"`
	CorrelationID *uuid.UUID     `json:"correlation_id,omitempty"`
	Metadata      map[string]any `json:"metadata"`
	OccurredAt    time.Time      `json:"occurred_at"`
}

const (
	ActionAgentTaskEnqueued  = "agent.task.enqueued"
	ActionAgentTaskStarted   = "agent.task.started"
	ActionAgentTaskCompleted = "agent.task.completed"
	ActionAgentTaskFailed    = "agent.task.failed"
	ActionModelRequest       = "model.request"
	ActionFileRead           = "file.read"
	ActionVCSBindingChanged  = "vcs.binding.changed"
	ActionModelRouteChanged  = "model.route.changed"
)
