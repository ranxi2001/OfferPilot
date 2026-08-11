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

func TestJDCoverageDoesNotInheritGlobalKnowledgeContext(t *testing.T) {
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
	if point.ID == "" || len(point.EvidenceRefs) != 1 || point.EvidenceRefs[0].Kind != SourceJD {
		t.Fatalf("JD coverage inherited unrelated global knowledge evidence: %+v", point)
	}
	if len(anchorsByKind(index, SourceKnowledge)) != 1 {
		t.Fatal("knowledge source should remain indexed for its own coverage point")
	}
}

func TestPublicGeneratedTextAllowsConceptsButRejectsReferenceCopy(t *testing.T) {
	t.Parallel()

	ref := EvidenceRef{
		Kind:  SourceKnowledge,
		Quote: "问题：ColBERT 如何工作？\n参考答案：ColBERT 会对每个 query token 与文档 token 做 MaxSim，再聚合局部匹配分数。",
	}
	if got := PublicGeneratedText("需要补充 MaxSim 的适用边界。", []EvidenceRef{ref}); got == "" {
		t.Fatal("short technical concept was treated as private reference leakage")
	}
	if got := PublicGeneratedText("每个 query token 与文档 token 做 MaxSim，再聚合局部匹配分数。", []EvidenceRef{ref}); got != "" {
		t.Fatalf("verbatim reference copy was not rejected: %q", got)
	}
}
