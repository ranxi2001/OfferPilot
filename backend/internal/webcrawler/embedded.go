package webcrawler

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

type embeddedJob struct {
	Title        string
	Description  string
	Requirements string
	Locations    []string
	Employment   string
	Organization string
	Identifier   string
	Score        int
}

func extractEmbeddedJobPosting(page []byte, source *url.URL) (Result, bool) {
	document, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return Result{}, false
	}
	var candidates []embeddedJob
	walkScripts(document, func(node *html.Node) {
		typeName := strings.ToLower(attribute(node, "type"))
		if typeName != "application/ld+json" && typeName != "application/json" && typeName != "text/json" {
			return
		}
		content := scriptText(node)
		if strings.TrimSpace(content) == "" {
			return
		}
		var decoded any
		decoder := json.NewDecoder(strings.NewReader(content))
		decoder.UseNumber()
		if decoder.Decode(&decoded) != nil {
			return
		}
		collectEmbeddedJobs(decoded, source, typeName == "application/ld+json", &candidates)
	})
	best := embeddedJob{}
	for _, candidate := range candidates {
		if candidate.Score > best.Score {
			best = candidate
		}
	}
	if best.Score < 5 || strings.TrimSpace(best.Title) == "" || utf8.RuneCountInString(best.Description+best.Requirements) < 80 {
		return Result{}, false
	}
	sections := []string{"职位：" + strings.TrimSpace(best.Title)}
	metadata := cleanList([]string{best.Employment, best.Organization})
	if len(metadata) > 0 {
		sections = append(sections, "招聘类型："+strings.Join(metadata, " · "))
	}
	if locations := cleanList(best.Locations); len(locations) > 0 {
		sections = append(sections, "工作地点："+strings.Join(locations, "、"))
	}
	if best.Identifier != "" {
		sections = append(sections, "职位编号："+best.Identifier)
	}
	if best.Description != "" {
		sections = append(sections, "岗位职责\n"+best.Description)
	}
	if best.Requirements != "" {
		sections = append(sections, "岗位要求与加分项\n"+best.Requirements)
	}
	return Result{
		Text: strings.Join(sections, "\n\n"), Title: strings.TrimSpace(best.Title),
		Source: source.String(), Provider: "embedded-json",
	}, true
}

func collectEmbeddedJobs(value any, source *url.URL, schema bool, result *[]embeddedJob) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectEmbeddedJobs(item, source, schema, result)
		}
	case map[string]any:
		if graph, exists := typed["@graph"]; exists {
			collectEmbeddedJobs(graph, source, true, result)
		}
		if candidate, ok := embeddedJobFromMap(typed, source, schema); ok {
			*result = append(*result, candidate)
		}
		for key, child := range typed {
			if key == "@graph" {
				continue
			}
			if _, ok := child.(map[string]any); ok {
				collectEmbeddedJobs(child, source, schema, result)
			} else if _, ok := child.([]any); ok {
				collectEmbeddedJobs(child, source, schema, result)
			}
		}
	}
}

func embeddedJobFromMap(value map[string]any, source *url.URL, schema bool) (embeddedJob, bool) {
	isJobPosting := schema && jsonTypeContains(value["@type"], "JobPosting")
	title := firstString(value, "title", "name", "jobTitle", "positionName")
	description := firstString(value, "responsibilities", "description", "jobDescription")
	requirements := firstString(value, "qualifications", "requirements", "requirement", "experienceRequirements")
	if !isJobPosting && (description == "" || requirements == "") {
		return embeddedJob{}, false
	}
	job := embeddedJob{
		Title:        textFromHTML(title),
		Description:  textFromHTML(description),
		Requirements: textFromHTML(requirements),
		Employment:   stringValue(value["employmentType"]),
		Identifier:   embeddedIdentifier(value["identifier"]),
	}
	job.Organization = nestedString(value["hiringOrganization"], "name")
	job.Locations = embeddedLocations(value["jobLocation"])
	if isJobPosting {
		job.Score += 5
	}
	if job.Title != "" {
		job.Score += 2
	}
	if utf8.RuneCountInString(job.Description) >= 50 {
		job.Score += 2
	}
	if utf8.RuneCountInString(job.Requirements) >= 30 {
		job.Score += 2
	}
	if job.Identifier != "" && strings.Contains(source.Path, job.Identifier) {
		job.Score += 3
	}
	return job, true
}

func walkScripts(node *html.Node, visit func(*html.Node)) {
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, "script") {
		visit(node)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkScripts(child, visit)
	}
}

func scriptText(node *html.Node) string {
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			text.WriteString(child.Data)
		}
	}
	return text.String()
}

func attribute(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, key) {
			return attribute.Val
		}
	}
	return ""
}

func jsonTypeContains(value any, expected string) bool {
	switch typed := value.(type) {
	case string:
		return strings.EqualFold(typed, expected)
	case []any:
		for _, item := range typed {
			if jsonTypeContains(item, expected) {
				return true
			}
		}
	}
	return false
}

func firstString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if result := stringValue(value[key]); result != "" {
			return result
		}
	}
	return ""
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func nestedString(value any, key string) string {
	if object, ok := value.(map[string]any); ok {
		return stringValue(object[key])
	}
	return ""
}

func embeddedIdentifier(value any) string {
	if text := stringValue(value); text != "" {
		return text
	}
	if object, ok := value.(map[string]any); ok {
		return firstString(object, "value", "name")
	}
	return ""
}

func embeddedLocations(value any) []string {
	items, ok := value.([]any)
	if !ok {
		items = []any{value}
	}
	locations := make([]string, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		address, _ := object["address"].(map[string]any)
		parts := []string{
			firstString(address, "addressLocality", "addressRegion"),
			stringValue(address["addressCountry"]),
		}
		parts = cleanList(parts)
		if len(parts) > 0 {
			locations = append(locations, strings.Join(parts, " "))
		}
	}
	return locations
}

func textFromHTML(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.Contains(value, "<") {
		return normalizeExtractedText(value)
	}
	document, err := html.Parse(strings.NewReader("<body>" + value + "</body>"))
	if err != nil {
		return normalizeExtractedText(value)
	}
	var title string
	var text strings.Builder
	walkHTML(document, false, &title, &text)
	return normalizeExtractedText(text.String())
}
