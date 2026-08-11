package profile

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNoMaterials    = errors.New("profile: JD or resume text is required")
	ErrUngroundedFact = errors.New("profile: ungrounded fact")

	yearsPattern  = regexp.MustCompile(`(?i)(?:\d+\s*(?:年|years?))|(?:\bsenior\b|\bstaff\b|\bprincipal\b|\blead\b|高级|资深|专家|负责人)`)
	metricPattern = regexp.MustCompile(`(?i)(?:\d+(?:\.\d+)?\s*(?:%|％|倍|万|亿|ms|s|秒|分钟|小时|qps|tps|rps|k|m|gb|tb|人|用户|请求|节点|台))|(?:p(?:50|90|95|99)|qps|tps|rps)\s*[:=]?\s*\d+`)
)

var technologyNames = []string{
	"Go", "Golang", "Java", "Python", "JavaScript", "TypeScript", "Node.js", "React", "Vue", "Next.js",
	"MySQL", "PostgreSQL", "SQLite", "Redis", "Kafka", "Pulsar", "RabbitMQ", "Elasticsearch", "ClickHouse",
	"Docker", "Kubernetes", "AWS", "Azure", "GCP", "gRPC", "GraphQL", "REST", "OpenTelemetry",
	"LLM", "RAG", "Agent", "LangChain", "LangGraph", "PyTorch", "TensorFlow",
}

type DeterministicExtractor struct{}

func NewDeterministicExtractor() *DeterministicExtractor {
	return &DeterministicExtractor{}
}

func (e *DeterministicExtractor) Extract(ctx context.Context, input Input) (Profile, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Profile{}, err
	}
	request, err := BuildAgentRequest(input)
	if err != nil {
		return Profile{}, err
	}
	result := Profile{
		Job: JobProfile{
			SenioritySignals:    make([]Fact, 0),
			MustHave:            make([]Fact, 0),
			NiceToHave:          make([]Fact, 0),
			Responsibilities:    make([]Fact, 0),
			TechnicalTopics:     make([]Fact, 0),
			BusinessConstraints: make([]Fact, 0),
		},
		Candidate: CandidateProfile{
			Skills:           make([]Fact, 0),
			Projects:         make([]ProjectProfile, 0),
			Responsibilities: make([]Fact, 0),
			Metrics:          make([]Fact, 0),
		},
	}

	result.Job = extractJob(anchorsOfKind(request.Anchors, SourceJD), result.Job)
	result.Candidate = extractCandidate(anchorsOfKind(request.Anchors, SourceResume), result.Candidate)
	if err := Validate(request, result); err != nil {
		return Profile{}, err
	}
	return result, nil
}

type AgentExtractor struct {
	agent AgentPort
}

func NewAgentExtractor(agent AgentPort) (*AgentExtractor, error) {
	if agent == nil {
		return nil, errors.New("profile: agent port is required")
	}
	return &AgentExtractor{agent: agent}, nil
}

