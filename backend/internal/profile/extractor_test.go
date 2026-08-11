package profile

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDeterministicExtractorBuildsGroundedChineseProfile(t *testing.T) {
	input := Input{
		JD: DocumentInput{SourceID: "jd-v1", Text: `职位：高级 Go 后端工程师
任职要求
- 5 年以上 Go 服务端经验，必须熟悉 MySQL 与 Redis
- 掌握 Kubernetes，具备高并发和低延迟系统设计经验
加分项
- 有 LLM Agent 或 RAG 落地经验优先
岗位职责
- 负责交易平台架构设计、核心服务交付与稳定性治理`},
		Resume: DocumentInput{SourceID: "resume-v3", Text: `张三｜后端工程师
项目经历
### OfferPilot | 2025.01-2026.06
- 主导 Go Agent Harness 与 SQLite 状态层设计，负责服务上线
- 将 P95 延迟从 800ms 降低至 220ms，支持 10k QPS
- 使用 Go、Redis、Docker 和 Kubernetes
技能
Go、TypeScript、PostgreSQL、OpenTelemetry`},
	}

	extractor := NewDeterministicExtractor()
	first, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("deterministic extractor returned different profiles")
	}
	if first.Job.Title == nil || first.Job.Title.Value != "高级 Go 后端工程师" {
		t.Fatalf("job title = %+v", first.Job.Title)
	}
	if len(first.Job.MustHave) != 2 || len(first.Job.NiceToHave) != 1 || len(first.Job.Responsibilities) != 1 {
		t.Fatalf("unexpected job extraction: %+v", first.Job)
	}
	if len(first.Job.SenioritySignals) == 0 || len(first.Job.BusinessConstraints) == 0 {
		t.Fatalf("seniority or constraints missing: %+v", first.Job)
	}
	if !hasFactValue(first.Job.TechnicalTopics, "Go") || !hasFactValue(first.Job.TechnicalTopics, "Kubernetes") {
		t.Fatalf("technical topics = %+v", first.Job.TechnicalTopics)
	}
	if len(first.Candidate.Projects) != 1 || first.Candidate.Projects[0].Name.Value != "OfferPilot" {
		t.Fatalf("projects = %+v", first.Candidate.Projects)
	}
	project := first.Candidate.Projects[0]
	if len(project.Responsibilities) != 1 || len(project.Metrics) != 1 {
		t.Fatalf("project facts = %+v", project)
	}
	if !hasFactValue(first.Candidate.Skills, "OpenTelemetry") || !hasFactValue(project.Technologies, "Kubernetes") {
		t.Fatalf("skills were not grounded: candidate=%+v project=%+v", first.Candidate.Skills, project.Technologies)
	}
	request, err := BuildAgentRequest(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(request, first); err != nil {
		t.Fatalf("profile is not grounded: %v", err)
	}
	assertAllEvidenceKinds(t, first, SourceJD, SourceResume)
}

