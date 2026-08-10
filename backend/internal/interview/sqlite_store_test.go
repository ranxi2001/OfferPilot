package interview

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestSQLiteStoreCreateLoadReturnsDefensiveSnapshots(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	session := sqliteTestSession()
	want := cloneSession(session)

	if err := store.Create(ctx, session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Create(ctx, session); !errors.Is(err, ErrStoreConflict) {
		t.Fatalf("duplicate Create() error = %v, want ErrStoreConflict", err)
	}

	session.Answers[0].Answer.Text = "mutated original"
	document := session.Sources.Documents["resume-1"]
	document.Content = "mutated original source"
	session.Sources.Documents["resume-1"] = document
	loaded, err := store.Load(ctx, want.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(loaded, want) {
		t.Fatalf("Load() = %#v, want %#v", loaded, want)
	}

	loaded.Answers[0].Answer.Text = "mutated loaded value"
	document = loaded.Sources.Documents["resume-1"]
	document.Content = "mutated loaded source"
	loaded.Sources.Documents["resume-1"] = document
	again, err := store.Load(ctx, want.ID)
	if err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("stored snapshot mutated through caller: got %#v, want %#v", again, want)
	}

	if _, err = store.Load(ctx, "missing"); !errors.Is(err, ErrStoreNotFound) {
		t.Fatalf("Load(missing) error = %v, want ErrStoreNotFound", err)
	}
}

func TestSQLiteStorePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interviews.sqlite")
	first, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("first OpenSQLiteStore() error = %v", err)
	}
	session := sqliteTestSession()
	if err = first.Create(context.Background(), session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err = first.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	second, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("second OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	loaded, err := second.Load(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Load() after reopen error = %v", err)
	}
	if !reflect.DeepEqual(loaded, session) {
		t.Fatalf("Load() after reopen = %#v, want %#v", loaded, session)
	}
}

func TestSQLiteStoreSaveRequiresExpectedVersion(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	session := sqliteTestSession()
	if err := store.Create(ctx, session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	next := cloneSession(session)
	next.Version = 2
	next.ClientSessionID = "updated"
	if err := store.Save(ctx, next, 0); !errors.Is(err, ErrStoreConflict) {
		t.Fatalf("Save(wrong expected version) error = %v, want ErrStoreConflict", err)
	}
	if err := store.Save(ctx, next, 1); err != nil {
		t.Fatalf("Save(valid) error = %v", err)
	}
	if err := store.Save(ctx, next, 1); !errors.Is(err, ErrStoreConflict) {
		t.Fatalf("Save(stale) error = %v, want ErrStoreConflict", err)
	}

	missing := next
	missing.ID = "missing"
	missing.Version = 2
	if err := store.Save(ctx, missing, 1); !errors.Is(err, ErrStoreNotFound) {
		t.Fatalf("Save(missing) error = %v, want ErrStoreNotFound", err)
	}
	missing.Version = 99
	if err := store.Save(ctx, missing, 1); !errors.Is(err, ErrStoreNotFound) {
		t.Fatalf("Save(missing with invalid transition) error = %v, want ErrStoreNotFound", err)
	}
	loaded, err := store.Load(ctx, session.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Version != 2 || loaded.ClientSessionID != "updated" {
		t.Fatalf("Load() = version %d clientSessionId %q, want 2/updated", loaded.Version, loaded.ClientSessionID)
	}
}

func TestSQLiteStoreConcurrentSaveHasOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interviews.sqlite")
	leftStore, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("first OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = leftStore.Close() })
	rightStore, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("second OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = rightStore.Close() })
	ctx := context.Background()
	session := sqliteTestSession()
	if err := leftStore.Create(ctx, session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	left := cloneSession(session)
	left.Version = 2
	left.ClientSessionID = "left"
	right := cloneSession(session)
	right.Version = 2
	right.ClientSessionID = "right"

	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, attempt := range []struct {
		store     *SQLiteStore
		candidate InterviewSession
	}{{leftStore, left}, {rightStore, right}} {
		attempt := attempt
		go func() {
			ready.Done()
			<-start
			results <- attempt.store.Save(ctx, attempt.candidate, 1)
		}()
	}
	ready.Wait()
	close(start)

	var successes, conflicts int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrStoreConflict):
			conflicts++
		default:
			t.Fatalf("concurrent Save() error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent saves: successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
	loaded, err := leftStore.Load(ctx, session.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Version != 2 || (loaded.ClientSessionID != "left" && loaded.ClientSessionID != "right") {
		t.Fatalf("winning snapshot = version %d clientSessionId %q", loaded.Version, loaded.ClientSessionID)
	}
}

func openTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "interviews.sqlite"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func sqliteTestSession() InterviewSession {
	startedAt := time.Date(2026, 8, 10, 12, 34, 56, 789000000, time.UTC)
	evidence := EvidenceRef{
		SourceID: "resume-1", Kind: SourceResume, AnchorID: "resume-1:1",
		Locator: "segment:1", Quote: "OfferPilot adaptive interview agent",
	}
	question := Question{
		ID: "question-1", RootID: "root-1", Text: "What did you own?",
		Kind: QuestionProject, Difficulty: DifficultyHard, CoveragePointID: "coverage-1",
		EvidenceRefs: []EvidenceRef{evidence},
		Adaptation:   QuestionAdaptation{Trigger: PolicyInitial, Reason: "resume evidence"},
	}
	return InterviewSession{
		ID: "interview-1", ClientSessionID: "client-1", Model: "test-model",
		Config: InterviewConfig{
			Focus: FocusProjects, Difficulty: DifficultyHard, QuestionCount: 3,
			Language: "en", FeedbackMode: FeedbackImmediate,
		},
		State: StateAwaitingAnswer,
		Profile: Profile{
			Resume: ResumeProfile{Projects: []ProfilePoint{{
				ID: "project-1", Label: "OfferPilot", EvidenceRefs: []EvidenceRef{evidence},
			}}},
			Coverage: []CoveragePoint{{
				ID: "coverage-1", Area: FocusProjects, Label: "OfferPilot", EvidenceRefs: []EvidenceRef{evidence},
			}},
		},
		Sources: SourceIndex{
			Documents: map[string]SourceDocument{
				"resume-1": {
					ID: "resume-1", Kind: SourceResume, Name: "resume.md",
					Content: "Built the OfferPilot adaptive interview agent.",
				},
			},
			Anchors: map[string]SourceAnchor{
				"resume-1:1": {
					ID: "resume-1:1", SourceID: "resume-1", Kind: SourceResume,
					Locator: "segment:1", Text: "OfferPilot adaptive interview agent",
				},
			},
			Order: []string{"resume-1:1"},
		},
		CurrentQuestion: &question,
		Answers: []AnswerRecord{{
			Question: question,
			Answer: AnswerPayload{
				Text: "I designed the policy and persistence boundary.", InputMode: InputModeText,
			},
			Assessment: Assessment{Correctness: 4, Depth: 4, Specificity: 4},
			Decision:   PolicyDecision{Action: PolicyAdvance, Difficulty: DifficultyHard},
			AnsweredAt: startedAt.Add(time.Minute),
		}},
		CoverageCursor: 1,
		StartedAt:      startedAt,
		Version:        1,
	}
}

var _ Store = (*SQLiteStore)(nil)
