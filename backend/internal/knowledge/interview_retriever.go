package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"offerpilot/backend/internal/interview"
)

const (
	defaultInterviewLimit = 8
	maxInterviewLimit     = 20
)

// InterviewRetriever adapts the question-granularity index to the interview
// domain port. The immutable Index makes this adapter safe for concurrent use.
type InterviewRetriever struct {
	index *Index
	limit int
}

func NewInterviewRetriever(index *Index, limit int) (*InterviewRetriever, error) {
	if index == nil {
		return nil, errors.New("knowledge: interview retriever needs an index")
	}
	if limit <= 0 {
		limit = defaultInterviewLimit
	} else if limit > maxInterviewLimit {
		limit = maxInterviewLimit
	}
	return &InterviewRetriever{index: index, limit: limit}, nil
}

func (r *InterviewRetriever) Retrieve(ctx context.Context, query interview.KnowledgeQuery) ([]interview.KnowledgeDocument, error) {
	if r == nil || r.index == nil {
		return nil, errors.New("knowledge: nil interview retriever")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	searchText := interviewSearchText(query)
	entries := r.index.Search(searchText, r.limit)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	documents := make([]interview.KnowledgeDocument, 0, len(entries))
	for _, entry := range entries {
		documents = append(documents, interview.KnowledgeDocument{
			ID:    entry.ID,
			Title: entry.Question,
			Content: fmt.Sprintf(
				"知识主题：%s\n问题：%s\n参考内容：%s\n来源：%s",
				entry.Title, entry.Question, entry.Excerpt, entry.Source,
			),
		})
	}
	return documents, nil
}

func interviewSearchText(query interview.KnowledgeQuery) string {
	parts := make([]string, 0, 5)
	if value := boundedMaterial(query.Objective, 1000); value != "" {
		parts = append(parts, value)
	}
	if value := boundedMaterial(query.Question, 1000); value != "" {
		parts = append(parts, value)
	}
	if value := boundedMaterial(strings.Join(query.PreviousGaps, "\n"), 2000); value != "" {
		parts = append(parts, value)
	}
	// Full materials are a seed query for legacy callers and knowledge-only
	// starts. Once a coverage objective exists, keeping the query local avoids
	// unrelated terms elsewhere in a long JD or resume dominating retrieval.
	if strings.TrimSpace(query.Objective) == "" {
		if value := boundedMaterial(query.JD, 12000); value != "" {
			parts = append(parts, value)
		}
		if value := boundedMaterial(query.Resume, 12000); value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		switch query.Focus {
		case interview.FocusProjects:
			parts = append(parts, "简历 项目 深挖 个人职责 量化指标 技术取舍")
		case interview.FocusKnowledge:
			parts = append(parts, "Agent 工程 原理 架构 工具调用 RAG 评测")
		default:
			parts = append(parts, "Agent 工程 项目 深挖 系统设计 技术取舍")
		}
	}
	return strings.Join(parts, "\n")
}

func boundedMaterial(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

var _ interview.KnowledgeRetriever = (*InterviewRetriever)(nil)
