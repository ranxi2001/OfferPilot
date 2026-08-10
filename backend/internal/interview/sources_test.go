package interview

import (
	"strings"
	"testing"
)

func TestKnowledgeQuestionBlockRemainsAtomicAndStable(t *testing.T) {
	t.Parallel()

	document := KnowledgeDocument{
		ID:      "kb_colbert",
		Title:   "ColBERT late interaction",
		Content: "知识主题：RAG 重排\n问题：ColBERT 为什么能保留 token 级交互？\n参考内容：分别编码 query 和 document token，再用 MaxSim 聚合。\n来源：rag.md",
	}
	profile, index := buildProfile(MaterialsInput{}, []KnowledgeDocument{document}, FocusKnowledge)

	anchors := anchorsByKind(index, SourceKnowledge)
	if len(anchors) != 1 {
		t.Fatalf("knowledge anchors = %d, want one atomic question block", len(anchors))
	}
	anchor := anchors[0]
	if anchor.SourceID != "knowledge:kb_colbert" || anchor.ID != "knowledge:kb_colbert:block" {
		t.Fatalf("unstable knowledge identity: %+v", anchor)
	}
	if !strings.Contains(anchor.Text, "问题：ColBERT") || !strings.Contains(anchor.Text, "参考内容：分别编码") {
		t.Fatalf("question and reference content were split: %q", anchor.Text)
	}
	if len(profile.Coverage) == 0 || len(profile.Coverage[0].EvidenceRefs) != 1 || profile.Coverage[0].EvidenceRefs[0].AnchorID != anchor.ID {
		t.Fatalf("coverage does not reference the complete block: %+v", profile.Coverage)
	}
}

func TestJDKnowledgeCoverageCarriesPrivateReferenceContext(t *testing.T) {
	t.Parallel()

	document := KnowledgeDocument{
		ID:      "kb_colbert",
		Title:   "ColBERT late interaction",
		Content: "知识主题：RAG 重排\n问题：ColBERT 为什么能保留 token 级交互？\n参考内容：分别编码 query 和 document token，再用 MaxSim 聚合。",
	}
	profile, index := buildProfile(
		MaterialsInput{JD: &MaterialInput{Text: "高级 RAG 工程师\n负责 ColBERT 重排与离线评测"}},
		[]KnowledgeDocument{document},
		FocusKnowledge,
	)
	var point CoveragePoint
	for _, candidate := range profile.Coverage {
		if strings.HasPrefix(candidate.ID, "jd-") {
			point = candidate
			break
		}
	}
	if point.ID == "" || len(point.EvidenceRefs) < 2 || point.EvidenceRefs[0].Kind != SourceJD {
		t.Fatalf("JD coverage is missing primary/private knowledge evidence: %+v", point)
	}
	question := Question{CoveragePointID: point.ID, Kind: QuestionKnowledge, EvidenceRefs: point.EvidenceRefs[:1]}
	anchors := assessmentAnchors(InterviewSession{Profile: profile, Sources: index}, question)
	foundReference := false
	for _, anchor := range anchors {
		if anchor.Kind == SourceKnowledge && strings.Contains(anchor.Text, "参考内容：分别编码") {
			foundReference = true
			break
		}
	}
	if !foundReference {
		t.Fatalf("assessor did not receive the atomic knowledge reference: %+v", anchors)
	}
}
