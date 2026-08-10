package harness

import (
	"context"
	"time"

	"offerpilot/backend/internal/executiontrace"
)

type TraceType string

const (
	TraceQueued    TraceType = "queued"
	TraceStarted   TraceType = "started"
	TraceSucceeded TraceType = "succeeded"
	TraceError     TraceType = "error"
)

type TraceEvent struct {
	TraceID  string        `json:"traceId"`
	AgentID  string        `json:"agentId"`
	Type     TraceType     `json:"type"`
	At       time.Time     `json:"at"`
	Wait     time.Duration `json:"wait,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
	Error    string        `json:"error,omitempty"`
}

func (r *Runtime) emit(ctx context.Context, event TraceEvent) {
	r.traceMu.Lock()
	if len(r.traces) == r.traceCapacity {
		copy(r.traces, r.traces[1:])
		r.traces[len(r.traces)-1] = event
	} else {
		r.traces = append(r.traces, event)
	}
	r.traceMu.Unlock()

	if r.traceSink != nil {
		callTraceSink(r.traceSink, event)
	}

	status := executiontrace.StatusRunning
	switch event.Type {
	case TraceQueued:
		status = executiontrace.StatusQueued
	case TraceStarted:
		status = executiontrace.StatusRunning
	case TraceSucceeded:
		status = executiontrace.StatusCompleted
	case TraceError:
		status = executiontrace.StatusFailed
	}
	requestEvent := executiontrace.Event{
		ID:         event.TraceID,
		Stage:      "agent",
		Label:      "Run structured interview agent",
		Status:     status,
		Agent:      event.AgentID,
		At:         event.At,
		DurationMS: event.Duration.Milliseconds(),
	}
	if event.Type == TraceError {
		// The global Harness trace retains the original diagnostic. Request
		// streams expose only a stable error class.
		requestEvent.Detail = "agent call failed"
	}
	executiontrace.Emit(ctx, requestEvent)
}

func callTraceSink(sink func(TraceEvent), event TraceEvent) {
	defer func() { _ = recover() }()
	sink(event)
}

// Traces returns a defensive copy, optionally restricted to one call.
func (r *Runtime) Traces(traceID string) []TraceEvent {
	r.traceMu.RLock()
	defer r.traceMu.RUnlock()
	result := make([]TraceEvent, 0, len(r.traces))
	for _, event := range r.traces {
		if traceID == "" || event.TraceID == traceID {
			result = append(result, event)
		}
	}
	return result
}