func TestDeterministicExtractorUnderstandsEnglishSections(t *testing.T) {
	result, err := NewDeterministicExtractor().Extract(context.Background(), Input{
		JD: DocumentInput{Text: `Position: Staff Platform Engineer
Requirements
- 7 years of experience with Go and Kubernetes
Nice to have
- RAG production experience preferred
Responsibilities
- Build low latency platform services and maintain observability`},
		Resume: DocumentInput{Text: `Platform engineer
Projects
Control Plane | 2024-2026
- Led the Go service design and delivered the Kubernetes migration
- Reduced P99 latency to 95ms and handled 20k RPS
Skills
Go, Kubernetes, Prometheus`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Job.Title == nil || result.Job.Title.Value != "Staff Platform Engineer" {
		t.Fatalf("title = %+v", result.Job.Title)
	}
	if len(result.Job.MustHave) != 1 || len(result.Job.NiceToHave) != 1 || len(result.Job.Responsibilities) != 1 {
		t.Fatalf("job = %+v", result.Job)
	}
	if len(result.Candidate.Projects) != 1 || len(result.Candidate.Projects[0].Metrics) != 1 {
		t.Fatalf("candidate = %+v", result.Candidate)
	}
	if !hasFactValue(result.Candidate.Skills, "Prometheus") {
		t.Fatalf("custom skill list item was not extracted: %+v", result.Candidate.Skills)
	}
}

func TestDeterministicExtractorHandlesInlineSectionsWithoutSubstringSkills(t *testing.T) {
	result, err := NewDeterministicExtractor().Extract(context.Background(), Input{
		JD:     DocumentInput{Text: "Position: Backend Engineer\nRequirements: Experience with MongoDB and Go\nResponsibilities: Build reliable APIs"},
		Resume: DocumentInput{Text: "Platform engineer\nProjects: Atlas\n- Built MongoDB migration tooling\nSkills: Go, MongoDB, Prometheus"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Job.MustHave) != 1 || result.Job.MustHave[0].Value != "Experience with MongoDB and Go" {
		t.Fatalf("inline requirement = %+v", result.Job.MustHave)
	}
	if len(result.Job.Responsibilities) != 1 || len(result.Candidate.Projects) != 1 {
		t.Fatalf("inline sections were not classified: job=%+v candidate=%+v", result.Job, result.Candidate)
	}
	if !hasFactValue(result.Candidate.Skills, "Go") || !hasFactValue(result.Candidate.Skills, "MongoDB") || !hasFactValue(result.Candidate.Skills, "Prometheus") {
		t.Fatalf("inline skills = %+v", result.Candidate.Skills)
	}
	if got := technologiesIn("MongoDB migration"); hasStringFold(got, "Go") {
		t.Fatalf("Go was matched as a MongoDB substring: %v", got)
	}
	if got := technologiesIn("熟悉Go语言并用于服务开发"); !hasStringFold(got, "Go") {
		t.Fatalf("Go next to Chinese text was not matched: %v", got)
	}
}

func TestDeterministicExtractorSplitsInlineProjectNameAndResponsibility(t *testing.T) {
	result, err := NewDeterministicExtractor().Extract(context.Background(), Input{
		Resume: DocumentInput{Text: "项目 OfferPilot：我负责 Go Agent Harness 的架构设计与实现。"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidate.Projects) != 1 || result.Candidate.Projects[0].Name.Value != "OfferPilot" {
		t.Fatalf("projects = %+v", result.Candidate.Projects)
	}
	project := result.Candidate.Projects[0]
	if len(project.Responsibilities) != 1 || project.Responsibilities[0].Value != "我负责 Go Agent Harness 的架构设计与实现。" {
		t.Fatalf("inline project responsibilities = %+v", project.Responsibilities)
	}
	if err := Validate(mustBuildAgentRequest(t, Input{Resume: DocumentInput{Text: "项目 OfferPilot：我负责 Go Agent Harness 的架构设计与实现。"}}), result); err != nil {
		t.Fatal(err)
	}
}

func TestAgentExtractorRejectsUngroundedAndForgedFacts(t *testing.T) {
	input := Input{
		JD:     DocumentInput{Text: "职位：Go 工程师\n任职要求\n- 必须熟悉 Go"},
		Resume: DocumentInput{Text: "Go 工程师\n- 负责 Go 服务开发"},
	}
	tests := []struct {
		name    string
		propose func(AgentRequest) Profile
	}{
		{
			name: "missing evidence",
			propose: func(AgentRequest) Profile {
				return Profile{Job: JobProfile{MustHave: []Fact{{ID: "must-1", Value: "必须熟悉 Go"}}}}
			},
		},
		{
			name: "unknown anchor",
			propose: func(AgentRequest) Profile {
				return Profile{Job: JobProfile{MustHave: []Fact{{
					ID: "must-1", Value: "必须熟悉 Go", EvidenceRefs: []EvidenceRef{{
						SourceID: "jd", Kind: SourceJD, AnchorID: "jd:999", Locator: "line:3", Quote: "必须熟悉 Go",
					}},
				}}}}
			},
		},
		{
			name: "unsupported value",
			propose: func(request AgentRequest) Profile {
				anchor := request.Anchors[len(request.Anchors)-1]
				return Profile{Job: JobProfile{MustHave: []Fact{{
					ID: "must-1", Value: "必须精通 Rust", EvidenceRefs: []EvidenceRef{evidence(anchor)},
				}}}}
			},
		},
		{
			name: "job fact backed by resume",
			propose: func(request AgentRequest) Profile {
				anchor := request.Anchors[len(request.Anchors)-1]
				return Profile{Job: JobProfile{MustHave: []Fact{{
					ID: "must-1", Value: "Go 服务开发", EvidenceRefs: []EvidenceRef{evidence(anchor)},
				}}}}
			},
		},
		{
			name: "candidate fact backed by JD",
			propose: func(request AgentRequest) Profile {
				anchor := request.Anchors[2]
				return Profile{Candidate: CandidateProfile{Skills: []Fact{{
					ID: "skill-1", Value: "Go", EvidenceRefs: []EvidenceRef{evidence(anchor)},
				}}}}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extractor, err := NewAgentExtractor(agentStub{propose: test.propose})
			if err != nil {
				t.Fatal(err)
			}
			_, err = extractor.Extract(context.Background(), input)
			if !errors.Is(err, ErrUngroundedFact) {
				t.Fatalf("error = %v, want ErrUngroundedFact", err)
			}
		})
	}
}

func TestAgentExtractorAcceptsCanonicalExtractiveProposal(t *testing.T) {
	input := Input{Resume: DocumentInput{SourceID: "resume-42", Text: "项目经历\nProject: Atlas\n- Led Go migration"}}
	extractor, err := NewAgentExtractor(agentStub{propose: func(request AgentRequest) Profile {
		anchor := request.Anchors[1]
		name := Fact{ID: "project-1-name", Value: "Atlas", EvidenceRefs: []EvidenceRef{evidence(anchor)}}
		return Profile{Candidate: CandidateProfile{Projects: []ProjectProfile{{ID: "project-1", Name: name}}}}
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := extractor.Extract(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidate.Projects) != 1 || result.Candidate.Projects[0].Name.Value != "Atlas" {
		t.Fatalf("result = %+v", result)
	}
}

func TestExtractorRejectsEmptyInputAndHonorsCancellation(t *testing.T) {
	_, err := NewDeterministicExtractor().Extract(context.Background(), Input{})
	if !errors.Is(err, ErrNoMaterials) {
		t.Fatalf("empty error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewDeterministicExtractor().Extract(ctx, Input{JD: DocumentInput{Text: "职位：Go 工程师"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
}

type agentStub struct {
	propose func(AgentRequest) Profile
}

func (a agentStub) ProposeProfile(_ context.Context, request AgentRequest) (Profile, error) {
	return a.propose(request), nil
}

func evidence(anchor SourceAnchor) EvidenceRef {
	return EvidenceRef{SourceID: anchor.SourceID, Kind: anchor.Kind, AnchorID: anchor.ID, Locator: anchor.Locator, Quote: anchor.Text}
}

func mustBuildAgentRequest(t *testing.T, input Input) AgentRequest {
	t.Helper()
	request, err := BuildAgentRequest(input)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func hasFactValue(facts []Fact, value string) bool {
	for _, fact := range facts {
		if strings.EqualFold(fact.Value, value) {
			return true
		}
	}
	return false
}

func hasStringFold(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(candidate, value) {
			return true
		}
	}
	return false
}

func assertAllEvidenceKinds(t *testing.T, result Profile, allowed ...SourceKind) {
	t.Helper()
	allowedKinds := make(map[SourceKind]struct{}, len(allowed))
	for _, kind := range allowed {
		allowedKinds[kind] = struct{}{}
	}
	check := func(fact Fact) {
		if len(fact.EvidenceRefs) == 0 {
			t.Fatalf("fact %q has no evidence", fact.ID)
		}
		for _, ref := range fact.EvidenceRefs {
			if _, ok := allowedKinds[ref.Kind]; !ok {
				t.Fatalf("fact %q has unexpected evidence kind %q", fact.ID, ref.Kind)
			}
		}
	}
	if result.Job.Title != nil {
		check(*result.Job.Title)
	}
	for _, facts := range [][]Fact{
		result.Job.SenioritySignals, result.Job.MustHave, result.Job.NiceToHave,
		result.Job.Responsibilities, result.Job.TechnicalTopics, result.Job.BusinessConstraints,
		result.Candidate.Skills, result.Candidate.Responsibilities, result.Candidate.Metrics,
	} {
		for _, fact := range facts {
			check(fact)
		}
	}
	if result.Candidate.Headline != nil {
		check(*result.Candidate.Headline)
	}
	for _, project := range result.Candidate.Projects {
		check(project.Name)
		for _, facts := range [][]Fact{project.Responsibilities, project.Metrics, project.Technologies, project.Highlights} {
			for _, fact := range facts {
				check(fact)
			}
		}
	}
}
