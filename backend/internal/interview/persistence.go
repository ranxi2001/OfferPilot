package interview

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrPersistenceNotFound    = errors.New("interview persistence: not found")
	ErrPersistenceConflict    = errors.New("interview persistence: conflict")
	ErrIdempotencyConflict    = errors.New("interview persistence: idempotency key reused with different request")
	ErrAnswerAlreadyCommitted = errors.New("interview persistence: answer already committed")
	ErrLeaseHeld              = errors.New("interview persistence: lease held by another owner")
	ErrLeaseLost              = errors.New("interview persistence: lease no longer owned")
	ErrOutboxClaimLost        = errors.New("interview persistence: outbox claim no longer owned")
)

type CommandStatus string

const (
	CommandPending   CommandStatus = "pending"
	CommandRunning   CommandStatus = "running"
	CommandSucceeded CommandStatus = "succeeded"
	CommandFailed    CommandStatus = "failed"
)

type CommandSpec struct {
	ID             string
	PrincipalID    string
	SessionID      string
	Action         string
	IdempotencyKey string
	SubjectID      string
	RequestHash    string
}

type Command struct {
	ID             string          `json:"id"`
	PrincipalID    string          `json:"principalId,omitempty"`
	SessionID      string          `json:"sessionId"`
	Action         string          `json:"action"`
	IdempotencyKey string          `json:"idempotencyKey"`
	SubjectID      string          `json:"subjectId,omitempty"`
	RequestHash    string          `json:"requestHash"`
	Status         CommandStatus   `json:"status"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          json.RawMessage `json:"error,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type CommandTransition struct {
	Status CommandStatus
	Result json.RawMessage
	Error  json.RawMessage
}

// AnswerCommitSpec is the single durable commit boundary for an accepted
// answer. Implementations must apply the session CAS, command result, and
// committed event atomically.
type AnswerCommitSpec struct {
	Session         InterviewSession
	ExpectedVersion int64
	CommandID       string
	Result          json.RawMessage
	Event           SessionEventSpec
}

// AnswerCommitRepository is optional because in-memory and legacy Store
// implementations do not expose transactions. A durable answer command is
// executed only when its persistence repository implements this contract.
type AnswerCommitRepository interface {
	CommitAnswer(context.Context, AnswerCommitSpec) error
}

type SessionEventSpec struct {
	EventID   string
	SessionID string
	CommandID string
	Type      string
	Payload   json.RawMessage
}

type SessionEvent struct {
	EventID   string          `json:"eventId"`
	SessionID string          `json:"sessionId"`
	Sequence  int64           `json:"sequence"`
	CommandID string          `json:"commandId,omitempty"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"createdAt"`
}

type InvocationStatus string

const (
	InvocationPending      InvocationStatus = "pending"
	InvocationRunning      InvocationStatus = "running"
	InvocationSucceeded    InvocationStatus = "succeeded"
	InvocationFailed       InvocationStatus = "failed"
	InvocationManualReview InvocationStatus = "manual_review"
)

type ModelInvocationSpec struct {
	ID          string
	RunID       string
	CommandID   string
	SessionID   string
	Agent       string
	RequestHash string
	Attempt     int
}

type ModelInvocation struct {
	ID          string           `json:"id"`
	RunID       string           `json:"runId"`
	CommandID   string           `json:"commandId,omitempty"`
	SessionID   string           `json:"sessionId"`
	Agent       string           `json:"agent"`
	RequestHash string           `json:"requestHash"`
	Status      InvocationStatus `json:"status"`
	Attempt     int              `json:"attempt"`
	Result      json.RawMessage  `json:"result,omitempty"`
	Error       json.RawMessage  `json:"error,omitempty"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
}

type InvocationTransition struct {
	Status InvocationStatus
	Result json.RawMessage
	Error  json.RawMessage
}

type CheckpointSpec struct {
	ID        string
	RunID     string
	CommandID string
	SessionID string
	Sequence  int64
	State     json.RawMessage
}

type Checkpoint struct {
	ID        string          `json:"id"`
	RunID     string          `json:"runId"`
	CommandID string          `json:"commandId,omitempty"`
	SessionID string          `json:"sessionId"`
	Sequence  int64           `json:"sequence"`
	State     json.RawMessage `json:"state"`
	CreatedAt time.Time       `json:"createdAt"`
}

type RunLease struct {
	RunID      string    `json:"runId"`
	OwnerID    string    `json:"ownerId"`
	LeaseToken string    `json:"leaseToken"`
	ExpiresAt  time.Time `json:"expiresAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "pending"
	OutboxClaimed   OutboxStatus = "claimed"
	OutboxPublished OutboxStatus = "published"
)

type OutboxMessageSpec struct {
	ID          string
	SessionID   string
	Sequence    int64
	Type        string
	Payload     json.RawMessage
	AvailableAt time.Time
}

type OutboxMessage struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"sessionId"`
	Sequence    int64           `json:"sequence"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Status      OutboxStatus    `json:"status"`
	AvailableAt time.Time       `json:"availableAt"`
	ClaimOwner  string          `json:"claimOwner,omitempty"`
	ClaimUntil  time.Time       `json:"claimUntil,omitempty"`
	Attempts    int             `json:"attempts"`
	LastError   string          `json:"lastError,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	PublishedAt time.Time       `json:"publishedAt,omitempty"`
}

// PersistenceRepository is the durable execution boundary used by the
// harness. It deliberately remains separate from Store so existing aggregate
// snapshots and callers retain their current contract.
type PersistenceRepository interface {
	CreateOrGetCommand(context.Context, CommandSpec) (Command, bool, error)
	GetCommand(context.Context, string) (Command, error)
	TransitionCommand(context.Context, string, CommandStatus, CommandTransition) (Command, error)

	AppendSessionEvent(context.Context, SessionEventSpec) (SessionEvent, bool, error)
	ListSessionEvents(context.Context, string, int64, int) ([]SessionEvent, error)

	CreateOrGetModelInvocation(context.Context, ModelInvocationSpec) (ModelInvocation, bool, error)
	GetModelInvocation(context.Context, string) (ModelInvocation, error)
	TransitionModelInvocation(context.Context, string, InvocationStatus, InvocationTransition) (ModelInvocation, error)
	ListRecoverableModelInvocations(context.Context, int) ([]ModelInvocation, error)

	SaveCheckpoint(context.Context, CheckpointSpec) (Checkpoint, bool, error)
	LoadLatestCheckpoint(context.Context, string) (Checkpoint, error)

	AcquireRunLease(context.Context, string, string, string, time.Time, time.Duration) (RunLease, bool, error)
	RenewRunLease(context.Context, string, string, string, time.Time, time.Duration) (RunLease, error)
	ReleaseRunLease(context.Context, string, string, string) error
	GetRunLease(context.Context, string) (RunLease, error)

	EnqueueOutbox(context.Context, OutboxMessageSpec) (OutboxMessage, bool, error)
	GetOutboxMessage(context.Context, string) (OutboxMessage, error)
	ClaimOutbox(context.Context, string, time.Time, time.Duration, int) ([]OutboxMessage, error)
	MarkOutboxPublished(context.Context, string, string, time.Time) error
	MarkOutboxFailed(context.Context, string, string, string, time.Time) error
}
