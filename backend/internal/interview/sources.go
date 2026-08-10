package interview

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var segmentBreak = regexp.MustCompile(`[\n。！？!?；;]+`)

var knowledgeQuestionPrefixes = []string{"问题：", "问题:", "question:", "question："}

var knowledgeReferenceMarkers = []string{
	"参考内容：", "参考内容:",
	"参考答案：", "参考答案:",
	"reference content:", "reference content：",
	"reference answer:", "reference answer：",
}

var knowledgeSourceMarkers = []string{"\n来源：", "\n来源:", "\nsource:", "\nsource："}

const minPrivateReferenceFragmentRunes = 16

// PublicEvidenceQuote returns the candidate-safe projection of an evidence
// quote. Knowledge anchors remain complete internally for assessment, while
// question generation and HTTP responses receive only the public question.
func PublicEvidenceQuote(ref EvidenceRef) string {
	if ref.Kind != SourceKnowledge {
		return ref.Quote
	}
	return publicKnowledgeQuestion(ref.Quote)
}

// PublicGeneratedText rejects model-authored text that reproduces a private
// knowledge reference. An empty result means callers must omit or replace it
// with a deterministic public summary.
func PublicGeneratedText(value string, refs []EvidenceRef) string {
	value = strings.TrimSpace(value)
	if value == "" || firstFoldedMarker(value, knowledgeReferenceMarkers) >= 0 {
		return ""
	}
	normalizedValue := normalizeQuestionGuardText(value)
	for _, ref := range refs {
		if ref.Kind != SourceKnowledge {
			continue
		}
		reference := knowledgeReferenceText(ref.Quote)
		if reference == "" {
			continue
		}
		normalizedReference := []rune(normalizeQuestionGuardText(reference))
		for start := 0; start+minPrivateReferenceFragmentRunes <= len(normalizedReference); start++ {
			fragment := string(normalizedReference[start : start+minPrivateReferenceFragmentRunes])
			if strings.Contains(normalizedValue, fragment) {
				return ""
			}
		}
	}
	return value
}

func publicKnowledgeQuestion(value string) string {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if question, ok := trimFoldedPrefix(line, knowledgeQuestionPrefixes); ok && question != "" {
			if position := firstFoldedMarker(question, knowledgeReferenceMarkers); position >= 0 {
				question = strings.TrimSpace(question[:position])
			}
			if question == "" {
				continue
			}
			return "问题：" + question
		}
	}
	if position := firstFoldedMarker(normalized, knowledgeReferenceMarkers); position >= 0 {
		if public := strings.TrimSpace(normalized[:position]); public != "" {
			return public
		}
	}
	return "知识题"
}

func knowledgeQuestionLabel(value string) string {
	public := publicKnowledgeQuestion(value)
	if question, ok := trimFoldedPrefix(strings.TrimSpace(public), knowledgeQuestionPrefixes); ok && question != "" {
		return question
	}
	return public
}

func knowledgeReferenceText(value string) string {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	position := firstFoldedMarker(normalized, knowledgeReferenceMarkers)
	if position < 0 {
		return ""
	}
	reference := normalized[position:]
	if _, body, ok := splitFoldedPrefix(reference, knowledgeReferenceMarkers); ok {
		reference = body
	}
	if end := firstFoldedMarker(reference, knowledgeSourceMarkers); end >= 0 {
		reference = reference[:end]
	}
	return strings.TrimSpace(reference)
}

func trimFoldedPrefix(value string, prefixes []string) (string, bool) {
	_, remainder, ok := splitFoldedPrefix(value, prefixes)
	return strings.TrimSpace(remainder), ok
}

func splitFoldedPrefix(value string, prefixes []string) (string, string, bool) {
	lower := strings.ToLower(value)
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, strings.ToLower(prefix)) {
			return value[:len(prefix)], value[len(prefix):], true
		}
	}
	return "", value, false
}

func firstFoldedMarker(value string, markers []string) int {
	lower := strings.ToLower(value)
	position := -1
	for _, marker := range markers {
		if candidate := strings.Index(lower, strings.ToLower(marker)); candidate >= 0 && (position < 0 || candidate < position) {
			position = candidate
		}
	}
	return position
}

