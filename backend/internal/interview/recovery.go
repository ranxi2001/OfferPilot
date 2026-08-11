package interview

import (
	"context"
	"errors"
	"strings"
	"time"
)

type SnapshotTurn struct {
	Question Question       `json:"question"`
	Answer   AnswerPayload  `json:"answer"`
	Feedback AnswerFeedback `json:"feedback"`
}

// SessionSnapshot is the public recovery projection. It deliberately omits
// SourceIndex, private knowledge references, model prompts, and raw event
// payloads.
type SessionSnapshot struct {
	InterviewID     string         `json:"interviewId"`
	State           InterviewState `json:"state"`
	Profile         Profile        `json:"profile"`
	CurrentQuestion *Question      `json:"currentQuestion,omitempty"`
	Turns           []SnapshotTurn `json:"turns"`
	Progress        Progress       `json:"progress"`
	ReportReady     bool           `json:"reportReady"`
}

type SessionEventSummary struct {
	EventID   string    `json:"eventId"`
	Sequence  int64     `json:"sequence"`
	CommandID string    `json:"commandId,omitempty"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"createdAt"`
}

type SessionEventPage struct {
	InterviewID  string                `json:"interviewId"`
	Events       []SessionEventSummary `json:"events"`
	NextSequence int64                 `json:"nextSequence"`
}

func (s *Service) Snapshot(ctx context.Context, interviewID string) (SessionSnapshot, error) {
	if strings.TrimSpace(interviewID) == "" {
		return SessionSnapshot{}, validation("interviewId", "is required")
	}
	session, err := s.load(ctx, interviewID)
	if err != nil {
		return SessionSnapshot{}, err
	}

	publicProfile := cloneSession(InterviewSession{Profile: session.Profile}).Profile
	publicProfileEvidence(session.Sources, &publicProfile)
	turns := make([]SnapshotTurn, 0, len(session.Answers))
	for _, stored := range session.Answers {
		record := cloneSession(InterviewSession{Answers: []AnswerRecord{stored}}).Answers[0]
		publicRecordEvidence(session.Sources, &record)
		focus := coveragePointByID(session.Profile, stored.Question.CoveragePointID).Area
		feedback := AnswerFeedback{Focus: focus}
		if session.Config.FeedbackMode == FeedbackDeferred && session.State != StateCompleted {
			feedback.Deferred = true
		} else {
			feedback.Assessment = record.Assessment
			feedback.Summary = assessmentSummary(focus, record.Assessment)
		}
		turns = append(turns, SnapshotTurn{Question: record.Question, Answer: record.Answer, Feedback: feedback})
	}

	var current *Question
	if session.CurrentQuestion != nil {
		question := *session.CurrentQuestion
		question.EvidenceRefs = publicQuestionEvidence(session.Sources, question.EvidenceRefs)
		question.Adaptation.Reason = publicDecisionReason(PolicyDecision{Action: question.Adaptation.Trigger})
		if questionLeaksKnowledgeReference(session.Sources, question.Text, evidenceIDSet(question.EvidenceRefs)) {
			question.Text = publicQuestionSummary(question.EvidenceRefs)
		}
		current = &question
	}
	return SessionSnapshot{
		InterviewID:     session.ID,
		State:           session.State,
		Profile:         publicProfile,
		CurrentQuestion: current,
		Turns:           turns,
		Progress:        progressFor(session),
		ReportReady:     session.State == StateCompleted,
	}, nil
}

func (s *Service) SessionEvents(ctx context.Context, interviewID string, afterSequence int64, limit int) (SessionEventPage, error) {
	if strings.TrimSpace(interviewID) == "" {
		return SessionEventPage{}, validation("interviewId", "is required")
	}
	if afterSequence < 0 {
		return SessionEventPage{}, validation("after", "must not be negative")
	}
	if _, err := s.load(ctx, interviewID); err != nil {
		return SessionEventPage{}, err
	}
	repository := s.persistence
	if repository == nil {
		return SessionEventPage{}, unavailable("interview event recovery is not configured", errors.New("persistence repository is unavailable"))
	}
	events, err := repository.ListSessionEvents(ctx, interviewID, afterSequence, limit)
	if err != nil {
		return SessionEventPage{}, unavailable("interview events are temporarily unavailable", err)
	}
	summaries := make([]SessionEventSummary, 0, len(events))
	next := afterSequence
	for _, event := range events {
		summaries = append(summaries, SessionEventSummary{
			EventID: event.EventID, Sequence: event.Sequence, CommandID: event.CommandID,
			Type: event.Type, CreatedAt: event.CreatedAt,
		})
		if event.Sequence > next {
			next = event.Sequence
		}
	}
	return SessionEventPage{InterviewID: interviewID, Events: summaries, NextSequence: next}, nil
}
