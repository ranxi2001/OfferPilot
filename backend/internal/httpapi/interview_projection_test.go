package httpapi

import (
	"testing"

	"offerpilot/backend/internal/interview"
)

func TestReportDimensionsMarkUncoveredAreasUnassessed(t *testing.T) {
	t.Parallel()

	turns := []interview.AnswerRecord{{
		Question: interview.Question{Kind: interview.QuestionProject},
		Assessment: interview.Assessment{
			Correctness: 4, Depth: 4, Specificity: 4, Ownership: 4, Metrics: 4, Tradeoffs: 4,
		},
	}}
	dimensions := reportDimensions(turns)
	byKey := make(map[string]webDimension, len(dimensions))
	for _, dimension := range dimensions {
		byKey[dimension.Key] = dimension
	}
	if byKey["knowledge_depth"].Assessed || byKey["knowledge_depth"].SampleCount != 0 {
		t.Fatalf("knowledge dimension should be unassessed: %+v", byKey["knowledge_depth"])
	}
	if !byKey["project_depth"].Assessed || byKey["project_depth"].Score != 80 {
		t.Fatalf("project dimension should be assessed: %+v", byKey["project_depth"])
	}
}

func TestReportReadinessRequiresJDAndProjectCoverage(t *testing.T) {
	t.Parallel()

	profile := interview.Profile{
		JD:     interview.JDProfile{Requirements: []interview.ProfilePoint{{ID: "jd-1", Label: "ColBERT"}}},
		Resume: interview.ResumeProfile{Projects: []interview.ProfilePoint{{ID: "project-1", Label: "Agent Router"}}},
	}
	turns := []interview.AnswerRecord{{Question: interview.Question{Kind: interview.QuestionProject}}}
	readiness := reportReadiness(
		85,
		profile,
		turns,
		[]webJDCoverage{{Requirement: "ColBERT", Status: "missing"}},
		[]webProjectCoverage{{Project: "Agent Router", Depth: 1}},
	)
	if readiness != "borderline" {
		t.Fatalf("readiness = %q, want borderline until JD is covered", readiness)
	}
}

func TestReportJDCoverageDeduplicatesFallbackRequirement(t *testing.T) {
	t.Parallel()

	point := interview.ProfilePoint{ID: "jd-1", Label: "负责 ColBERT 重排"}
	profile := interview.Profile{JD: interview.JDProfile{
		Requirements:     []interview.ProfilePoint{point},
		Responsibilities: []interview.ProfilePoint{point},
	}}
	coverage := reportJDCoverage(profile, nil)
	if len(coverage) != 1 {
		t.Fatalf("JD coverage rows = %d, want one deduplicated row: %+v", len(coverage), coverage)
	}
}

func TestQuestionProjectionDoesNotLeakKnowledgeReferenceAnswer(t *testing.T) {
	t.Parallel()

	refs := []interview.EvidenceRef{{
		SourceID: "knowledge:kb_colbert",
		Kind:     interview.SourceKnowledge,
		AnchorID: "knowledge:kb_colbert:block",
		Locator:  "question-block",
		Quote:    "知识主题：RAG\n问题：ColBERT 的 MaxSim 如何工作？\n参考内容：这是候选人在作答前不能看到的答案。",
	}}
	mapped := mapQuestionEvidenceRefs(refs)
	if len(mapped) != 1 || mapped[0].Excerpt != "问题：ColBERT 的 MaxSim 如何工作？" {
		t.Fatalf("unexpected public knowledge excerpt: %+v", mapped)
	}
}
