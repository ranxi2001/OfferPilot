package knowledge

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"offerpilot/backend/internal/interview"
)

func TestLoadParsesEveryQuestionBlock(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	directory := filepath.Join(root, "topic")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	markdown := `---
title: ignored frontmatter title
---
# 分布式系统拷打

### Q：如何处理缓存雪崩？

第一题答案，随机过期时间。

#### 追问

怎样预热？

### Q: 如何处理缓存击穿？

第二题答案，singleflight 与互斥锁。

## Q：如何处理缓存穿透？

第三题答案，布隆过滤器。
`
	path := filepath.Join(directory, "cache.md")
	if err := os.WriteFile(path, []byte(markdown), 0o600); err != nil {
		t.Fatal(err)
	}

	index, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if index.Len() != 3 {
		t.Fatalf("Len()=%d, want three question documents", index.Len())
	}
	entries := index.Entries()
	if entries[0].Title != "分布式系统拷打" || entries[0].Source != "topic/cache.md" {
		t.Fatalf("unexpected metadata: %#v", entries[0])
	}
	if strings.Contains(entries[0].Excerpt, "缓存击穿") {
		t.Fatalf("first question swallowed the next block: %q", entries[0].Excerpt)
	}

	results := index.Search("singleflight 缓存击穿", 2)
	if len(results) == 0 || results[0].Question != "如何处理缓存击穿？" {
		t.Fatalf("unexpected top result: %#v", results)
	}
	if results[0].ID == "" || results[0].Excerpt == "" || results[0].Score <= 0 {
		t.Fatalf("result is missing required fields: %#v", results[0])
	}
}

func TestRepositoryKnowledgeIsIndexedByQuestionNotFile(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "..", "knowledge"))
	if _, err := os.Stat(root); err != nil {
		t.Skipf("repository knowledge directory is unavailable: %v", err)
	}
	fileCount := 0
	if err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			fileCount++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	index, err := Load(root)
	if err != nil {
		t.Fatalf("Load repository knowledge: %v", err)
	}
	if index.Len() <= fileCount {
		t.Fatalf("indexed %d entries from %d files; expected question-granularity expansion", index.Len(), fileCount)
	}
	t.Logf("indexed %d question blocks from %d Markdown files", index.Len(), fileCount)
	results := index.Search("Harness Engineering", 3)
	if len(results) == 0 {
		t.Fatal("real knowledge index returned no Harness result")
	}
	retriever, err := NewInterviewRetriever(index, 3)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := retriever.Retrieve(context.Background(), interview.KnowledgeQuery{
		Focus: interview.FocusKnowledge,
		JD:    "负责 Agent Harness Engineering、子 Agent 调度与结构化输出",
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(documents) == 0 || len(documents) > 3 || documents[0].ID == "" || documents[0].Content == "" {
		t.Fatalf("unexpected interview knowledge documents: %#v", documents)
	}
}
