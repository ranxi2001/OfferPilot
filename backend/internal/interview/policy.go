package interview

func normalizeAssessment(assessment Assessment) Assessment {
	assessment.Correctness = clampScore(assessment.Correctness)
	assessment.Depth = clampScore(assessment.Depth)
	assessment.Specificity = clampScore(assessment.Specificity)
	assessment.Ownership = clampScore(assessment.Ownership)
	assessment.Metrics = clampScore(assessment.Metrics)
	assessment.Tradeoffs = clampScore(assessment.Tradeoffs)
	assessment.FactualErrors = nonNilStrings(assessment.FactualErrors)
	assessment.Strengths = nonNilStrings(assessment.Strengths)
	assessment.Gaps = nonNilStrings(assessment.Gaps)
	assessment.EvidenceRefs = nonNilEvidence(assessment.EvidenceRefs)
	assessment.ClaimChecks = nonNilClaimChecks(assessment.ClaimChecks)
	return assessment
}

func derivePolicy(session InterviewSession, assessment Assessment) PolicyDecision {
	current := *session.CurrentQuestion
	area := coveragePointByID(session.Profile, current.CoveragePointID).Area
	depth := current.Adaptation.Depth
	decision := PolicyDecision{
		Difficulty:      current.Difficulty,
		CoveragePointID: current.CoveragePointID,
		RootID:          current.RootID,
		FollowUpDepth:   depth,
	}

	if len(session.Answers)+1 >= session.Config.QuestionCount {
		decision.Action = PolicyComplete
		decision.Reason = "configured question count reached"
		return decision
	}

	if len(assessment.FactualErrors) > 0 || assessment.Correctness <= 1 {
		if depth < 2 {
			decision.Action = PolicyPrerequisite
			decision.Reason = "factual error detected; verify the prerequisite before continuing"
			decision.Difficulty = lowerDifficulty(current.Difficulty)
			decision.FollowUpDepth = depth + 1
			return decision
		}
		decision.Action = PolicyAdvance
		decision.Reason = "root follow-up limit reached after prerequisite remediation"
		decision.Difficulty = lowerDifficulty(current.Difficulty)
		decision.FollowUpDepth = 0
		return decision
	}

	if area == FocusProjects && hasClaimVerdict(assessment, ClaimContradicted) && depth < 2 {
		decision.Action = PolicyFollowUp
		decision.Reason = "answer contradicts supplied resume evidence; request reconciliation"
		decision.FollowUpAxis = "verification"
		decision.FollowUpDepth = depth + 1
		return decision
	}

	if vagueAssessmentForArea(area, assessment) && depth < 2 {
		axis := chooseFollowUpAxis(session, assessment, area)
		decision.Action = PolicyFollowUp
		decision.Reason = "answer is too general; request verifiable detail"
		decision.FollowUpAxis = axis
		decision.FollowUpDepth = depth + 1
		return decision
	}

	decision.Action = PolicyAdvance
	decision.FollowUpDepth = 0
	if strongAssessmentForArea(area, assessment) {
		decision.Reason = "strong answer; rotate coverage and increase difficulty"
		decision.Difficulty = raiseDifficulty(current.Difficulty)
	} else if depth >= 2 {
		decision.Reason = "root follow-up limit reached; rotate coverage"
	} else {
		decision.Reason = "answer is sufficient; rotate to the next coverage point"
	}
	return decision
}

func vagueAssessment(assessment Assessment) bool {
	return assessment.Specificity <= 2 || assessment.Ownership <= 2 || assessment.Metrics <= 2 || assessment.Tradeoffs <= 2
}

func vagueAssessmentForArea(area Focus, assessment Assessment) bool {
	if area == FocusKnowledge {
		return assessment.Correctness <= 2 || assessment.Depth <= 2 || assessment.Specificity <= 2
	}
	return vagueAssessment(assessment)
}

func strongAssessment(assessment Assessment) bool {
	return assessment.Correctness >= 4 && assessment.Depth >= 4 && assessment.Specificity >= 4 && len(assessment.FactualErrors) == 0
}

func strongAssessmentForArea(area Focus, assessment Assessment) bool {
	if !strongAssessment(assessment) {
		return false
	}
	if area == FocusProjects {
		return assessment.Ownership >= 4 && assessment.Metrics >= 3 && assessment.Tradeoffs >= 3
	}
	return true
}

func hasClaimVerdict(assessment Assessment, verdict ClaimVerdict) bool {
	for _, check := range assessment.ClaimChecks {
		if check.Verdict == verdict {
			return true
		}
	}
	return false
}

func chooseFollowUpAxis(session InterviewSession, assessment Assessment, area Focus) string {
	used := make(map[string]struct{})
	for _, record := range session.Answers {
		if record.Question.RootID == session.CurrentQuestion.RootID && record.Decision.FollowUpAxis != "" {
			used[record.Decision.FollowUpAxis] = struct{}{}
		}
	}
	if axis := session.CurrentQuestion.Adaptation.FollowUpAxis; axis != "" {
		used[axis] = struct{}{}
	}
	var candidates []struct {
		name  string
		score int
	}
	if area == FocusKnowledge {
		candidates = []struct {
			name  string
			score int
		}{
			{name: "principle", score: assessment.Correctness},
			{name: "boundary", score: assessment.Depth},
			{name: "example", score: assessment.Specificity},
		}
	} else {
		candidates = []struct {
			name  string
			score int
		}{
			{name: "ownership", score: assessment.Ownership},
			{name: "metrics", score: assessment.Metrics},
			{name: "tradeoff", score: assessment.Tradeoffs},
		}
	}
	for _, candidate := range candidates {
		if candidate.score <= 2 {
			if _, exists := used[candidate.name]; !exists {
				return candidate.name
			}
		}
	}
	for _, candidate := range candidates {
		if _, exists := used[candidate.name]; !exists {
			return candidate.name
		}
	}
	return "specificity"
}

func questionKind(point CoveragePoint, decision PolicyDecision) QuestionKind {
	switch decision.Action {
	case PolicyPrerequisite:
		return QuestionPrerequisite
	case PolicyFollowUp:
		return QuestionFollowUp
	default:
		if point.Area == FocusProjects {
			return QuestionProject
		}
		return QuestionKnowledge
	}
}

func lowerDifficulty(value Difficulty) Difficulty {
	switch value {
	case DifficultyHard:
		return DifficultyMedium
	case DifficultyMedium:
		return DifficultyEasy
	default:
		return DifficultyEasy
	}
}

func raiseDifficulty(value Difficulty) Difficulty {
	switch value {
	case DifficultyEasy:
		return DifficultyMedium
	case DifficultyMedium:
		return DifficultyHard
	default:
		return DifficultyHard
	}
}

func clampScore(value int) int {
	if value < 1 {
		return 1
	}
	if value > 5 {
		return 5
	}
	return value
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return make([]string, 0)
	}
	return values
}

func nonNilEvidence(values []EvidenceRef) []EvidenceRef {
	if values == nil {
		return make([]EvidenceRef, 0)
	}
	return values
}

func nonNilClaimChecks(values []ClaimCheck) []ClaimCheck {
	if values == nil {
		return make([]ClaimCheck, 0)
	}
	return values
}

func appendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}
