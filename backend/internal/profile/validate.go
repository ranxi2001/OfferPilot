package profile

import (
	"fmt"
	"strings"
)

// Validate rejects missing, forged, or unrelated evidence. Every returned
// Fact must cite a canonical request anchor and remain extractive from it.
func Validate(request AgentRequest, result Profile) error {
	documents := make(map[string]SourceDocument, len(request.Documents))
	for _, document := range request.Documents {
		if strings.TrimSpace(document.ID) == "" {
			return errorsAt("request.documents", "empty document ID")
		}
		if document.Kind != SourceJD && document.Kind != SourceResume {
			return errorsAt("request.documents", fmt.Sprintf("unsupported source kind %q", document.Kind))
		}
		if _, exists := documents[document.ID]; exists {
			return errorsAt("request.documents", fmt.Sprintf("duplicate document ID %q", document.ID))
		}
		documents[document.ID] = document
	}
	anchors := make(map[string]SourceAnchor, len(request.Anchors))
	for _, anchor := range request.Anchors {
		if strings.TrimSpace(anchor.ID) == "" {
			return errorsAt("request.anchors", "empty anchor ID")
		}
		if _, exists := anchors[anchor.ID]; exists {
			return errorsAt("request.anchors", fmt.Sprintf("duplicate anchor ID %q", anchor.ID))
		}
		document, exists := documents[anchor.SourceID]
		if !exists || document.Kind != anchor.Kind {
			return errorsAt("request.anchors", fmt.Sprintf("anchor %q does not belong to a canonical document", anchor.ID))
		}
		if strings.TrimSpace(anchor.Locator) == "" || strings.TrimSpace(anchor.Text) == "" {
			return errorsAt("request.anchors", fmt.Sprintf("anchor %q is missing locator or text", anchor.ID))
		}
		anchors[anchor.ID] = anchor
	}
	seenFactIDs := make(map[string]string)
	validate := func(path string, fact Fact, expectedKind SourceKind) error {
		return validateFact(path, fact, expectedKind, anchors, seenFactIDs)
	}

	if result.Job.Title != nil {
		if err := validate("job.title", *result.Job.Title, SourceJD); err != nil {
			return err
		}
	}
	for _, group := range []struct {
		path  string
		facts []Fact
	}{
		{"job.senioritySignals", result.Job.SenioritySignals},
		{"job.mustHave", result.Job.MustHave},
		{"job.niceToHave", result.Job.NiceToHave},
		{"job.responsibilities", result.Job.Responsibilities},
		{"job.technicalTopics", result.Job.TechnicalTopics},
		{"job.businessConstraints", result.Job.BusinessConstraints},
	} {
		for index, fact := range group.facts {
			if err := validate(fmt.Sprintf("%s[%d]", group.path, index), fact, SourceJD); err != nil {
				return err
			}
		}
	}

	if result.Candidate.Headline != nil {
		if err := validate("candidate.headline", *result.Candidate.Headline, SourceResume); err != nil {
			return err
		}
	}
	for _, group := range []struct {
		path  string
		facts []Fact
	}{
		{"candidate.skills", result.Candidate.Skills},
		{"candidate.responsibilities", result.Candidate.Responsibilities},
		{"candidate.metrics", result.Candidate.Metrics},
	} {
		for index, fact := range group.facts {
			if err := validate(fmt.Sprintf("%s[%d]", group.path, index), fact, SourceResume); err != nil {
				return err
			}
		}
	}

	seenProjects := make(map[string]struct{}, len(result.Candidate.Projects))
	for projectIndex, project := range result.Candidate.Projects {
		path := fmt.Sprintf("candidate.projects[%d]", projectIndex)
		if strings.TrimSpace(project.ID) == "" {
			return errorsAt(path, "project ID is required")
		}
		if _, exists := seenProjects[project.ID]; exists {
			return errorsAt(path, fmt.Sprintf("duplicate project ID %q", project.ID))
		}
		seenProjects[project.ID] = struct{}{}
		if err := validate(path+".name", project.Name, SourceResume); err != nil {
			return err
		}
		for _, group := range []struct {
			name  string
			facts []Fact
		}{
			{"responsibilities", project.Responsibilities},
			{"metrics", project.Metrics},
			{"technologies", project.Technologies},
			{"highlights", project.Highlights},
		} {
			for index, fact := range group.facts {
				if err := validate(fmt.Sprintf("%s.%s[%d]", path, group.name, index), fact, SourceResume); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateFact(path string, fact Fact, expectedKind SourceKind, anchors map[string]SourceAnchor, seenIDs map[string]string) error {
	id := strings.TrimSpace(fact.ID)
	value := strings.TrimSpace(fact.Value)
	if id == "" {
		return errorsAt(path, "fact ID is required")
	}
	if previous, exists := seenIDs[id]; exists {
		return errorsAt(path, fmt.Sprintf("fact ID %q already used at %s", id, previous))
	}
	seenIDs[id] = path
	if value == "" {
		return errorsAt(path, "fact value is required")
	}
	if len(fact.EvidenceRefs) == 0 {
		return errorsAt(path, "at least one evidence reference is required")
	}
	supported := false
	seenRefs := make(map[string]struct{}, len(fact.EvidenceRefs))
	for _, ref := range fact.EvidenceRefs {
		if _, duplicate := seenRefs[ref.AnchorID]; duplicate {
			return errorsAt(path, fmt.Sprintf("duplicate evidence anchor %q", ref.AnchorID))
		}
		seenRefs[ref.AnchorID] = struct{}{}
		anchor, exists := anchors[ref.AnchorID]
		if !exists {
			return errorsAt(path, fmt.Sprintf("unknown evidence anchor %q", ref.AnchorID))
		}
		if ref.SourceID != anchor.SourceID || ref.Kind != anchor.Kind || ref.Locator != anchor.Locator || ref.Quote != anchor.Text {
			return errorsAt(path, fmt.Sprintf("evidence metadata does not match anchor %q", ref.AnchorID))
		}
		if anchor.Kind != expectedKind {
			return errorsAt(path, fmt.Sprintf("evidence anchor %q has kind %q, want %q", ref.AnchorID, anchor.Kind, expectedKind))
		}
		if strings.Contains(normalizeGrounding(anchor.Text), normalizeGrounding(value)) {
			supported = true
		}
	}
	if !supported {
		return errorsAt(path, fmt.Sprintf("value %q is not extractive from its evidence", value))
	}
	return nil
}

func normalizeGrounding(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func errorsAt(path, reason string) error {
	return fmt.Errorf("%w at %s: %s", ErrUngroundedFact, path, reason)
}
