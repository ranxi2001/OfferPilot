package interview

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestReviewExposesOnlyKnowledgeBoundToAnsweredQuestion(t *testing.T) {
	const allowed = "ALLOWED_REFERENCE_32BD"
	const unrelated = "UNRELATED_REFERENCE_91EF"
	allowedRef := EvidenceRef{SourceID: "knowledge:allowed", Kind: SourceKnowledge, AnchorID: "knowledge:allowed:1", Locator: "block:1", Quote: "问题：允许的问题\n参考答案：" + allowed}
	unrelatedRef := EvidenceRef{SourceID: "knowledge:other", Kind: SourceKnowledge, AnchorID: "knowledge:other:1", Locator: "block:2", Quote: "问题：其他问题\n参考答案：" + unrelated}
	store := NewMemoryStore()
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	session := InterviewSession{
		ID: "review-1", State: StateAwaitingAnswer, StartedAt: now,
		Profile: Profile{Coverage: []CoveragePoint{{ID: "coverage-1", Area: FocusKnowledge}}},
		Sources: SourceIndex{Anchors: map[string]SourceAnchor{
			allowedRef.AnchorID:   {ID: allowedRef.AnchorID, SourceID: allowedRef.SourceID, Kind: SourceKnowledge, Locator: allowedRef.Locator, Text: allowedRef.Quote},
			unrelatedRef.AnchorID: {ID: unrelatedRef.AnchorID, SourceID: unrelatedRef.SourceID, Kind: SourceKnowledge, Locator: unrelatedRef.Locator, Text: unrelatedRef.Quote},
		}},
		Answers: []AnswerRecord{{
			Question:   Question{ID: "question-1", Text: "允许的问题", CoveragePointID: "coverage-1", EvidenceRefs: []EvidenceRef{allowedRef}},
			Answer:     AnswerPayload{Text: "candidate", InputMode: InputModeText},
			Assessment: Assessment{Correctness: 2, Gaps: []string{"gap"}}, AnsweredAt: now,
		}},
	}
	if err := store.Create(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	service := NewService(Dependencies{Store: store, Clock: fixedClock{value: now}})

	review, err := service.Review(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Turns) != 1 || len(review.Turns[0].References) != 1 {
		t.Fatalf("review=%+v", review)
	}
	if !strings.Contains(review.Turns[0].References[0].Answer, allowed) {
		t.Fatalf("allowed reference missing: %+v", review)
	}
	if strings.Contains(review.Turns[0].References[0].Answer, unrelated) {
		t.Fatalf("unrelated reference leaked: %+v", review)
	}

	snapshot, err := service.Snapshot(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot.Turns[0].Question.EvidenceRefs[0].Quote, allowed) {
		t.Fatal("ordinary snapshot leaked review reference")
	}
}