func buildProfile(materials MaterialsInput, knowledge []KnowledgeDocument, focus Focus) (Profile, SourceIndex) {
	index := SourceIndex{
		Documents: make(map[string]SourceDocument),
		Anchors:   make(map[string]SourceAnchor),
		Order:     make([]string, 0),
	}

	if materials.JD != nil && strings.TrimSpace(materials.JD.Text) != "" {
		addDocument(&index, "jd", SourceJD, materials.JD.Name, materials.JD.Text)
	}
	if materials.Resume != nil && strings.TrimSpace(materials.Resume.Text) != "" {
		addDocument(&index, "resume", SourceResume, materials.Resume.Name, materials.Resume.Text)
	}
	for i, document := range knowledge {
		if strings.TrimSpace(document.Content) == "" {
			continue
		}
		name := strings.TrimSpace(document.Title)
		if name == "" {
			name = document.ID
		}
		sourceID := "knowledge:" + strings.TrimSpace(document.ID)
		if strings.TrimSpace(document.ID) == "" {
			sourceID = fmt.Sprintf("knowledge:%03d", i+1)
		}
		if _, exists := index.Documents[sourceID]; exists {
			sourceID = fmt.Sprintf("%s:%03d", sourceID, i+1)
		}
		addKnowledgeDocument(&index, sourceID, name, document.Content)
	}

	profile := Profile{
		JD: JDProfile{
			Requirements:     make([]ProfilePoint, 0),
			Responsibilities: make([]ProfilePoint, 0),
		},
		Resume: ResumeProfile{
			Skills:   make([]string, 0),
			Projects: make([]ProfilePoint, 0),
		},
		Coverage: make([]CoveragePoint, 0),
	}

	jdAnchors := anchorsByKind(index, SourceJD)
	resumeAnchors := anchorsByKind(index, SourceResume)
	knowledgeAnchors := anchorsByKind(index, SourceKnowledge)
	if len(jdAnchors) > 0 {
		profile.JD.Title = concise(jdAnchors[0].Text, 80)
	}
	for _, anchor := range jdAnchors {
		point := pointFromAnchor("jd-point-"+anchor.ID, anchor)
		lower := strings.ToLower(anchor.Text)
		if containsAny(lower, "负责", "职责", "建设", "设计", "develop", "build", "maintain", "deliver") {
			profile.JD.Responsibilities = appendLimitedPoint(profile.JD.Responsibilities, point, 8)
		} else {
			profile.JD.Requirements = appendLimitedPoint(profile.JD.Requirements, point, 8)
		}
	}
	if len(profile.JD.Requirements) == 0 && len(profile.JD.Responsibilities) > 0 {
		profile.JD.Requirements = append(profile.JD.Requirements, profile.JD.Responsibilities[0])
	}

	if len(resumeAnchors) > 0 {
		profile.Resume.Headline = concise(resumeAnchors[0].Text, 80)
	}
	profile.Resume.Skills = extractSkills(resumeAnchors)
	projectAnchors := make([]SourceAnchor, 0)
	for _, anchor := range resumeAnchors {
		lower := strings.ToLower(anchor.Text)
		if containsAny(lower, "项目", "负责", "设计", "实现", "优化", "上线", "project", "built", "designed", "implemented", "led", "owned", "improved") {
			projectAnchors = append(projectAnchors, anchor)
			profile.Resume.Projects = appendLimitedPoint(profile.Resume.Projects, pointFromAnchor("resume-point-"+anchor.ID, anchor), 8)
		}
	}
	if len(profile.Resume.Projects) == 0 && len(resumeAnchors) > 0 {
		projectAnchors = append(projectAnchors, resumeAnchors...)
		for _, anchor := range resumeAnchors {
			profile.Resume.Projects = appendLimitedPoint(profile.Resume.Projects, pointFromAnchor("resume-point-"+anchor.ID, anchor), 4)
		}
	}

	projectCoverage := projectCoverageFromAnchors(projectAnchors, resumeAnchors, 6)
	jdCoverage := coverageFromAnchors("jd", FocusKnowledge, prioritizeJDAnchors(jdAnchors), 6)
	jdCoverage = attachKnowledgeContext(jdCoverage, knowledgeAnchors, 2)
	knowledgeCoverage := jdCoverage
	knowledgeCoverage = append(knowledgeCoverage, coverageFromAnchors("knowledge", FocusKnowledge, knowledgeAnchors, 4)...)
	switch focus {
	case FocusProjects:
		profile.Coverage = append(profile.Coverage, projectCoverage...)
		profile.Coverage = append(profile.Coverage, knowledgeCoverage...)
	case FocusKnowledge:
		profile.Coverage = append(profile.Coverage, knowledgeCoverage...)
		profile.Coverage = append(profile.Coverage, projectCoverage...)
	default:
		profile.Coverage = interleaveCoverage(projectCoverage, knowledgeCoverage)
	}

	return profile, index
}

