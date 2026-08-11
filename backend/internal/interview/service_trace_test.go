package interview

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"offerpilot/backend/internal/executiontrace"
)

func TestServiceEmitsSafeExecutionStagesAcrossInterviewLifecycle(t *testing.T) {
	service := newTestService(groundedAgent(), NewMemoryStore())
	events := make([]executiontrace.Event, 0)
	ctx := executiontrace.WithSink(context.Background(), func(event executiontrace.Event) {
		events = append(events, event)
	})
	materials := standardMaterials()
	materials.JD.Text += "\nSECRET_JD_REQUIREMENT"
	materials.Resume.Text += "\nSECRET_RESUME_PROJECT SECRET_KNOWLEDGE_REFERENCE"

	started, err := service.Start(ctx, startRequest(2, FocusMixed, materials))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	first, err := service.Answer(ctx, answerRequest(started, "SECRET_CANDIDATE_ANSWER"))
	if err != nil {
		t.Fatalf("first Answer() error = %v", err)
	}
	if first.NextQuestion == nil {
		t.Fatal("first Answer() did not produce a next question")
	}
	secondRequest := AnswerRequest{
		Action: ActionAnswer, InterviewID: first.InterviewID, QuestionID: first.NextQuestion.ID, ClientAnswerID: "trace-answer-2",
		Answer: AnswerPayload{Text: "second answer", InputMode: InputModeText},
	}
	second, err := service.Answer(ctx, secondRequest)
	if err != nil || !second.ReportReady {
		t.Fatalf("second Answer() = %#v, %v", second, err)
	}
	if _, err := service.Report(ctx, ReportRequest{Action: ActionReport, InterviewID: second.InterviewID}); err != nil {
		t.Fatalf("Report() error = %v", err)
	}

	completedStages := make(map[string]bool)
	for _, event := range events {
		if event.Status == executiontrace.StatusCompleted {
			completedStages[event.Stage] = true
		}
	}
	for _, stage := range []string{"request", "materials", "interviewer", "persistence", "assessor", "policy", "planner", "reporter"} {
		if !completedStages[stage] {
			t.Errorf("missing completed trace stage %q; events = %#v", stage, events)
		}
	}

	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"SECRET_JD_REQUIREMENT", "SECRET_RESUME_PROJECT", "SECRET_KNOWLEDGE_REFERENCE", "SECRET_CANDIDATE_ANSWER",
	} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("service trace leaked %q: %s", secret, encoded)
		}
	}
}