func (e *AgentExtractor) Extract(ctx context.Context, input Input) (Profile, error) {
	if e == nil || e.agent == nil {
		return Profile{}, errors.New("profile: agent extractor is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Profile{}, err
	}
	request, err := BuildAgentRequest(input)
	if err != nil {
		return Profile{}, err
	}
	result, err := e.agent.ProposeProfile(ctx, request)
	if err != nil {
		return Profile{}, fmt.Errorf("profile: agent proposal: %w", err)
	}
	if err := Validate(request, result); err != nil {
		return Profile{}, err
	}
	return result, nil
}

func BuildAgentRequest(input Input) (AgentRequest, error) {
	request := AgentRequest{
		Documents: make([]SourceDocument, 0, 2),
		Anchors:   make([]SourceAnchor, 0),
	}
	seenSources := make(map[string]struct{}, 2)
	for _, material := range []struct {
		kind       SourceKind
		fallbackID string
		input      DocumentInput
	}{
		{kind: SourceJD, fallbackID: "jd", input: input.JD},
		{kind: SourceResume, fallbackID: "resume", input: input.Resume},
	} {
		text := strings.TrimSpace(material.input.Text)
		if text == "" {
			continue
		}
		sourceID := strings.TrimSpace(material.input.SourceID)
		if sourceID == "" {
			sourceID = material.fallbackID
		}
		if _, exists := seenSources[sourceID]; exists {
			return AgentRequest{}, fmt.Errorf("profile: duplicate source ID %q", sourceID)
		}
		seenSources[sourceID] = struct{}{}
		request.Documents = append(request.Documents, SourceDocument{
			ID: sourceID, Kind: material.kind, Name: strings.TrimSpace(material.input.Name),
		})
		segments := splitMaterial(material.input.Text)
		for index, segment := range segments {
			anchorID := fmt.Sprintf("%s:%03d", sourceID, index+1)
			request.Anchors = append(request.Anchors, SourceAnchor{
				ID: anchorID, SourceID: sourceID, Kind: material.kind,
				Locator: segment.locator, Text: segment.text,
			})
		}
	}
	if len(request.Documents) == 0 {
		return AgentRequest{}, ErrNoMaterials
	}
	return request, nil
}

type materialSegment struct {
	text    string
	locator string
}

func splitMaterial(value string) []materialSegment {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	segments := make([]materialSegment, 0, len(lines))
	for lineIndex, line := range lines {
		parts := splitSentenceParts(line)
		for partIndex, part := range parts {
			part = strings.TrimSpace(part)
			if utf8.RuneCountInString(cleanValue(part)) < 2 {
				continue
			}
			locator := fmt.Sprintf("line:%d", lineIndex+1)
			if len(parts) > 1 {
				locator = fmt.Sprintf("line:%d,part:%d", lineIndex+1, partIndex+1)
			}
			segments = append(segments, materialSegment{text: part, locator: locator})
		}
	}
	return segments
}

func splitSentenceParts(line string) []string {
	parts := make([]string, 0, 2)
	start := 0
	for index, r := range line {
		if !strings.ContainsRune("。！？!?；;", r) {
			continue
		}
		end := index + utf8.RuneLen(r)
		parts = append(parts, line[start:end])
		start = end
	}
	if start < len(line) {
		parts = append(parts, line[start:])
	}
	if len(parts) == 0 {
		return []string{line}
	}
	return parts
}

type section int

const (
	sectionUnknown section = iota
	sectionRequirements
	sectionNiceToHave
	sectionResponsibilities
	sectionProjects
	sectionSkills
	sectionExperience
)

func extractJob(anchors []SourceAnchor, result JobProfile) JobProfile {
	currentSection := sectionUnknown
	seen := map[string]map[string]struct{}{
		"seniority": {}, "must": {}, "nice": {}, "responsibility": {}, "topic": {}, "constraint": {},
	}
	for _, anchor := range anchors {
		value := cleanValue(anchor.Text)
		if detected, ok := headingSection(value); ok {
			currentSection = detected
			continue
		}
		if detected, body, ok := inlineSection(value); ok {
			currentSection = detected
			value = body
		}
		if value == "" {
			continue
		}
		if result.Title == nil {
			if title, ok := prefixedValue(value, "职位", "岗位", "职位名称", "position", "role"); ok {
				fact := factFromAnchor("job-title", title, anchor)
				result.Title = &fact
				continue
			}
			if currentSection == sectionUnknown && utf8.RuneCountInString(value) <= 80 && !looksLikeBullet(anchor.Text) {
				fact := factFromAnchor("job-title", value, anchor)
				result.Title = &fact
				continue
			}
		}

		lower := strings.ToLower(value)
		switch {
		case currentSection == sectionNiceToHave:
			result.NiceToHave = appendFact(result.NiceToHave, "job-nice", value, anchor, seen["nice"])
		case currentSection == sectionResponsibilities:
			result.Responsibilities = appendFact(result.Responsibilities, "job-responsibility", value, anchor, seen["responsibility"])
		case currentSection == sectionRequirements:
			result.MustHave = appendFact(result.MustHave, "job-must", value, anchor, seen["must"])
		case containsAny(lower, "加分", "优先", "preferred", "nice to have", "bonus", "a plus"):
			result.NiceToHave = appendFact(result.NiceToHave, "job-nice", value, anchor, seen["nice"])
		case containsAny(lower, "必须", "要求", "任职", "具备", "熟悉", "精通", "掌握", "至少", "年以上", "must", "required", "requirement", "proficient", "experience with", "years of"):
			result.MustHave = appendFact(result.MustHave, "job-must", value, anchor, seen["must"])
		case containsAny(lower, "负责", "职责", "主导", "设计", "建设", "维护", "交付", "develop", "build", "design", "maintain", "deliver", "own"):
			result.Responsibilities = appendFact(result.Responsibilities, "job-responsibility", value, anchor, seen["responsibility"])
		}

		if yearsPattern.MatchString(value) {
			result.SenioritySignals = appendFact(result.SenioritySignals, "job-seniority", value, anchor, seen["seniority"])
		}
		if containsAny(lower, "高并发", "可用性", "低延迟", "安全", "合规", "成本", "scalab", "availability", "latency", "security", "compliance", "cost") {
			result.BusinessConstraints = appendFact(result.BusinessConstraints, "job-constraint", value, anchor, seen["constraint"])
		}
		for _, technology := range technologiesIn(value) {
			result.TechnicalTopics = appendFact(result.TechnicalTopics, "job-topic", technology, anchor, seen["topic"])
		}
	}
	return result
}

func extractCandidate(anchors []SourceAnchor, result CandidateProfile) CandidateProfile {
	currentSection := sectionUnknown
	currentProject := -1
	seenSkills := make(map[string]struct{})
	seenResponsibilities := make(map[string]struct{})
	seenMetrics := make(map[string]struct{})
	for _, anchor := range anchors {
		value := cleanValue(anchor.Text)
		if detected, ok := headingSection(value); ok {
			currentSection = detected
			if detected != sectionProjects {
				currentProject = -1
			}
			continue
		}
		if detected, body, ok := inlineSection(value); ok {
			currentSection = detected
			if detected != sectionProjects {
				currentProject = -1
			}
			value = body
		}
		if value == "" {
			continue
		}

		if name, ok := projectName(value, anchor.Text, currentSection); ok {
			projectID := fmt.Sprintf("candidate-project-%03d", len(result.Projects)+1)
			result.Projects = append(result.Projects, ProjectProfile{
				ID:               projectID,
				Name:             factFromAnchor(projectID+"-name", name, anchor),
				Responsibilities: make([]Fact, 0), Metrics: make([]Fact, 0),
				Technologies: make([]Fact, 0), Highlights: make([]Fact, 0),
			})
			currentProject = len(result.Projects) - 1
			continue
		}

		if result.Headline == nil && currentSection == sectionUnknown && !looksLikeBullet(anchor.Text) {
			fact := factFromAnchor("candidate-headline", value, anchor)
			result.Headline = &fact
			continue
		}

		lower := strings.ToLower(value)
		isResponsibility := containsAny(lower, "负责", "主导", "独立", "设计", "实现", "搭建", "优化", "交付", "led", "owned", "designed", "implemented", "built", "delivered", "maintained")
		isMetric := metricPattern.MatchString(value) && containsAny(lower, "提升", "降低", "减少", "增长", "缩短", "节省", "达到", "支持", "覆盖", "从", "至", "to ", "increased", "reduced", "decreased", "improved", "saved", "served", "handled")

		if isResponsibility {
			result.Responsibilities = appendFact(result.Responsibilities, "candidate-responsibility", value, anchor, seenResponsibilities)
			if currentProject >= 0 {
				project := &result.Projects[currentProject]
				project.Responsibilities = appendFact(project.Responsibilities, project.ID+"-responsibility", value, anchor, nil)
			}
		}
		if isMetric {
			result.Metrics = appendFact(result.Metrics, "candidate-metric", value, anchor, seenMetrics)
			if currentProject >= 0 {
				project := &result.Projects[currentProject]
				project.Metrics = appendFact(project.Metrics, project.ID+"-metric", value, anchor, nil)
			}
		}

		technologies := technologiesIn(value)
		if currentSection == sectionSkills {
			technologies = append(technologies, skillListValues(value)...)
		}
		for _, technology := range uniqueStrings(technologies) {
			result.Skills = appendFact(result.Skills, "candidate-skill", technology, anchor, seenSkills)
			if currentProject >= 0 {
				project := &result.Projects[currentProject]
				project.Technologies = appendFact(project.Technologies, project.ID+"-technology", technology, anchor, nil)
			}
		}
		if currentProject >= 0 && !isResponsibility && !isMetric && currentSection == sectionProjects {
			project := &result.Projects[currentProject]
			project.Highlights = appendFact(project.Highlights, project.ID+"-highlight", value, anchor, nil)
		}
	}
	return result
}

func anchorsOfKind(anchors []SourceAnchor, kind SourceKind) []SourceAnchor {
	result := make([]SourceAnchor, 0)
	for _, anchor := range anchors {
		if anchor.Kind == kind {
			result = append(result, anchor)
		}
	}
	return result
}

func factFromAnchor(id, value string, anchor SourceAnchor) Fact {
	return Fact{ID: id, Value: strings.TrimSpace(value), EvidenceRefs: []EvidenceRef{{
		SourceID: anchor.SourceID, Kind: anchor.Kind, AnchorID: anchor.ID,
		Locator: anchor.Locator, Quote: anchor.Text,
	}}}
}

func appendFact(facts []Fact, prefix, value string, anchor SourceAnchor, seen map[string]struct{}) []Fact {
	value = strings.TrimSpace(value)
	key := strings.ToLower(value)
	if value == "" {
		return facts
	}
	if seen != nil {
		if _, exists := seen[key]; exists {
			return facts
		}
		seen[key] = struct{}{}
	}
	id := fmt.Sprintf("%s-%03d", prefix, len(facts)+1)
	return append(facts, factFromAnchor(id, value, anchor))
}

func headingSection(value string) (section, bool) {
	heading := strings.ToLower(strings.TrimSpace(value))
	heading = strings.Trim(heading, "#*：: -_[]【】")
	switch heading {
	case "任职要求", "岗位要求", "职位要求", "任职资格", "requirements", "qualifications", "must have", "must-have":
		return sectionRequirements, true
	case "加分项", "优先条件", "优先项", "nice to have", "nice-to-have", "preferred", "bonus":
		return sectionNiceToHave, true
	case "岗位职责", "工作职责", "职位职责", "职责", "responsibilities", "what you will do":
		return sectionResponsibilities, true
	case "项目经历", "项目经验", "项目", "projects", "project experience":
		return sectionProjects, true
	case "技能", "专业技能", "技术栈", "skills", "technologies", "tech stack":
		return sectionSkills, true
	case "工作经历", "工作经验", "experience", "employment", "work experience":
		return sectionExperience, true
	default:
		return sectionUnknown, false
	}
}

func inlineSection(value string) (section, string, bool) {
	groups := []struct {
		section  section
		prefixes []string
	}{
		{sectionRequirements, []string{"任职要求", "岗位要求", "职位要求", "任职资格", "requirements", "qualifications", "must have", "must-have"}},
		{sectionNiceToHave, []string{"加分项", "优先条件", "优先项", "nice to have", "nice-to-have", "preferred", "bonus"}},
		{sectionResponsibilities, []string{"岗位职责", "工作职责", "职位职责", "职责", "responsibilities", "what you will do"}},
		{sectionProjects, []string{"项目经历", "项目经验", "projects", "project experience"}},
		{sectionSkills, []string{"技能", "专业技能", "技术栈", "skills", "technologies", "tech stack"}},
		{sectionExperience, []string{"工作经历", "工作经验", "experience", "employment", "work experience"}},
	}
	for _, group := range groups {
		if body, ok := prefixedValue(value, group.prefixes...); ok {
			return group.section, body, true
		}
	}
	return sectionUnknown, value, false
}

func projectName(value, raw string, currentSection section) (string, bool) {
	if name, ok := prefixedValue(value, "项目名称", "项目", "project"); ok {
		return trimProjectDate(name), name != ""
	}
	if currentSection != sectionProjects || looksLikeBullet(raw) {
		return "", false
	}
	if utf8.RuneCountInString(value) > 100 || containsAny(strings.ToLower(value), "负责", "主导", "实现", "优化", "built", "designed", "implemented") {
		return "", false
	}
	if strings.HasPrefix(strings.TrimSpace(raw), "#") || strings.Contains(value, "|") || strings.Contains(value, "｜") {
		name := trimProjectDate(value)
		return name, name != ""
	}
	return value, true
}

func trimProjectDate(value string) string {
	for _, separator := range []string{"|", "｜", "\t"} {
		if index := strings.Index(value, separator); index > 0 {
			value = value[:index]
		}
	}
	return strings.TrimSpace(value)
}

func prefixedValue(value string, prefixes ...string) (string, bool) {
	lower := strings.ToLower(value)
	for _, prefix := range prefixes {
		prefix = strings.ToLower(prefix)
		for _, delimiter := range []string{"：", ":"} {
			marker := prefix + delimiter
			if strings.HasPrefix(lower, marker) {
				result := strings.TrimSpace(value[len(marker):])
				return result, result != ""
			}
		}
	}
	return "", false
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	for strings.HasPrefix(value, "#") {
		value = strings.TrimSpace(strings.TrimPrefix(value, "#"))
	}
	for _, prefix := range []string{"- ", "* ", "+ ", "• ", "· ", "> "} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(value, prefix))
		}
	}
	for index, r := range value {
		if !unicode.IsDigit(r) {
			if index > 0 && (r == '.' || r == '、' || r == ')' || r == '）') {
				return strings.TrimSpace(value[index+utf8.RuneLen(r):])
			}
			break
		}
	}
	return value
}

