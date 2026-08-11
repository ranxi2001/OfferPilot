package interview

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSnapshotRestoresPublicCurrentQuestionAndTurns(t *testing.T) {
	const privateReference = "SNAPSHOT_PRIVATE_REFERENCE_6D2A"
	documents := []KnowledgeDocument{{
		ID: "snapshot", Title: "Snapshot knowledge",
		Content: "问题：如何设计幂等回答？\n参考内容：" + privateReference,
	}}
	agent := groundedAgent()
	agent.assessFn = func(request AssessAnswerRequest, _ int) (Assessment, error) {
		return Assessment{
			Correctness: 4, Depth: 4, Specificity: 3, Ownership: 3, Metrics: 2, Tradeoffs: 4,
			Strengths: []string{"说明了幂等键"}, Gaps: []string{"需要补充冲突语义"},
			EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])},
		}, nil
	}
	store := NewMemoryStore()
	service := NewService(Dependencies{
		Agent: agent, Retriever: staticKnowledgeRetriever(documents), Store: store,
		Clock: fixedClock{value: time.Now()}, IDs: &sequenceIDs{},
	})
	started, err := service.Start(context.Background(), startRequest(2, FocusKnowledge, MaterialsInput{}))
	if err != nil {
		t.Fatal(err)
	}
	answered, err := service.Answer(context.Background(), answerRequest(started, "使用客户端幂等键与请求哈希。"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(context.Background(), started.InterviewID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentQuestion == nil || answered.NextQuestion == nil || snapshot.CurrentQuestion.ID != answered.NextQuestion.ID {
		t.Fatalf("current question was not restored: %+v", snapshot.CurrentQuestion)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].Answer.Text == "" || snapshot.Turns[0].Feedback.Summary == "" {
		t.Fatalf("turns were not restored: %+v", snapshot.Turns)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), privateReference) || strings.Contains(string(encoded), "参考内容") {
		t.Fatalf("snapshot leaked private knowledge: %s", encoded)
	}
}

func TestSessionEventsReturnsMetadataWithoutPayload(t *testing.T) {
	store := openTestSQLiteStore(t)
	service := NewService(Dependencies{Agent: groundedAgent(), Store: store, Clock: fixedClock{value: time.Now()}, IDs: &sequenceIDs{}})
	started, err := service.Start(context.Background(), startRequest(1, FocusProjects, standardMaterials()))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.AppendSessionEvent(context.Background(), SessionEventSpec{
		EventID: "event-safe", SessionID: started.InterviewID, CommandID: "command-safe",
		Type: "answer.accepted", Payload: json.RawMessage(`{"private":"must-not-be-returned"}`),
	}); err != nil {
		t.Fatal(err)
	}
	page, err := service.SessionEvents(context.Background(), started.InterviewID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.NextSequence != 1 || strings.Contains(string(encoded), "must-not-be-returned") {
		t.Fatalf("event page = %s", encoded)
	}
}
