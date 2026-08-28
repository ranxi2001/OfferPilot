package webcrawler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

const (
	maxObservationText       = 5000
	maxObservationSource     = 7000
	maxResourceContent       = 12000
	maxObservationURLs       = 32
	maxObservationScriptURLs = 32
	maxScriptsPerScan        = 12
)

var (
	quotedURLPattern      = regexp.MustCompile(`["']((?:https?://|/)[^"'\s<>\\]{2,300})["']`)
	apiBasePathPattern    = regexp.MustCompile(`(?i)^/api(?:/v[0-9]+)?/?$`)
	detailEndpointPattern = regexp.MustCompile(`(?is)(?:getPositionDetail|getJobDetail|getPostingDetail).{0,800}?["'](/(?:job|position)/[^"']{1,160})["']`)
)

func (f *Fetcher) InspectPage(ctx context.Context, request Request) (PageObservation, error) {
	target, err := normalizeURL(request.URL)
	if err != nil {
		return PageObservation{}, err
	}
	page, contentType, finalURL, err := f.get(ctx, f.newHTTPClient(), target)
	if err != nil {
		return PageObservation{}, err
	}
	title, text, _ := extractDocument(contentType, page)
	scripts, links := discoverDocumentURLs(page, finalURL)
	candidates := mergeURLs(links, discoverTextURLs(string(page), finalURL, finalURL), maxObservationURLs)
	return PageObservation{
		URL:           finalURL.String(),
		Title:         compactRunes(title, 500),
		Text:          compactRunes(text, maxObservationText),
		ContentType:   contentType,
		ScriptURLs:    limitStrings(scripts, maxObservationScriptURLs),
		CandidateURLs: candidates,
		SourceExcerpt: pageSourceExcerpt(page),
	}, nil
}

func (f *Fetcher) ScanScripts(ctx context.Context, request Request) (ScriptScanObservation, error) {
	page, err := f.InspectPage(ctx, request)
	if err != nil {
		return ScriptScanObservation{}, err
	}
	scripts := prioritizedScripts(page.ScriptURLs, maxScriptsPerScan)
	if len(scripts) == 0 {
		return ScriptScanObservation{}, ErrNoContent
	}
	type scriptResult struct {
		index       int
		url         string
		observation ResourceObservation
	}
	results := make([]scriptResult, 0, len(scripts))
	var mutex sync.Mutex
	var wait sync.WaitGroup
	semaphore := make(chan struct{}, 4)
	for index, scriptURL := range scripts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-semaphore }()
			observation, fetchErr := f.FetchResource(ctx, ResourceRequest{URL: scriptURL, Referer: page.URL})
			if fetchErr != nil {
				return
			}
			mutex.Lock()
			results = append(results, scriptResult{index: index, url: scriptURL, observation: observation})
			mutex.Unlock()
		}()
	}
	wait.Wait()
	if len(results) == 0 {
		if ctx.Err() != nil {
			return ScriptScanObservation{}, ctx.Err()
		}
		return ScriptScanObservation{}, ErrNoContent
	}
	sort.Slice(results, func(i, j int) bool { return results[i].index < results[j].index })
	var content strings.Builder
	completed := make([]string, 0, len(results))
	candidates := make([]string, 0)
	templates := make([]string, 0)
	for _, result := range results {
		completed = append(completed, result.url)
		candidates = mergeURLs(candidates, result.observation.CandidateURLs, maxObservationURLs)
		templates = mergeURLs(templates, result.observation.APITemplates, 16)
		if strings.TrimSpace(result.observation.Content) != "" {
			content.WriteString("SCRIPT ")
			content.WriteString(result.url)
			content.WriteByte('\n')
			content.WriteString(result.observation.Content)
			content.WriteString("\n===\n")
		}
	}
	return ScriptScanObservation{
		PageURL: page.URL, ScriptsScanned: completed,
		Content: compactRunes(content.String(), maxResourceContent), CandidateURLs: candidates, APITemplates: templates,
	}, nil
}