func looksLikeBullet(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.Contains("-*+•·>", string([]rune(value)[0])) {
		return true
	}
	for _, r := range value {
		if unicode.IsDigit(r) {
			continue
		}
		return r == '.' || r == '、' || r == ')' || r == '）'
	}
	return false
}

func technologiesIn(value string) []string {
	result := make([]string, 0)
	for _, technology := range technologyNames {
		if containsTechnology(value, technology) {
			result = append(result, technology)
		}
	}
	return uniqueStrings(result)
}

func containsTechnology(value, technology string) bool {
	value = strings.ToLower(value)
	technology = strings.ToLower(technology)
	searchFrom := 0
	for searchFrom < len(value) {
		position := strings.Index(value[searchFrom:], technology)
		if position < 0 {
			return false
		}
		start := searchFrom + position
		end := start + len(technology)
		beforeWord := false
		if start > 0 {
			before, _ := utf8.DecodeLastRuneInString(value[:start])
			beforeWord = isASCIITokenRune(before)
		}
		afterWord := false
		if end < len(value) {
			after, _ := utf8.DecodeRuneInString(value[end:])
			afterWord = isASCIITokenRune(after)
		}
		if !beforeWord && !afterWord {
			return true
		}
		searchFrom = start + 1
	}
	return false
}

func isASCIITokenRune(value rune) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '_'
}

func skillListValues(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return strings.ContainsRune(",，、/|｜;；", r)
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if name, ok := prefixedValue(part, "技能", "技术栈", "skills", "technologies"); ok {
			part = name
		}
		count := utf8.RuneCountInString(part)
		if count >= 2 && count <= 40 {
			result = append(result, part)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]string)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if existing, ok := seen[key]; !ok || len(value) < len(existing) {
			seen[key] = value
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, seen[key])
	}
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

var _ ProfileExtractor = (*DeterministicExtractor)(nil)
var _ ProfileExtractor = (*AgentExtractor)(nil)
