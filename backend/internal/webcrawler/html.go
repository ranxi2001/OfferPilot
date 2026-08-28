package webcrawler

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

var ignoredElements = map[string]bool{
	"script": true, "style": true, "noscript": true, "svg": true,
	"nav": true, "header": true, "footer": true,
}

var blockElements = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"br": true, "div": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "li": true, "main": true,
	"p": true, "section": true, "table": true, "tr": true,
}

func extractDocument(contentType string, data []byte) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(contentType), "text/plain") {
		text := normalizeExtractedText(string(data))
		if utf8.RuneCountInString(text) < 40 {
			return "", "", ErrNoContent
		}
		return "", text, nil
	}
	document, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", "", fmt.Errorf("%w: parse HTML", ErrNoContent)
	}
	var title string
	var text strings.Builder
	walkHTML(document, false, &title, &text)
	normalized := normalizeExtractedText(text.String())
	if utf8.RuneCountInString(normalized) < 40 {
		return "", "", ErrNoContent
	}
	return strings.TrimSpace(title), normalized, nil
}

func walkHTML(node *html.Node, ignored bool, title *string, text *strings.Builder) {
	if node.Type == html.ElementNode {
		name := strings.ToLower(node.Data)
		ignored = ignored || ignoredElements[name]
		if name == "title" && node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
			*title = normalizeInlineText(node.FirstChild.Data)
		}
		if !ignored && blockElements[name] {
			text.WriteByte('\n')
		}
	}
	if node.Type == html.TextNode && !ignored && strings.ToLower(parentElement(node)) != "title" {
		value := normalizeInlineText(node.Data)
		if value != "" {
			text.WriteString(value)
			text.WriteByte(' ')
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkHTML(child, ignored, title, text)
	}
	if node.Type == html.ElementNode && !ignored && blockElements[strings.ToLower(node.Data)] {
		text.WriteByte('\n')
	}
}

func parentElement(node *html.Node) string {
	if node.Parent != nil && node.Parent.Type == html.ElementNode {
		return node.Parent.Data
	}
	return ""
}

func normalizeInlineText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func normalizeExtractedText(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r", ""), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = normalizeInlineText(line)
		if line == "" {
			if len(result) > 0 && result[len(result)-1] != "" {
				result = append(result, "")
			}
			continue
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}
