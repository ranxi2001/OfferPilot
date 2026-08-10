package interview

import "testing"

func TestKnowledgeGapUsesKnowledgeFollowUpAxis(t *testing.T) {
	t.Parallel()

	session := policySession(FocusKnowledge)
	decision := derivePolicy(session, Assessment{
		Correctness: 4,
		Depth:       2,
		Specificity: 4,
		Ownership:   1,
		Metrics:     1,
		Tradeoffs:   4,
	})
	if decision.Action != PolicyFollowUp || decision.FollowUpAxis != "boundary" {
		t.Fatalf("decision = %+v, want knowledge boundary follow-up", decision)
	}
}

func TestProjectGapIsNotPromotedToStrongAnswer(t *testing.T) {
	t.Parallel()

	session := policySession(FocusProjects)
	decision := derivePolicy(session, Assessment{
		Correctness: 5,
		Depth:       5,
		Specificity: 5,
		Ownership:   1,
		Metrics:     1,
		Tradeoffs:   5,
	})
	if decision.Action != PolicyFollowUp || decision.FollowUpAxis != "ownership" {
		t.Fatalf("decision = %+v, want ownership follow-up", decision)
	}
}

func TestContradictedProjectClaimTriggersVerification(t *testing.T) {
	t.Parallel()

	session := policySession(FocusProjects)
	decision := derivePolicy(session, Assessment{
		Correctness: 4,
		Depth:       4,
		Specificity: 4,
		Ownership:   4,
		Metrics:     4,
		Tradeoffs:   4,
		ClaimChecks: []ClaimCheck{{Claim: "延迟口径", Verdict: ClaimContradicted}},
	})
	if decision.Action != PolicyFollowUp || decision.FollowUpAxis != "verification" {
		t.Fatalf("decision = %+v, want evidence verification", decision)
	}
}

func TestKnowledgeScoreDoesNotPenalizeProjectOnlyRubricFields(t *testing.T) {
	t.Parallel()

	records := []AnswerRecord{{
		Question: Question{Kind: QuestionKnowledge},
		Assessment: Assessment{
			Correctness: 5,
			Depth:       5,
			Specificity: 5,
			Ownership:   1,
			Metrics:     1,
			Tradeoffs:   5,
		},
	}}
	if score := scoreReport(records); score != 100 {
		t.Fatalf("knowledge score = %d, want 100 without project-only ownership/metrics", score)
	}
}

func TestKnowledgeFollowUpUsesKnowledgeRubric(t *testing.T) {
	t.Parallel()

	records := []AnswerRecord{{
		Question: Question{
			Kind: QuestionFollowUp,
			EvidenceRefs: []EvidenceRef{{
				Kind: SourceKnowledge,
			}},
		},
		Assessment: Assessment{
			Correctness: 5,
			Depth:       5,
			Specificity: 5,
			Ownership:   1,
			Metrics:     1,
			Tradeoffs:   5,
		},
	}}
	if score := scoreReport(records); score != 100 {
		t.Fatalf("knowledge follow-up score = %d, want 100 without project-only ownership/metrics", score)
	}
}

func policySession(area Focus) InterviewSession {
	return InterviewSession{
		Config: InterviewConfig{QuestionCount: 3},
		Profile: Profile{Coverage: []CoveragePoint{{
			ID: "coverage-1", Area: area,
		}}},
		CurrentQuestion: &Question{
			ID: "question-1", RootID: "root-1", CoveragePointID: "coverage-1",
			Difficulty: DifficultyMedium,
		},
	}
}