func addDocument(index *SourceIndex, id string, kind SourceKind, name, content string) {
	document := SourceDocument{ID: id, Kind: kind, Name: strings.TrimSpace(name), Content: content}
	index.Documents[id] = document
	segments := segmentMaterial(content)
	for i, text := range segments {
		anchorID := fmt.Sprintf("%s:%03d", id, i+1)
		anchor := SourceAnchor{
			ID:       anchorID,
			SourceID: id,
			Kind:     kind,
			Locator:  fmt.Sprintf("segment:%d", i+1),
			Text:     text,
		}
		index.Anchors[anchorID] = anchor
		index.Order = append(index.Order, anchorID)
	}
}

// Knowledge retrieval already returns one complete question-and-answer block.
// Keep that block atomic so the assessor always sees the reference content
// that belongs to the generated question.
func addKnowledgeDocument(index *SourceIndex, id, name, content string) {
	content = strings.TrimSpace(content)
	document := SourceDocument{ID: id, Kind: SourceKnowledge, Name: strings.TrimSpace(name), Content: content}
	index.Documents[id] = document
	anchorID := id + ":block"
	index.Anchors[anchorID] = SourceAnchor{
		ID:       anchorID,
		SourceID: id,
		Kind:     SourceKnowledge,
		Locator:  "question-block",
		Text:     content,
	}
	index.Order = append(index.Order, anchorID)
}

func segmentMaterial(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	parts := segmentBreak.Split(normalized, -1)
	segments := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		part = stripBullet(strings.TrimSpace(part))
		part = strings.Join(strings.Fields(part), " ")
		if utf8.RuneCountInString(part) < 3 {
			continue
		}
		key := strings.ToLower(part)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		segments = append(segments, part)
		if len(segments) == 40 {
			break
		}
	}
	return segments
}

func stripBullet(value string) string {
	return strings.TrimLeftFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("-*#>•·0123456789.、)）(", r)
	})
}

func anchorsByKind(index SourceIndex, kind SourceKind) []SourceAnchor {
	anchors := make([]SourceAnchor, 0)
	for _, id := range index.Order {
		anchor, exists := index.Anchors[id]
		if exists && anchor.Kind == kind {
			anchors = append(anchors, anchor)
		}
	}
	return anchors
}

func allAnchors(index SourceIndex) []SourceAnchor {
	anchors := make([]SourceAnchor, 0, len(index.Order))
	for _, id := range index.Order {
		if anchor, exists := index.Anchors[id]; exists {
			anchors = append(anchors, anchor)
		}
	}
	return anchors
}

func anchorsForEvidence(index SourceIndex, refs []EvidenceRef) []SourceAnchor {
	anchors := make([]SourceAnchor, 0, len(refs))
	seen := make(map[string]struct{})
	for _, ref := range refs {
		if _, exists := seen[ref.AnchorID]; exists {
			continue
		}
		if anchor, exists := index.Anchors[ref.AnchorID]; exists {
			seen[ref.AnchorID] = struct{}{}
			anchors = append(anchors, anchor)
		}
	}
	return anchors
}

func evidenceFromAnchor(anchor SourceAnchor) EvidenceRef {
	return EvidenceRef{
		SourceID: anchor.SourceID,
		Kind:     anchor.Kind,
		AnchorID: anchor.ID,
		Locator:  anchor.Locator,
		Quote:    anchor.Text,
	}
}

func allEvidence(index SourceIndex) []EvidenceRef {
	refs := make([]EvidenceRef, 0, len(index.Order))
	for _, anchor := range allAnchors(index) {
		refs = append(refs, evidenceFromAnchor(anchor))
	}
	return refs
}

func validateEvidenceRefs(index SourceIndex, refs []EvidenceRef, allowed map[string]struct{}, require bool) error {
	if require && len(refs) == 0 {
		return fmt.Errorf("at least one evidence reference is required")
	}
	for _, ref := range refs {
		anchor, exists := index.Anchors[ref.AnchorID]
		if !exists {
			return fmt.Errorf("unknown anchor %q", ref.AnchorID)
		}
		if allowed != nil {
			if _, exists := allowed[ref.AnchorID]; !exists {
				return fmt.Errorf("anchor %q is outside the allowed evidence set", ref.AnchorID)
			}
		}
		if ref.SourceID != anchor.SourceID || ref.Kind != anchor.Kind || ref.Locator != anchor.Locator {
			return fmt.Errorf("metadata does not match anchor %q", ref.AnchorID)
		}
		if normalizeEvidenceText(ref.Quote) != normalizeEvidenceText(anchor.Text) {
			return fmt.Errorf("quote does not match anchor %q", ref.AnchorID)
		}
	}
	return nil
}

func canonicalEvidence(index SourceIndex, refs []EvidenceRef) []EvidenceRef {
	result := make([]EvidenceRef, 0, len(refs))
	seen := make(map[string]struct{})
	for _, ref := range refs {
		if _, exists := seen[ref.AnchorID]; exists {
			continue
		}
		if anchor, exists := index.Anchors[ref.AnchorID]; exists {
			seen[ref.AnchorID] = struct{}{}
			result = append(result, evidenceFromAnchor(anchor))
		}
	}
	return result
}

