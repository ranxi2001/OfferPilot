package interview

import (
	"context"
	"errors"
	"fmt"
	"strings"

	typedprofile "offerpilot/backend/internal/profile"
)

type typedProfileBuilder struct {
	extractor typedprofile.ProfileExtractor
}

func newDefaultProfileBuilder() ProfileBuilder {
	return typedProfileBuilder{extractor: typedprofile.NewDeterministicExtractor()}
}

func (b typedProfileBuilder) Build(
	ctx context.Context,
	materials MaterialsInput,
	knowledge []KnowledgeDocument,
	focus Focus,
) (Profile, SourceIndex, error) {
	return ExtractTypedProfile(ctx, b.extractor, materials, knowledge, focus)
}

// ExtractTypedProfile adapts the shared typed profile domain to the existing
// interview aggregate. The returned SourceIndex is rebuilt from canonical
// extractor anchors; model-supplied EvidenceRef fields are never trusted as
// the source of truth.
func ExtractTypedProfile(
	ctx context.Context,
	extractor typedprofile.ProfileExtractor,
	materials MaterialsInput,
	knowledge []KnowledgeDocument,
	focus Focus,
) (Profile, SourceIndex, error) {
	if extractor == nil {
		return Profile{}, SourceIndex{}, errors.New("interview: typed profile extractor is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Profile{}, SourceIndex{}, err
	}

	input := typedProfileInput(materials)
	request := typedprofile.AgentRequest{Documents: make([]typedprofile.SourceDocument, 0), Anchors: make([]typedprofile.SourceAnchor, 0)}
	extracted := typedprofile.Profile{}
	if strings.TrimSpace(input.JD.Text) != "" || strings.TrimSpace(input.Resume.Text) != "" {
		var err error
		request, err = typedprofile.BuildAgentRequest(input)
		if err != nil {
			return Profile{}, SourceIndex{}, fmt.Errorf("interview: build typed profile evidence: %w", err)
		}
		extracted, err = extractor.Extract(ctx, input)
		if err != nil {
			return Profile{}, SourceIndex{}, fmt.Errorf("interview: extract typed profile: %w", err)
		}
		if err := typedprofile.Validate(request, extracted); err != nil {
			return Profile{}, SourceIndex{}, fmt.Errorf("interview: validate typed profile: %w", err)
		}
	}

	index, err := typedInterviewSourceIndex(request, input)
	if err != nil {
		return Profile{}, SourceIndex{}, err
	}
	result, jdCoverage, projectCoverage, err := adaptTypedProfile(extracted, index)
	if err != nil {
		return Profile{}, SourceIndex{}, err
	}

	knowledgeRefs := mergeKnowledgeDocuments(&index, knowledge, 0)
	knowledgeCoverage := make([]CoveragePoint, 0, len(knowledgeRefs))
	for position, ref := range knowledgeRefs {
		anchor, exists := index.Anchors[ref.AnchorID]
		if !exists {
			return Profile{}, SourceIndex{}, fmt.Errorf("interview: missing merged knowledge anchor %q", ref.AnchorID)
		}
		knowledgeCoverage = append(knowledgeCoverage, CoveragePoint{
			ID:           fmt.Sprintf("typed-knowledge-%03d", position+1),
			Area:         FocusKnowledge,
			Label:        publicAnchorLabel(anchor),
			EvidenceRefs: []EvidenceRef{evidenceFromAnchor(anchor)},
		})
	}

	knowledgeOriented := append(jdCoverage, knowledgeCoverage...)
	switch focus {
	case FocusProjects:
		result.Coverage = append(result.Coverage, projectCoverage...)
		result.Coverage = append(result.Coverage, knowledgeOriented...)
	case FocusKnowledge:
		result.Coverage = append(result.Coverage, knowledgeOriented...)
		result.Coverage = append(result.Coverage, projectCoverage...)
	default:
		result.Coverage = interleaveCoverage(projectCoverage, knowledgeOriented)
	}
	if err := validateAdaptedTypedProfile(index, result); err != nil {
		return Profile{}, SourceIndex{}, err
	}
	return result, index, nil
}

var _ ProfileBuilder = typedProfileBuilder{}

func typedProfileInput(materials MaterialsInput) typedprofile.Input {
	input := typedprofile.Input{}
	if materials.JD != nil {
		input.JD = typedprofile.DocumentInput{
			SourceID: "jd", Name: strings.TrimSpace(materials.JD.Name), Text: materials.JD.Text,
		}
	}
	if materials.Resume != nil {
		input.Resume = typedprofile.DocumentInput{
			SourceID: "resume", Name: strings.TrimSpace(materials.Resume.Name), Text: materials.Resume.Text,
		}
	}
	return input
}

func typedInterviewSourceIndex(request typedprofile.AgentRequest, input typedprofile.Input) (SourceIndex, error) {
	index := SourceIndex{
		Documents: make(map[string]SourceDocument, len(request.Documents)),
		Anchors:   make(map[string]SourceAnchor, len(request.Anchors)),
		Order:     make([]string, 0, len(request.Anchors)),
	}
	for _, document := range request.Documents {
		kind, err := typedInterviewSourceKind(document.Kind)
		if err != nil {
			return SourceIndex{}, err
		}
		content := input.JD.Text
		if document.Kind == typedprofile.SourceResume {
			content = input.Resume.Text
		}
		index.Documents[document.ID] = SourceDocument{
			ID: document.ID, Kind: kind, Name: document.Name, Content: content,
		}
	}
	for _, anchor := range request.Anchors {
		kind, err := typedInterviewSourceKind(anchor.Kind)
		if err != nil {
			return SourceIndex{}, err
		}
		if _, exists := index.Anchors[anchor.ID]; exists {
			return SourceIndex{}, fmt.Errorf("interview: duplicate typed anchor %q", anchor.ID)
		}
		index.Anchors[anchor.ID] = SourceAnchor{
			ID: anchor.ID, SourceID: anchor.SourceID, Kind: kind,
			Locator: anchor.Locator, Text: anchor.Text,
		}
		index.Order = append(index.Order, anchor.ID)
	}
	return index, nil
}

func typedInterviewSourceKind(kind typedprofile.SourceKind) (SourceKind, error) {
	switch kind {
	case typedprofile.SourceJD:
		return SourceJD, nil
	case typedprofile.SourceResume:
		return SourceResume, nil
	default:
		return "", fmt.Errorf("interview: unsupported typed profile source kind %q", kind)
	}
}

func adaptTypedProfile(extracted typedprofile.Profile, index SourceIndex) (Profile, []CoveragePoint, []CoveragePoint, error) {
	result := Profile{
		JD:       JDProfile{Requirements: make([]ProfilePoint, 0), Responsibilities: make([]ProfilePoint, 0)},
		Resume:   ResumeProfile{Skills: make([]string, 0), Projects: make([]ProfilePoint, 0)},
		Coverage: make([]CoveragePoint, 0),
	}
	if extracted.Job.Title != nil {
		result.JD.Title = extracted.Job.Title.Value
	}
	if extracted.Candidate.Headline != nil {
		result.Resume.Headline = extracted.Candidate.Headline.Value
	}

	for _, group := range [][]typedprofile.Fact{extracted.Job.MustHave, extracted.Job.NiceToHave} {
		for _, fact := range group {
			point, err := typedProfilePoint("typed-jd-requirement", len(result.JD.Requirements)+1, fact, index)
			if err != nil {
				return Profile{}, nil, nil, err
			}
			result.JD.Requirements = append(result.JD.Requirements, point)
		}
	}
	for _, fact := range extracted.Job.Responsibilities {
		point, err := typedProfilePoint("typed-jd-responsibility", len(result.JD.Responsibilities)+1, fact, index)
		if err != nil {
			return Profile{}, nil, nil, err
		}
		result.JD.Responsibilities = append(result.JD.Responsibilities, point)
	}

	seenSkills := make(map[string]struct{})
	for _, fact := range extracted.Candidate.Skills {
		value := strings.TrimSpace(fact.Value)
		key := strings.ToLower(value)
		if value == "" {
			continue
		}
		if _, exists := seenSkills[key]; exists {
			continue
		}
		seenSkills[key] = struct{}{}
		result.Resume.Skills = append(result.Resume.Skills, value)
	}

	for position, project := range extracted.Candidate.Projects {
		refs := append([]typedprofile.EvidenceRef{}, project.Name.EvidenceRefs...)
		for _, group := range [][]typedprofile.Fact{
			project.Responsibilities, project.Metrics, project.Technologies, project.Highlights,
		} {
			for _, fact := range group {
				refs = append(refs, fact.EvidenceRefs...)
			}
		}
		canonical, err := typedCanonicalEvidence(index, refs)
		if err != nil {
			return Profile{}, nil, nil, err
		}
		result.Resume.Projects = append(result.Resume.Projects, ProfilePoint{
			ID: fmt.Sprintf("typed-resume-project-%03d", position+1), Label: concise(project.Name.Value, 120), EvidenceRefs: canonical,
		})
	}

	jdCoverage := make([]CoveragePoint, 0)
	projectCoverage := make([]CoveragePoint, 0)
	seenJDCoverage := make(map[string]struct{})
	seenProjectCoverage := make(map[string]struct{})
	for _, group := range [][]typedprofile.Fact{
		extracted.Job.MustHave,
		extracted.Job.Responsibilities,
		extracted.Job.NiceToHave,
		extracted.Job.SenioritySignals,
		extracted.Job.BusinessConstraints,
		extracted.Job.TechnicalTopics,
	} {
		for _, fact := range group {
			var err error
			jdCoverage, err = appendTypedCoverage(jdCoverage, "typed-jd", FocusKnowledge, fact, index, seenJDCoverage)
			if err != nil {
				return Profile{}, nil, nil, err
			}
		}
	}
	for position, project := range extracted.Candidate.Projects {
		refs := append([]typedprofile.EvidenceRef{}, project.Name.EvidenceRefs...)
		for _, group := range [][]typedprofile.Fact{
			project.Responsibilities, project.Metrics, project.Technologies, project.Highlights,
		} {
			for _, fact := range group {
				refs = append(refs, fact.EvidenceRefs...)
			}
		}
		canonical, err := typedCanonicalEvidence(index, refs)
		if err != nil {
			return Profile{}, nil, nil, err
		}
		point := CoveragePoint{
			ID: fmt.Sprintf("typed-project-%03d", position+1), Area: FocusProjects,
			Label: concise(project.Name.Value, 120), EvidenceRefs: canonical,
		}
		signature := typedCoverageSignature(point.Label, point.EvidenceRefs)
		if _, duplicate := seenProjectCoverage[signature]; !duplicate {
			seenProjectCoverage[signature] = struct{}{}
			projectCoverage = append(projectCoverage, point)
		}
	}
	for _, group := range [][]typedprofile.Fact{
		extracted.Candidate.Responsibilities,
		extracted.Candidate.Metrics,
	} {
		for _, fact := range group {
			var err error
			projectCoverage, err = appendTypedCoverage(projectCoverage, "typed-resume", FocusProjects, fact, index, seenProjectCoverage)
			if err != nil {
				return Profile{}, nil, nil, err
			}
		}
	}
	for _, fact := range extracted.Candidate.Skills {
		var err error
		jdCoverage, err = appendTypedCoverage(jdCoverage, "typed-skill", FocusKnowledge, fact, index, seenJDCoverage)
		if err != nil {
			return Profile{}, nil, nil, err
		}
	}
	if len(jdCoverage) == 0 && extracted.Job.Title != nil {
		var err error
		jdCoverage, err = appendTypedCoverage(jdCoverage, "typed-jd", FocusKnowledge, *extracted.Job.Title, index, seenJDCoverage)
		if err != nil {
			return Profile{}, nil, nil, err
		}
	}
	if len(projectCoverage) == 0 && extracted.Candidate.Headline != nil {
		var err error
		projectCoverage, err = appendTypedCoverage(projectCoverage, "typed-resume", FocusProjects, *extracted.Candidate.Headline, index, seenProjectCoverage)
		if err != nil {
			return Profile{}, nil, nil, err
		}
	}
	return result, jdCoverage, projectCoverage, nil
}

func typedProfilePoint(prefix string, position int, fact typedprofile.Fact, index SourceIndex) (ProfilePoint, error) {
	refs, err := typedCanonicalEvidence(index, fact.EvidenceRefs)
	if err != nil {
		return ProfilePoint{}, err
	}
	return ProfilePoint{
		ID: fmt.Sprintf("%s-%03d", prefix, position), Label: concise(fact.Value, 120), EvidenceRefs: refs,
	}, nil
}

func appendTypedCoverage(
	points []CoveragePoint,
	prefix string,
	area Focus,
	fact typedprofile.Fact,
	index SourceIndex,
	seen map[string]struct{},
) ([]CoveragePoint, error) {
	refs, err := typedCanonicalEvidence(index, fact.EvidenceRefs)
	if err != nil {
		return nil, err
	}
	label := concise(fact.Value, 120)
	signature := typedCoverageSignature(label, refs)
	if _, duplicate := seen[signature]; duplicate {
		return points, nil
	}
	seen[signature] = struct{}{}
	return append(points, CoveragePoint{
		ID: fmt.Sprintf("%s-%03d", prefix, len(points)+1), Area: area, Label: label, EvidenceRefs: refs,
	}), nil
}

func typedCanonicalEvidence(index SourceIndex, refs []typedprofile.EvidenceRef) ([]EvidenceRef, error) {
	result := make([]EvidenceRef, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if _, duplicate := seen[ref.AnchorID]; duplicate {
			continue
		}
		anchor, exists := index.Anchors[ref.AnchorID]
		if !exists {
			return nil, fmt.Errorf("interview: typed profile references unknown anchor %q", ref.AnchorID)
		}
		seen[ref.AnchorID] = struct{}{}
		result = append(result, evidenceFromAnchor(anchor))
	}
	if len(result) == 0 {
		return nil, errors.New("interview: typed profile point requires evidence")
	}
	return result, nil
}

func typedCoverageSignature(label string, refs []EvidenceRef) string {
	parts := make([]string, 0, len(refs)+1)
	parts = append(parts, strings.ToLower(strings.Join(strings.Fields(label), " ")))
	for _, ref := range refs {
		parts = append(parts, ref.AnchorID)
	}
	return strings.Join(parts, "|")
}

func validateAdaptedTypedProfile(index SourceIndex, result Profile) error {
	for _, group := range [][]ProfilePoint{result.JD.Requirements, result.JD.Responsibilities, result.Resume.Projects} {
		for _, point := range group {
			if err := validateEvidenceRefs(index, point.EvidenceRefs, nil, true); err != nil {
				return fmt.Errorf("interview: validate adapted profile point %q: %w", point.ID, err)
			}
		}
	}
	for _, point := range result.Coverage {
		if err := validateEvidenceRefs(index, point.EvidenceRefs, nil, true); err != nil {
			return fmt.Errorf("interview: validate adapted coverage point %q: %w", point.ID, err)
		}
	}
	return nil
}
