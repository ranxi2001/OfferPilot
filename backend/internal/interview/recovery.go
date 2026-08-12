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

// ReviewSnapshot is an explicit candidate-requested learning projection. Unlike
// SessionSnapshot, it may include the private reference text already bound to
// an answered knowledge question. It still excludes prompts, model reasoning,
// raw materials, and unrelated knowledge.
type ReviewSnapshot struct {
	SchemaVersion string         `json:"schemaVersion"`
	InterviewID   string         `json:"interviewId"`
	State         InterviewState `json:"state"`
	StartedAt     time.Time      `json:"startedAt"`
	GeneratedAt   time.Time      `json:"generatedAt"`
	Turns         []ReviewTurn   `json:"turns"`
}

type ReviewTurn struct {
	Question   Question          `json:"question"`
	Answer     AnswerPayload     `json:"answer"`
	Feedback   AnswerFeedback    `json:"feedback"`
	References []ReviewReference `json:"references"`
	AnsweredAt time.Time         `json:"answeredAt"`
}

type ReviewReference struct {
	EvidenceID string `json:"evidenceId"`
	Title      string `json:"title"`
	Answer     string `json:"answer"`
	Locator    string `json:"locator,omitempty"`
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

func (s *Service) Review(ctx context.Context, interviewID string) (ReviewSnapshot, error) {
	if strings.TrimSpace(interviewID) == "" {
		return ReviewSnapshot{}, validation("interviewId", "is required")
	}
	session, err := s.load(ctx, interviewID)
	if err != nil {
		return ReviewSnapshot{}, err
	}

	turns := make([]ReviewTurn, 0, len(session.Answers))
	for _, stored := range session.Answers {
		record := cloneSession(InterviewSession{Answers: []AnswerRecord{stored}}).Answers[0]
		publicRecordEvidence(session.Sources, &record)
		focus := coveragePointByID(session.Profile, stored.Question.CoveragePointID).Area
		feedback := AnswerFeedback{
			Focus: focus, Assessment: record.Assessment,
			Summary: assessmentSummary(focus, record.Assessment),
		}
		turns = append(turns, ReviewTurn{
			Question: record.Question, Answer: record.Answer, Feedback: feedback,
			References: reviewReferences(stored.Question.EvidenceRefs), AnsweredAt: stored.AnsweredAt,
		})
	}
	return ReviewSnapshot{
		SchemaVersion: "1.0.0", InterviewID: session.ID, State: session.State,
		StartedAt: session.StartedAt, GeneratedAt: s.clock.Now(), Turns: turns,
	}, nil
}

func reviewReferences(refs []EvidenceRef) []ReviewReference {
	result := make([]ReviewReference, 0, len(refs))
	seen := make(map[string]struct{})
	for _, ref := range refs {
		if ref.Kind != SourceKnowledge {
			continue
		}
		answer := knowledgeReferenceText(ref.Quote)
		if answer == "" {
			continue
		}
		if _, exists := seen[ref.AnchorID]; exists {
			continue
		}
		seen[ref.AnchorID] = struct{}{}
		result = append(result, ReviewReference{
			EvidenceID: ref.AnchorID, Title: knowledgeQuestionLabel(ref.Quote),
			Answer: answer, Locator: ref.Locator,
		})
	}
	return result
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
