// Package executiontrace carries sanitized, request-scoped progress events.
// It must never receive prompts, source material, model output, or raw errors.
package executiontrace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Event struct {
	ID         string    `json:"id"`
	Stage      string    `json:"stage"`
	Label      string    `json:"label"`
	Detail     string    `json:"detail,omitempty"`
	Status     Status    `json:"status"`
	Agent      string    `json:"agent,omitempty"`
	At         time.Time `json:"at"`
	DurationMS int64     `json:"durationMs,omitempty"`
}

type Sink func(Event)

type sinkKey struct{}

var fallbackID atomic.Uint64

// WithSink attaches one request-local sink. The sink is intentionally not
// composed with a parent sink, which prevents accidental cross-request fanout.
func WithSink(ctx context.Context, sink Sink) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, sinkKey{}, sink)
}

// Emit delivers an event to the request-local sink and isolates callers from
// a faulty observer.
func Emit(ctx context.Context, event Event) {
	if ctx == nil {
		return
	}
	sink, _ := ctx.Value(sinkKey{}).(Sink)
	if sink == nil {
		return
	}
	defer func() { _ = recover() }()
	sink(event)
}

// Span emits queued/running events immediately and exactly one terminal event.
type Span struct {
	ctx     context.Context
	event   Event
	started time.Time
	once    sync.Once
}

func Start(ctx context.Context, stage, label, agent string) *Span {
	now := time.Now().UTC()
	span := &Span{
		ctx:     ctx,
		started: now,
		event: Event{
			ID:    newID(),
			Stage: stage,
			Label: label,
			Agent: agent,
			At:    now,
		},
	}
	queued := span.event
	queued.Status = StatusQueued
	Emit(ctx, queued)
	running := span.event
	running.Status = StatusRunning
	running.At = time.Now().UTC()
	Emit(ctx, running)
	return span
}

// End completes a span. successDetail must contain only fixed decisions or
// aggregate counts; failures are reduced to a safe error class.
func (s *Span) End(err error, successDetail string) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		finished := time.Now().UTC()
		event := s.event
		event.At = finished
		event.DurationMS = finished.Sub(s.started).Milliseconds()
		if err == nil {
			event.Status = StatusCompleted
			event.Detail = successDetail
		} else {
			event.Status = StatusFailed
			event.Detail = FailureClass(err)
		}
		Emit(s.ctx, event)
	})
}

// FailureClass deliberately exposes no provider or domain error text.
func FailureClass(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "unavailable"
	}
}

func newID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("event-%d-%d", time.Now().UnixNano(), fallbackID.Add(1))
}
