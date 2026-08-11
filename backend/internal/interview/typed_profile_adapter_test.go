package interview

import (
	"context"
	"errors"
	"strings"
	"testing"

	typedprofile "offerpilot/backend/internal/profile"
)

func TestExtractTypedProfileBuildsCanonicalProfileSourcesAndCoverage(t *testing.T) {
	materials := MaterialsInput{
		JD: &MaterialInput{Name: "role.md", Text: `职位：高级 Go 后端工程师
任职要求
- 5 年以上 Go 经验，必须熟悉 Redis
岗位职责
- 负责高并发服务架构设计与交付`},
		Resume: &MaterialInput{Name: "resume.md", Text: `后端工程师
项目经历
OfferPilot | 2025-2026
- 主导 Go Agent Harness 设计与上线
- 将 P95 延迟从 800ms 降低至 220ms，支持 10k QPS
技能
Go、SQLite、OpenTelemetry`},
	}
	knowledge := []KnowledgeDocument{{
		ID: "go-scheduler", Title: "Go 调度器",
		Content: "问题：解释 Go 调度器\n参考内容：private scheduler reference",
	}}

	result, index, err := ExtractTypedProfile(
		context.Background(), typedprofile.NewDeterministicExtractor(), materials, knowledge, FocusMixed,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.JD.Title != "高级 Go 后端工程师" || len(result.JD.Requirements) != 1 || len(result.JD.Responsibilities) != 1 {
		t.Fatalf("JD profile = %+v", result.JD)
	}
	if result.Resume.Headline != "后端工程师" || len(result.Resume.Projects) != 1 {
		t.Fatalf("resume profile = %+v", result.Resume)
	}
	if _, ok := index.Documents["jd"]; !ok {
		t.Fatal("JD source document is missing")
	}
	if _, ok := index.Documents["resume"]; !ok {
		t.Fatal("resume source document is missing")
	}
	if _, ok := index.Documents["knowledge:go-scheduler"]; !ok {
		t.Fatal("knowledge source document is missing")
	}

	expectedLabels := []string{
		"5 年以上 Go 经验，必须熟悉 Redis",
		"负责高并发服务架构设计与交付",
		"OfferPilot",
		"将 P95 延迟从 800ms 降低至 220ms，支持 10k QPS",
	}
	for _, label := range expectedLabels {
		point, ok := coverageWithLabel(result.Coverage, label)
		if !ok {
			t.Fatalf("coverage for %q is missing: %+v", label, result.Coverage)
		}
		if err := validateEvidenceRefs(index, point.EvidenceRefs, nil, true); err != nil {
			t.Fatalf("coverage %q is not canonical: %v", label, err)
		}
	}
	project := result.Resume.Projects[0]
	metricPoint, _ := coverageWithLabel(result.Coverage, expectedLabels[3])
	if !sharesEvidenceAnchor(project.EvidenceRefs, metricPoint.EvidenceRefs) {
		t.Fatalf("project evidence does not retain metric anchor: project=%+v metric=%+v", project.EvidenceRefs, metricPoint.EvidenceRefs)
	}
	for _, point := range result.Coverage {
		if strings.Contains(point.Label, "private scheduler reference") {
			t.Fatalf("knowledge coverage label leaked reference content: %q", point.Label)
		}
	}
}

func TestExtractTypedProfileOrdersCoverageByFocusWithoutDroppingFacts(t *testing.T) {
	materials := MaterialsInput{
		JD:     &MaterialInput{Text: "职位：Go 工程师\n任职要求\n- 必须熟悉 Go"},
		Resume: &MaterialInput{Text: "工程师\n项目经历\nAtlas | 2025\n- 主导 Go 服务设计"},
	}
	for _, test := range []struct {
		focus     Focus
		firstArea Focus
	}{
		{focus: FocusKnowledge, firstArea: FocusKnowledge},
		{focus: FocusProjects, firstArea: FocusProjects},
	} {
		t.Run(string(test.focus), func(t *testing.T) {
			result, _, err := ExtractTypedProfile(context.Background(), typedprofile.NewDeterministicExtractor(), materials, nil, test.focus)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Coverage) < 2 || result.Coverage[0].Area != test.firstArea {
				t.Fatalf("coverage order = %+v", result.Coverage)
			}
			if _, ok := coverageWithLabel(result.Coverage, "必须熟悉 Go"); !ok {
				t.Fatal("must-have coverage was dropped")
			}
			if _, ok := coverageWithLabel(result.Coverage, "Atlas"); !ok {
				t.Fatal("project coverage was dropped")
			}
		})
	}
}

func TestExtractTypedProfileRevalidatesExtractorOutput(t *testing.T) {
	extractor := typedExtractorStub{extract: func(context.Context, typedprofile.Input) (typedprofile.Profile, error) {
		return typedprofile.Profile{Job: typedprofile.JobProfile{MustHave: []typedprofile.Fact{{
			ID: "fabricated", Value: "必须精通 Rust", EvidenceRefs: []typedprofile.EvidenceRef{{
				SourceID: "jd", Kind: typedprofile.SourceJD, AnchorID: "jd:999", Locator: "line:1", Quote: "必须精通 Rust",
			}},
		}}}}, nil
	}}
	_, _, err := ExtractTypedProfile(
		context.Background(), extractor,
		MaterialsInput{JD: &MaterialInput{Text: "任职要求\n- 必须熟悉 Go"}}, nil, FocusKnowledge,
	)
	if !errors.Is(err, typedprofile.ErrUngroundedFact) {
		t.Fatalf("error = %v, want typedprofile.ErrUngroundedFact", err)
	}
}

func TestExtractTypedProfileSupportsKnowledgeOnlySession(t *testing.T) {
	result, index, err := ExtractTypedProfile(
		context.Background(), typedprofile.NewDeterministicExtractor(), MaterialsInput{},
		[]KnowledgeDocument{{ID: "one", Title: "One", Content: "问题：公开问题\n参考内容：private"}},
		FocusKnowledge,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Coverage) != 1 || result.Coverage[0].Area != FocusKnowledge {
		t.Fatalf("coverage = %+v", result.Coverage)
	}
	if err := validateEvidenceRefs(index, result.Coverage[0].EvidenceRefs, nil, true); err != nil {
		t.Fatal(err)
	}
}

type typedExtractorStub struct {
	extract func(context.Context, typedprofile.Input) (typedprofile.Profile, error)
}

func (s typedExtractorStub) Extract(ctx context.Context, input typedprofile.Input) (typedprofile.Profile, error) {
	return s.extract(ctx, input)
}

func coverageWithLabel(points []CoveragePoint, label string) (CoveragePoint, bool) {
	for _, point := range points {
		if point.Label == label {
			return point, true
		}
	}
	return CoveragePoint{}, false
}

func sharesEvidenceAnchor(left, right []EvidenceRef) bool {
	anchors := make(map[string]struct{}, len(left))
	for _, ref := range left {
		anchors[ref.AnchorID] = struct{}{}
	}
	for _, ref := range right {
		if _, ok := anchors[ref.AnchorID]; ok {
			return true
		}
	}
	return false
}
