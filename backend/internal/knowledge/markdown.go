package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func parseMarkdown(source, markdown string) []Entry {
	matches := questionHeading.FindAllStringSubmatchIndex(markdown, -1)
	if len(matches) == 0 {
		return nil
	}
	title := markdownTitle(source, markdown)
	entries := make([]Entry, 0, len(matches))
	ordinals := make(map[string]int)
	for idx, match := range matches {
		question := strings.TrimSpace(markdown[match[2]:match[3]])
		if question == "" {
			continue
		}
		bodyStart := match[1]
		bodyEnd := len(markdown)
		if idx+1 < len(matches) {
			bodyEnd = matches[idx+1][0]
		}
		body := strings.TrimSpace(markdown[bodyStart:bodyEnd])
		plain := plainText(body)
		ordinalKey := strings.ToLower(question)
		ordinal := ordinals[ordinalKey]
		ordinals[ordinalKey]++
		entries = append(entries, Entry{
			ID:       entryID(source, question, ordinal),
			Source:   source,
			Title:    title,
			Question: question,
			Excerpt:  truncateRunes(plain, defaultExcerptRunes),
			body:     plain,
		})
	}
	return entries
}

func markdownTitle(source, markdown string) string {
	if match := titleHeading.FindStringSubmatch(markdown); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	if match := frontTitle.FindStringSubmatch(markdown); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	base := filepath.Base(source)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func entryID(source, question string, ordinal int) string {
	digest := sha256Sum(source + "\x00" + question + fmt.Sprintf("\x00%d", ordinal))
	return "kb_" + digest[:20]
}

func sha256Sum(value string) string {
	digest := sha256Bytes([]byte(value))
	return hexBytes(digest)
}

func sha256Bytes(value []byte) []byte {
	digest := sha256.Sum256(value)
	return digest[:]
}

func hexBytes(value []byte) string {
	return hex.EncodeToString(value)
}

func plainText(markdown string) string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence {
			trimmed = strings.TrimLeft(trimmed, "#> -*+\t")
		}
		trimmed = markdownLink.ReplaceAllString(trimmed, "$1")
		trimmed = htmlTag.ReplaceAllString(trimmed, " ")
		trimmed = strings.NewReplacer(
			"**", "", "__", "", "`", "", "~~", "",
		).Replace(trimmed)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.TrimSpace(spaceRun.ReplaceAllString(strings.Join(cleaned, " "), " "))
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}
