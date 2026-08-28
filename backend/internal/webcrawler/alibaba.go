package webcrawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	alibabaPositionPath   = regexp.MustCompile(`^/campus/position/([0-9]+)/?$`)
	alibabaTokenPattern   = regexp.MustCompile(`__token__\s*:\s*["']([^"']+)["']`)
	alibabaChannelPattern = regexp.MustCompile(`(?s)channelCodeMap\s*:\s*\{.{0,1000}?\bcampus\s*:\s*["']([^"']+)["']`)
)

type alibabaPositionEnvelope struct {
	Success  bool                   `json:"success"`
	ErrorMsg string                 `json:"errorMsg"`
	Content  alibabaPositionContent `json:"content"`
}

type alibabaPositionContent struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Requirement   string   `json:"requirement"`
	WorkLocations []string `json:"workLocations"`
	CategoryName  string   `json:"categoryName"`
	BatchName     string   `json:"batchName"`
	CircleNames   []string `json:"circleNames"`
}

func alibabaCampusPositionID(target *url.URL) (string, bool) {
	if !strings.EqualFold(target.Hostname(), "campus-talent.alibaba.com") {
		return "", false
	}
	match := alibabaPositionPath.FindStringSubmatch(target.EscapedPath())
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func (f *Fetcher) fetchAlibabaCampusPosition(ctx context.Context, client *http.Client, pageURL *url.URL, positionID string, page []byte) (Result, error) {
	tokenMatch := alibabaTokenPattern.FindSubmatch(page)
	if len(tokenMatch) != 2 {
		return Result{}, fmt.Errorf("%w: Alibaba page token is missing", ErrUpstreamFailed)
	}
	channel := "new_campus_group_official_site"
	if match := alibabaChannelPattern.FindSubmatch(page); len(match) == 2 && strings.TrimSpace(string(match[1])) != "" {
		channel = strings.TrimSpace(string(match[1]))
	}
	payload, _ := json.Marshal(map[string]string{
		"id": positionID, "channel": channel, "language": "zh",
	})
	detailURL := &url.URL{
		Scheme:   pageURL.Scheme,
		Host:     pageURL.Host,
		Path:     "/position/detail",
		RawQuery: url.Values{"_csrf": []string{string(tokenMatch[1])}}.Encode(),
	}
	body, err := f.postJSON(ctx, client, detailURL, pageURL, payload)
	if err != nil {
		return Result{}, err
	}
	var envelope alibabaPositionEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: invalid Alibaba position response", ErrUpstreamFailed)
	}
	if !envelope.Success {
		message := strings.TrimSpace(envelope.ErrorMsg)
		if message == "" {
			message = "position detail request was rejected"
		}
		return Result{}, fmt.Errorf("%w: %s", ErrUpstreamFailed, message)
	}
	text := formatAlibabaPosition(envelope.Content)
	if utf8.RuneCountInString(text) < 80 {
		return Result{}, ErrNoContent
	}
	return Result{
		Text: text, Title: strings.TrimSpace(envelope.Content.Name),
		Source: pageURL.String(), Provider: "alibaba-campus",
	}, nil
}

func formatAlibabaPosition(position alibabaPositionContent) string {
	sections := make([]string, 0, 6)
	if name := strings.TrimSpace(position.Name); name != "" {
		sections = append(sections, "职位："+name)
	}
	metadata := make([]string, 0, 2)
	if batch := strings.TrimSpace(position.BatchName); batch != "" {
		metadata = append(metadata, batch)
	}
	if category := strings.TrimSpace(position.CategoryName); category != "" {
		metadata = append(metadata, category)
	}
	if len(metadata) > 0 {
		sections = append(sections, "招聘类型："+strings.Join(metadata, " · "))
	}
	if locations := cleanList(position.WorkLocations); len(locations) > 0 {
		sections = append(sections, "工作地点："+strings.Join(locations, "、"))
	}
	if circles := cleanList(position.CircleNames); len(circles) > 0 {
		sections = append(sections, "招聘组织："+strings.Join(circles, "、"))
	}
	if description := normalizeExtractedText(position.Description); description != "" {
		sections = append(sections, "岗位职责\n"+description)
	}
	if requirement := normalizeExtractedText(position.Requirement); requirement != "" {
		sections = append(sections, "岗位要求与加分项\n"+requirement)
	}
	return strings.Join(sections, "\n\n")
}

func cleanList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
