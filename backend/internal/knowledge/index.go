// Package knowledge loads the repository Markdown knowledge base at question
// granularity and provides a dependency-free lexical top-K search.
package knowledge

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const defaultExcerptRunes = 520

var (
	questionHeading = regexp.MustCompile(`(?m)^#{2,6}[ \t]+Q[ \t]*[：:][ \t]*(.+?)[ \t]*\r?$`)
	titleHeading    = regexp.MustCompile(`(?m)^#[ \t]+([^#\r\n].*?)[ \t]*\r?$`)
	frontTitle      = regexp.MustCompile(`(?m)^title:[ \t]*["']?(.+?)["']?[ \t]*\r?$`)
	markdownLink    = regexp.MustCompile(`!?\[([^]]*)\]\([^)]*\)`)
	htmlTag         = regexp.MustCompile(`<[^>]+>`)
	spaceRun        = regexp.MustCompile(`\s+`)
)

type Entry struct {
	ID       string  `json:"id"`
	Source   string  `json:"source"`
	Title    string  `json:"title"`
	Question string  `json:"question"`
	Excerpt  string  `json:"excerpt"`
	Score    float64 `json:"score"`

	body string
}

type document struct {
	entry Entry
	terms map[string]int
	len   int
}

// Index is immutable after construction and safe for concurrent searches.
type Index struct {
	documents []document
	df        map[string]int
	avgLen    float64
}

// Load recursively reads all Markdown files beneath root. Each Q heading is a
// separate indexed document, even when a source file contains many questions.
func Load(root string) (*Index, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("knowledge: root directory is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("knowledge: stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("knowledge: %s is not a directory", root)
	}

	var entries []Entry
	err = filepath.WalkDir(root, func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.IsDir() || !strings.EqualFold(filepath.Ext(item.Name()), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative source %s: %w", path, err)
		}
		entries = append(entries, parseMarkdown(filepath.ToSlash(relative), string(data))...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("knowledge: walk root: %w", err)
	}
	return buildIndex(entries), nil
}

// NewIndex builds an index from entries, primarily for programmatic knowledge
// sources. Excerpt is used as searchable body when no parsed Markdown body is
// available.
func NewIndex(entries []Entry) *Index {
	copyEntries := make([]Entry, len(entries))
	copy(copyEntries, entries)
	for i := range copyEntries {
		if copyEntries[i].body == "" {
			copyEntries[i].body = copyEntries[i].Excerpt
		}
		copyEntries[i].Score = 0
	}
	return buildIndex(copyEntries)
}

func buildIndex(entries []Entry) *Index {
	index := &Index{df: make(map[string]int)}
	var totalLength int
	for _, entry := range entries {
		weighted := entry.Title + " " + entry.Title + " " +
			entry.Question + " " + entry.Question + " " + entry.Question + " " + entry.body
		tokens := tokenize(weighted)
		terms := make(map[string]int, len(tokens))
		for _, token := range tokens {
			terms[token]++
		}
		for token := range terms {
			index.df[token]++
		}
		totalLength += len(tokens)
		index.documents = append(index.documents, document{entry: entry, terms: terms, len: len(tokens)})
	}
	if len(index.documents) > 0 {
		index.avgLen = float64(totalLength) / float64(len(index.documents))
	}
	if index.avgLen == 0 {
		index.avgLen = 1
	}
	return index
}

func (i *Index) Len() int {
	if i == nil {
		return 0
	}
	return len(i.documents)
}

func (i *Index) Entries() []Entry {
	if i == nil {
		return nil
	}
	result := make([]Entry, len(i.documents))
	for idx, document := range i.documents {
		result[idx] = publicEntry(document.entry, 0)
	}
	return result
}

// Search returns only positively matching entries, ordered by descending
// BM25 score. limit <= 0 uses a conservative default of five.
func (i *Index) Search(query string, limit int) []Entry {
	if i == nil || len(i.documents) == 0 || strings.TrimSpace(query) == "" {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}
	queryTokens := unique(tokenize(query))
	if len(queryTokens) == 0 {
		return nil
	}

	type scoredDocument struct {
		entry Entry
		score float64
	}
	results := make([]scoredDocument, 0, min(limit, len(i.documents)))
	normalizedQuery := normalizeForPhrase(query)
	hasStrongQueryTerm := false
	for _, token := range queryTokens {
		if utf8.RuneCountInString(token) > 1 {
			hasStrongQueryTerm = true
			break
		}
	}
	for _, document := range i.documents {
		if hasStrongQueryTerm && !matchesStrongTerm(document, queryTokens) {
			continue
		}
		score := i.bm25(document, queryTokens)
		if score == 0 {
			continue
		}
		normalizedQuestion := normalizeForPhrase(document.entry.Question)
		normalizedBody := normalizeForPhrase(document.entry.body)
		if utf8.RuneCountInString(normalizedQuery) >= 2 {
			if strings.Contains(normalizedQuestion, normalizedQuery) {
				score += 6
			} else if strings.Contains(normalizedBody, normalizedQuery) {
				score += 2
			}
		}
		results = append(results, scoredDocument{entry: document.entry, score: score})
	}
	sort.Slice(results, func(left, right int) bool {
		if results[left].score == results[right].score {
			return results[left].entry.ID < results[right].entry.ID
		}
		return results[left].score > results[right].score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	entries := make([]Entry, len(results))
	for idx, result := range results {
		entries[idx] = publicEntry(result.entry, math.Round(result.score*1e6)/1e6)
	}
	return entries
}

func matchesStrongTerm(document document, queryTokens []string) bool {
	for _, token := range queryTokens {
		if utf8.RuneCountInString(token) > 1 && document.terms[token] > 0 {
			return true
		}
	}
	return false
}

func (i *Index) bm25(document document, queryTokens []string) float64 {
	const k1 = 1.35
	const b = 0.72
	documentCount := float64(len(i.documents))
	var score float64
	for _, token := range queryTokens {
		frequency := document.terms[token]
		if frequency == 0 {
			continue
		}
		documentFrequency := float64(i.df[token])
		idf := math.Log(1 + (documentCount-documentFrequency+0.5)/(documentFrequency+0.5))
		tf := float64(frequency)
		normalizer := tf + k1*(1-b+b*float64(document.len)/i.avgLen)
		score += idf * (tf * (k1 + 1) / normalizer)
	}
	return score
}

func publicEntry(entry Entry, score float64) Entry {
	return Entry{
		ID:       entry.ID,
		Source:   entry.Source,
		Title:    entry.Title,
		Question: entry.Question,
		Excerpt:  entry.Excerpt,
		Score:    score,
	}
}
