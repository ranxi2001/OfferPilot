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

var byteDancePositionPath = regexp.MustCompile(`^/campus/position/([0-9]+)/detail/?$`)

type byteDancePositionEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Detail byteDancePosition `json:"job_post_detail"`
	} `json:"data"`
}

type byteDancePosition struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Requirement string `json:"requirement"`
	Code        string `json:"code"`
	CityInfo    struct {
		Name     string `json:"name"`
		I18NName string `json:"i18n_name"`
	} `json:"city_info"`
	CityList []struct {
		Name     string `json:"name"`
		I18NName string `json:"i18n_name"`
	} `json:"city_list"`
	JobCategory struct {
		Name     string `json:"name"`
		I18NName string `json:"i18n_name"`
	} `json:"job_category"`
	RecruitType struct {
		Name     string `json:"name"`
		I18NName string `json:"i18n_name"`
		Parent   struct {
			Name     string `json:"name"`
			I18NName string `json:"i18n_name"`
		} `json:"parent"`
	} `json:"recruit_type"`
}

func byteDanceCampusPositionID(target *url.URL) (string, bool) {
	if !strings.EqualFold(target.Hostname(), "jobs.bytedance.com") {
		return "", false
	}
	match := byteDancePositionPath.FindStringSubmatch(target.EscapedPath())
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func (f *Fetcher) fetchByteDanceCampusPosition(ctx context.Context, client *http.Client, pageURL *url.URL, positionID string) (Result, error) {
	detailURL := &url.URL{
		Scheme: pageURL.Scheme,
		Host:   pageURL.Host,
		Path:   "/api/v1/job/posts/" + positionID,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detailURL.String(), nil)
	if err != nil {
		return Result{}, fmt.Errorf("%w: create ByteDance detail request", ErrInvalidURL)
	}
	req.Header.Set("User-Agent", f.options.UserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.7")
	req.Header.Set("Referer", pageURL.String())
	response, err := client.Do(req)
	if err != nil {
		return Result{}, classifyRequestError(err)
	}
	body, err := readResponse(response, f.options.MaxResponseBytes)
	if err != nil {
		return Result{}, err
	}
	var envelope byteDancePositionEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: invalid ByteDance position response", ErrUpstreamFailed)
	}
	if envelope.Code != 0 || strings.TrimSpace(envelope.Data.Detail.ID) == "" {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = "position detail request was rejected"
		}
		return Result{}, fmt.Errorf("%w: %s", ErrUpstreamFailed, message)
	}
	text := formatByteDancePosition(envelope.Data.Detail)
	if utf8.RuneCountInString(text) < 80 {
		return Result{}, ErrNoContent
	}
	return Result{
		Text: text, Title: strings.TrimSpace(envelope.Data.Detail.Title),
		Source: pageURL.String(), Provider: "bytedance-campus",
	}, nil
}

func formatByteDancePosition(position byteDancePosition) string {
	sections := make([]string, 0, 7)
	if title := strings.TrimSpace(position.Title); title != "" {
		sections = append(sections, "职位："+title)
	}
	metadata := cleanList([]string{
		localizedName(position.RecruitType.Parent.I18NName, position.RecruitType.Parent.Name),
		localizedName(position.RecruitType.I18NName, position.RecruitType.Name),
		localizedName(position.JobCategory.I18NName, position.JobCategory.Name),
	})
	if len(metadata) > 0 {
		sections = append(sections, "招聘类型："+strings.Join(metadata, " · "))
	}
	locations := make([]string, 0, len(position.CityList)+1)
	locations = append(locations, localizedName(position.CityInfo.I18NName, position.CityInfo.Name))
	for _, city := range position.CityList {
		locations = append(locations, localizedName(city.I18NName, city.Name))
	}
	if locations = cleanList(locations); len(locations) > 0 {
		sections = append(sections, "工作地点："+strings.Join(locations, "、"))
	}
	if code := strings.TrimSpace(position.Code); code != "" {
		sections = append(sections, "职位编号："+code)
	}
	if description := normalizeExtractedText(position.Description); description != "" {
		sections = append(sections, "岗位职责\n"+description)
	}
	if requirement := normalizeExtractedText(position.Requirement); requirement != "" {
		sections = append(sections, "岗位要求与加分项\n"+requirement)
	}
	return strings.Join(sections, "\n\n")
}

func localizedName(preferred, fallback string) string {
	if preferred = strings.TrimSpace(preferred); preferred != "" {
		return preferred
	}
	return strings.TrimSpace(fallback)
}