func normalizeEvidenceText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func pointFromAnchor(id string, anchor SourceAnchor) ProfilePoint {
	return ProfilePoint{ID: id, Label: publicAnchorLabel(anchor), EvidenceRefs: []EvidenceRef{evidenceFromAnchor(anchor)}}
}

func coverageFromAnchors(prefix string, area Focus, anchors []SourceAnchor, limit int) []CoveragePoint {
	points := make([]CoveragePoint, 0, minInt(limit, len(anchors)))
	for i, anchor := range anchors {
		if i == limit {
			break
		}
		points = append(points, CoveragePoint{
			ID:           fmt.Sprintf("%s-%03d", prefix, i+1),
			Area:         area,
			Label:        publicAnchorLabel(anchor),
			EvidenceRefs: []EvidenceRef{evidenceFromAnchor(anchor)},
		})
	}
	return points
}

func publicAnchorLabel(anchor SourceAnchor) string {
	if anchor.Kind == SourceKnowledge {
		return concise(knowledgeQuestionLabel(anchor.Text), 120)
	}
	return concise(anchor.Text, 120)
}

func attachKnowledgeContext(points []CoveragePoint, anchors []SourceAnchor, limit int) []CoveragePoint {
	if limit <= 0 || len(anchors) == 0 {
		return points
	}
	if len(anchors) < limit {
		limit = len(anchors)
	}
	for index := range points {
		for _, anchor := range anchors[:limit] {
			points[index].EvidenceRefs = append(points[index].EvidenceRefs, evidenceFromAnchor(anchor))
		}
	}
	return points
}

func projectCoverageFromAnchors(projectAnchors, allResumeAnchors []SourceAnchor, limit int) []CoveragePoint {
	points := make([]CoveragePoint, 0, minInt(limit, len(projectAnchors)))
	positions := make(map[string]int, len(allResumeAnchors))
	for i, anchor := range allResumeAnchors {
		positions[anchor.ID] = i
	}
	for i, anchor := range projectAnchors {
		if i == limit {
			break
		}
		refs := []EvidenceRef{evidenceFromAnchor(anchor)}
		if position, exists := positions[anchor.ID]; exists && position+1 < len(allResumeAnchors) {
			refs = append(refs, evidenceFromAnchor(allResumeAnchors[position+1]))
		}
		points = append(points, CoveragePoint{
			ID:           fmt.Sprintf("project-%03d", i+1),
			Area:         FocusProjects,
			Label:        concise(anchor.Text, 120),
			EvidenceRefs: refs,
		})
	}
	return points
}

func prioritizeJDAnchors(anchors []SourceAnchor) []SourceAnchor {
	if len(anchors) < 2 {
		return anchors
	}
	prioritized := make([]SourceAnchor, 0, len(anchors))
	seen := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors[1:] {
		lower := strings.ToLower(anchor.Text)
		if containsAny(lower, "要求", "熟悉", "掌握", "经验", "能力", "must", "required", "proficient", "experience") {
			prioritized = append(prioritized, anchor)
			seen[anchor.ID] = struct{}{}
		}
	}
	for _, anchor := range anchors[1:] {
		if _, exists := seen[anchor.ID]; !exists {
			prioritized = append(prioritized, anchor)
			seen[anchor.ID] = struct{}{}
		}
	}
	prioritized = append(prioritized, anchors[0])
	return prioritized
}

func interleaveCoverage(first, second []CoveragePoint) []CoveragePoint {
	result := make([]CoveragePoint, 0, len(first)+len(second))
	for i := 0; i < len(first) || i < len(second); i++ {
		if i < len(first) {
			result = append(result, first[i])
		}
		if i < len(second) {
			result = append(result, second[i])
		}
	}
	return result
}

func appendLimitedPoint(points []ProfilePoint, point ProfilePoint, limit int) []ProfilePoint {
	if len(points) >= limit {
		return points
	}
	return append(points, point)
}

func extractSkills(anchors []SourceAnchor) []string {
	known := []string{
		"go", "golang", "java", "python", "javascript", "typescript", "react", "vue", "next.js",
		"mysql", "postgresql", "redis", "kafka", "docker", "kubernetes", "aws", "grpc", "rag", "llm", "agent",
	}
	found := make(map[string]struct{})
	for _, anchor := range anchors {
		lower := strings.ToLower(anchor.Text)
		for _, skill := range known {
			if strings.Contains(lower, skill) {
				found[skill] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(found))
	for skill := range found {
		result = append(result, skill)
	}
	sort.Strings(result)
	return result
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func concise(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