func (f *Fetcher) FetchResource(ctx context.Context, input ResourceRequest) (ResourceObservation, error) {
	target, err := normalizeURL(input.URL)
	if err != nil {
		return ResourceObservation{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return ResourceObservation{}, fmt.Errorf("%w: create resource request", ErrInvalidURL)
	}
	req.Header.Set("User-Agent", f.options.UserAgent)
	req.Header.Set("Accept", "application/json,text/plain,text/html,application/javascript,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.7")
	if referer, refererErr := normalizeURL(input.Referer); refererErr == nil {
		req.Header.Set("Referer", referer.String())
	}
	response, err := f.newHTTPClient().Do(req)
	if err != nil {
		return ResourceObservation{}, classifyRequestError(err)
	}
	contentType := response.Header.Get("Content-Type")
	finalURL := response.Request.URL
	data, err := readResponse(response, f.options.MaxResponseBytes)
	if err != nil {
		return ResourceObservation{}, err
	}
	rootURL := finalURL
	if referer, refererErr := normalizeURL(input.Referer); refererErr == nil {
		rootURL = referer
	}
	candidates := discoverTextURLs(string(data), finalURL, rootURL)
	templates := discoverAPITemplates(string(data), rootURL)
	return ResourceObservation{
		URL:           finalURL.String(),
		ContentType:   contentType,
		Content:       resourceExcerpt(data, contentType),
		CandidateURLs: candidates,
		APITemplates:  templates,
	}, nil
}

func discoverDocumentURLs(page []byte, base *url.URL) ([]string, []string) {
	document, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, nil
	}
	scripts := make([]string, 0)
	links := make([]string, 0)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "script":
				if resolved := resolveCandidateURL(attribute(node, "src"), base); resolved != "" {
					scripts = appendUnique(scripts, resolved, maxObservationScriptURLs)
				}
			case "a", "link":
				if resolved := resolveCandidateURL(attribute(node, "href"), base); resolved != "" && interestingURL(resolved) {
					links = appendUnique(links, resolved, maxObservationURLs)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	return scripts, links
}

func discoverTextURLs(content string, base, rootBase *url.URL) []string {
	result := make([]string, 0)
	fragments := make([]string, 0)
	for _, match := range quotedURLPattern.FindAllStringSubmatch(content, 200) {
		if len(match) != 2 {
			continue
		}
		fragments = append(fragments, match[1])
		resolutionBase := base
		if strings.HasPrefix(match[1], "/") && rootBase != nil {
			resolutionBase = rootBase
		}
		resolved := resolveCandidateURL(match[1], resolutionBase)
		if resolved != "" && interestingURL(resolved) {
			result = appendUnique(result, resolved, maxObservationURLs)
		}
	}
	result = mergeURLs(result, composedAPICandidates(fragments, rootBase), maxObservationURLs)
	return result
}

func composedAPICandidates(fragments []string, root *url.URL) []string {
	if root == nil {
		return nil
	}
	prefixes := make([]string, 0)
	endpoints := make([]string, 0)
	for _, fragment := range fragments {
		path := strings.TrimSpace(strings.ReplaceAll(fragment, `\/`, "/"))
		lower := strings.ToLower(path)
		if apiBasePathPattern.MatchString(lower) {
			prefixes = appendUnique(prefixes, strings.TrimRight(path, "/"), 8)
			continue
		}
		if strings.HasPrefix(path, "/") && (strings.HasPrefix(lower, "/job/") || strings.HasPrefix(lower, "/position/") || strings.HasPrefix(lower, "/search/")) {
			endpoints = appendUnique(endpoints, path, 24)
		}
	}
	result := make([]string, 0)
	for _, prefix := range prefixes {
		for _, endpoint := range endpoints {
			resolved := resolveCandidateURL(prefix+endpoint, root)
			if resolved != "" {
				result = appendUnique(result, resolved, maxObservationURLs)
			}
		}
	}
	return result
}

func discoverAPITemplates(content string, root *url.URL) []string {
	if root == nil {
		return nil
	}
	fragments := make([]string, 0)
	for _, match := range quotedURLPattern.FindAllStringSubmatch(content, 400) {
		if len(match) == 2 {
			fragments = append(fragments, match[1])
		}
	}
	prefixes := make([]string, 0)
	for _, fragment := range fragments {
		path := strings.TrimSpace(strings.ReplaceAll(fragment, `\/`, "/"))
		if apiBasePathPattern.MatchString(path) {
			prefixes = appendUnique(prefixes, strings.TrimRight(path, "/"), 8)
		}
	}
	if len(prefixes) == 0 {
		return nil
	}
	result := make([]string, 0)
	for _, match := range detailEndpointPattern.FindAllStringSubmatch(content, 24) {
		if len(match) != 2 {
			continue
		}
		endpoint := strings.TrimSpace(strings.ReplaceAll(match[1], `\/`, "/"))
		if !strings.HasSuffix(endpoint, "/") {
			endpoint += "/"
		}
		for _, prefix := range prefixes {
			resolved := resolveCandidateURL(prefix+endpoint, root)
			if resolved != "" {
				result = appendUnique(result, resolved+"{id}", 16)
			}
		}
	}
	return result
}

func resolveCandidateURL(raw string, base *url.URL) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\/`, "/"))
	if raw == "" || strings.HasPrefix(raw, "//") {
		if strings.HasPrefix(raw, "//") {
			raw = base.Scheme + ":" + raw
		} else {
			return ""
		}
	}
	reference, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	resolved.Fragment = ""
	return resolved.String()
}

func interestingURL(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"/api/", "job", "position", "career", "detail", "search"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func pageSourceExcerpt(page []byte) string {
	content := string(page)
	windows := relevantWindows(content, []string{"__next_data__", "application/ld+json", "application/json", "/api/", "job", "position"}, maxObservationSource)
	if strings.TrimSpace(windows) != "" {
		return windows
	}
	return compactRunes(content, maxObservationSource)
}

func resourceExcerpt(data []byte, contentType string) string {
	content := strings.TrimSpace(string(data))
	lowerType := strings.ToLower(contentType)
	if strings.Contains(lowerType, "json") || strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
		return compactRunes(content, maxResourceContent)
	}
	return relevantWindows(content, []string{"/api/v1", "/api/", "job/posts", "position/detail", "description", "requirement", "qualification"}, maxResourceContent)
}

func relevantWindows(content string, markers []string, limit int) string {
	lower := strings.ToLower(content)
	var result strings.Builder
	for _, marker := range markers {
		searchFrom := 0
		for result.Len() < limit {
			index := strings.Index(lower[searchFrom:], strings.ToLower(marker))
			if index < 0 {
				break
			}
			index += searchFrom
			start := max(0, index-500)
			end := min(len(content), index+1500)
			result.WriteString(content[start:end])
			result.WriteString("\n---\n")
			searchFrom = end
		}
		if result.Len() >= limit {
			break
		}
	}
	if result.Len() == 0 {
		return compactRunes(content, limit)
	}
	return compactRunes(result.String(), limit)
}

func compactRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "...[truncated]"
}

func appendUnique(values []string, value string, limit int) []string {
	if len(values) >= limit {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func mergeURLs(first, second []string, limit int) []string {
	result := make([]string, 0, min(limit, len(first)+len(second)))
	for _, values := range [][]string{first, second} {
		for _, value := range values {
			result = appendUnique(result, value, limit)
		}
	}
	return result
}

func limitStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func prioritizedScripts(values []string, limit int) []string {
	if len(values) <= limit {
		return append([]string(nil), values...)
	}
	leading := min(2, limit)
	trailing := limit - leading
	result := append([]string(nil), values[:leading]...)
	return append(result, values[len(values)-trailing:]...)
}
