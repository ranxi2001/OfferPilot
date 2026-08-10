package httpapi

import (
	"encoding/json"
	"strings"
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

func TestReportReadinessRequiresBalancedDeepSampling(t *testing.T) {
	t.Parallel()

	profile := interview.Profile{
		JD: interview.JDProfile{Requirements: []interview.ProfilePoint{
			{ID: "jd-1", Label: "ColBERT"},
			{ID: "jd-2", Label: "Go concurrency"},
		}},
		Resume: interview.ResumeProfile{Projects: []interview.ProfilePoint{{ID: "project-1", Label: "Agent Router"}}},
	}
	knowledgeTurn := interview.AnswerRecord{Question: interview.Question{Kind: interview.QuestionKnowledge}}
	projectTurn := interview.AnswerRecord{Question: interview.Question{Kind: interview.QuestionProject}}

	tests := []struct {
		name     string
		turns    []interview.AnswerRecord
		jd       []webJDCoverage
		projects []webProjectCoverage
		want     string
	}{
		{
			name:     "one shallow sample per area is insufficient",
			turns:    []interview.AnswerRecord{knowledgeTurn, projectTurn},
			jd:       []webJDCoverage{{Requirement: "ColBERT", Status: "covered"}, {Requirement: "Go concurrency", Status: "covered"}},
			projects: []webProjectCoverage{{Project: "Agent Router", Depth: 1}},
			want:     "borderline",
		},
		{
			name:     "partial JD evidence is insufficient",
			turns:    []interview.AnswerRecord{knowledgeTurn, knowledgeTurn, projectTurn, projectTurn},
			jd:       []webJDCoverage{{Requirement: "ColBERT", Status: "covered"}, {Requirement: "Go concurrency", Status: "partial"}},
			projects: []webProjectCoverage{{Project: "Agent Router", Depth: 2}},
			want:     "borderline",
		},
		{
			name:     "balanced evidence can be ready",
			turns:    []interview.AnswerRecord{knowledgeTurn, knowledgeTurn, projectTurn, projectTurn},
			jd:       []webJDCoverage{{Requirement: "ColBERT", Status: "covered"}, {Requirement: "Go concurrency", Status: "covered"}},
			projects: []webProjectCoverage{{Project: "Agent Router", Depth: 2}},
			want:     "ready",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := reportReadiness(85, profile, test.turns, test.jd, test.projects); got != test.want {
				t.Fatalf("readiness = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCachedReportProjectionRedactsNarrativeWithoutDroppingRisk(t *testing.T) {
	t.Parallel()

	const secret = "CACHED_PRIVATE_REFERENCE_91AF"
	const crossAnchorSecret = "CROSS_ANCHOR_PRIVATE_REFERENCE_42B7"
	knowledgeRef := func(id, question, reference string) interview.EvidenceRef {
		return interview.EvidenceRef{
			SourceID: "knowledge:" + id, Kind: interview.SourceKnowledge, AnchorID: "knowledge:" + id + ":block",
			Locator: "question-block", Quote: "问题：" + question + "\n参考内容：" + reference + "\n参考答案：" + reference,
		}
	}
	firstKnowledge := knowledgeRef("one", "解释 late interaction", secret)
	secondKnowledge := knowledgeRef("two", "解释 deadline 降级", crossAnchorSecret)
	projectRef := interview.EvidenceRef{
		SourceID: "resume", Kind: interview.SourceResume, AnchorID: "resume:project", Locator: "segment:1", Quote: "OfferPilot 项目",
	}
	strong := func(ref interview.EvidenceRef) interview.Assessment {
		return interview.Assessment{
			Correctness: 5, Depth: 5, Specificity: 5, Ownership: 5, Metrics: 5, Tradeoffs: 5,
			Strengths: []string{"证据充分"}, EvidenceRefs: []interview.EvidenceRef{ref},
		}
	}
	firstAssessment := strong(firstKnowledge)
	firstAssessment.ClaimChecks = []interview.ClaimCheck{{
		Claim: "参考答案：" + secret, Verdict: interview.ClaimContradicted, EvidenceRefs: []interview.EvidenceRef{firstKnowledge},
	}}
	projectAssessment := strong(projectRef)
	projectAssessment.Gaps = []string{"reference content: " + secret}
	report := interview.Report{
		OverallScore: 95,
		Summary:      "参考答案：" + secret,
		Strengths:    []string{"reference content: " + secret},
		Gaps:         []string{"参考内容：" + secret},
		EvidenceRefs: []interview.EvidenceRef{firstKnowledge},
		Profile: interview.Profile{
			JD: interview.JDProfile{Requirements: []interview.ProfilePoint{
				{ID: "jd-1", Label: "Late interaction", EvidenceRefs: []interview.EvidenceRef{firstKnowledge}},
				{ID: "jd-2", Label: "Deadline degradation", EvidenceRefs: []interview.EvidenceRef{secondKnowledge}},
			}},
			Resume: interview.ResumeProfile{Projects: []interview.ProfilePoint{{ID: "project-1", Label: "OfferPilot", EvidenceRefs: []interview.EvidenceRef{projectRef}}}},
			Coverage: []interview.CoveragePoint{
				{ID: "knowledge-1", Area: interview.FocusKnowledge, EvidenceRefs: []interview.EvidenceRef{firstKnowledge}},
				{ID: "knowledge-2", Area: interview.FocusKnowledge, EvidenceRefs: []interview.EvidenceRef{secondKnowledge}},
				{ID: "project-1", Area: interview.FocusProjects, EvidenceRefs: []interview.EvidenceRef{projectRef}},
			},
		},
		Turns: []interview.AnswerRecord{
			{Question: interview.Question{ID: "q1", Text: "泄漏的旁题内容 " + crossAnchorSecret, Kind: interview.QuestionKnowledge, EvidenceRefs: []interview.EvidenceRef{firstKnowledge}}, Assessment: firstAssessment},
			{Question: interview.Question{ID: "q2", Text: "解释 deadline 降级", Kind: interview.QuestionKnowledge, EvidenceRefs: []interview.EvidenceRef{secondKnowledge}}, Assessment: strong(secondKnowledge)},
			{Question: interview.Question{ID: "q3", Text: "项目职责", Kind: interview.QuestionProject, EvidenceRefs: []interview.EvidenceRef{projectRef}}, Assessment: projectAssessment},
			{Question: interview.Question{ID: "q4", Text: "项目取舍", Kind: interview.QuestionProject, EvidenceRefs: []interview.EvidenceRef{projectRef}}, Assessment: strong(projectRef)},
		},
	}

	mapped := mapReport("interview-cached", report)
	if mapped.Readiness != "borderline" {
		t.Fatalf("readiness = %q, want borderline because contradicted risk must survive display redaction", mapped.Readiness)
	}
	encoded, err := json.Marshal(mapped)
	if err != nil {
		t.Fatal(err)
	}
	assertNoKnowledgeReferenceLeak(t, "cached report", string(encoded), secret)
	assertNoKnowledgeReferenceLeak(t, "cached report cross-anchor question", string(encoded), crossAnchorSecret)
}

func TestDeferredFeedbackProjectionHasStableEmptyShape(t *testing.T) {
	t.Parallel()

	feedback := deferredFeedback("question-1")
	if !feedback.Deferred || feedback.Verdict != "deferred" || feedback.KnowledgeVerdict != "deferred" {
		t.Fatalf("unexpected deferred feedback: %+v", feedback)
	}
	if feedback.Strengths == nil || feedback.Gaps == nil || feedback.ClaimChecks == nil {
		t.Fatalf("deferred feedback collections must be non-nil: %+v", feedback)
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

func TestKnowledgeQuestionTopicDoesNotLeakReferenceAnswer(t *testing.T) {
	t.Parallel()

	const quote = "知识主题：RAG\n问题：MaxSim 如何工作？\n参考内容：TOPIC_CONTENT_SECRET\n参考答案：TOPIC_ANSWER_SECRET"
	profile := interview.Profile{Coverage: []interview.CoveragePoint{{
		ID:    "knowledge:colbert",
		Area:  interview.FocusKnowledge,
		Label: quote,
	}}}
	question := interview.Question{
		ID:              "question-1",
		Kind:            interview.QuestionKnowledge,
		CoveragePointID: "knowledge:colbert",
		EvidenceRefs: []interview.EvidenceRef{{
			SourceID: "knowledge:kb_colbert",
			Kind:     interview.SourceKnowledge,
			AnchorID: "knowledge:kb_colbert:block",
			Quote:    quote,
		}},
	}

	mapped := mapQuestion(question, 1, &profile)
	assertNoKnowledgeReferenceLeak(t, "question.topic", mapped.Topic, "TOPIC_CONTENT_SECRET", "TOPIC_ANSWER_SECRET")
}

func TestQuestionEvidenceExcerptDoesNotLeakReferenceAnswer(t *testing.T) {
	t.Parallel()

	refs := []interview.EvidenceRef{{
		SourceID: "knowledge:kb_colbert",
		Kind:     interview.SourceKnowledge,
		AnchorID: "knowledge:kb_colbert:block",
		Locator:  "question-block",
		Quote:    "问题：MaxSim 如何工作？ 参考内容：EXCERPT_CONTENT_SECRET 参考答案：EXCERPT_ANSWER_SECRET",
	}}

	mapped := mapQuestionEvidenceRefs(refs)
	if len(mapped) != 1 {
		t.Fatalf("question evidence refs = %d, want 1", len(mapped))
	}
	assertNoKnowledgeReferenceLeak(t, "question.evidenceRefs[0].excerpt", mapped[0].Excerpt, "EXCERPT_CONTENT_SECRET", "EXCERPT_ANSWER_SECRET")
}

func TestFeedbackEvidenceDoesNotLeakKnowledgeReferenceAnswer(t *testing.T) {
	t.Parallel()

	assessment := interview.Assessment{
		Correctness: 3,
		Depth:       3,
		Specificity: 3,
		Tradeoffs:   3,
		EvidenceRefs: []interview.EvidenceRef{{
			SourceID: "knowledge:kb_colbert",
			Kind:     interview.SourceKnowledge,
			AnchorID: "knowledge:kb_colbert:block",
			Locator:  "question-block",
			Quote:    "问题：MaxSim 如何工作？\n参考内容：FEEDBACK_CONTENT_SECRET\n参考答案：FEEDBACK_ANSWER_SECRET",
		}},
	}

	mapped := mapFeedback("question-1", assessment, "", "knowledge")
	for _, ref := range mapped.EvidenceRefs {
		assertNoKnowledgeReferenceLeak(
			t,
			"feedback.evidenceRefs[].excerpt",
			ref.Excerpt,
			"FEEDBACK_CONTENT_SECRET",
			"FEEDBACK_ANSWER_SECRET",
		)
	}
}

func TestAdaptationReasonDoesNotLeakKnowledgeReferenceAnswer(t *testing.T) {
	t.Parallel()

	question := interview.Question{
		ID:   "question-2",
		Kind: interview.QuestionKnowledge,
		Adaptation: interview.QuestionAdaptation{
			Trigger: interview.PolicyAdvance,
			Reason:  "切换主题。参考内容：ADAPTATION_CONTENT_SECRET；参考答案：ADAPTATION_ANSWER_SECRET",
		},
	}

	mapped := mapQuestion(question, 2, nil)
	if mapped.Adaptation == nil {
		return
	}
	assertNoKnowledgeReferenceLeak(t, "question.adaptation.basedOn", mapped.Adaptation.BasedOn, "ADAPTATION_CONTENT_SECRET", "ADAPTATION_ANSWER_SECRET")
}

func assertNoKnowledgeReferenceLeak(t *testing.T, field, value string, secrets ...string) {
	t.Helper()

	for _, forbidden := range append([]string{"参考内容", "参考答案"}, secrets...) {
		if strings.Contains(value, forbidden) {
			t.Errorf("%s leaked %q: %q", field, forbidden, value)
		}
	}
}
