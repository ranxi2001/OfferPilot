package webcrawler

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const defaultUserAgent = "Mozilla/5.0 (compatible; OfferPilot-WebCrawler/1.0)"

type Options struct {
	RequestTimeout       time.Duration
	MaxResponseBytes     int64
	MaxRedirects         int
	AllowBenchmarkTunnel bool
	UserAgent            string
}

type Fetcher struct {
	options   Options
	transport http.RoundTripper
}

func NewFetcher(options Options) *Fetcher {
	options = normalizeOptions(options)
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           safeDialContext(options.AllowBenchmarkTunnel),
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ResponseHeaderTimeout: options.RequestTimeout,
		IdleConnTimeout:       30 * time.Second,
	}
	return &Fetcher{options: options, transport: transport}
}

func newFetcherWithTransport(options Options, transport http.RoundTripper) *Fetcher {
	return &Fetcher{options: normalizeOptions(options), transport: transport}
}

func normalizeOptions(options Options) Options {
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 10 * time.Second
	}
	if options.MaxResponseBytes <= 0 {
		options.MaxResponseBytes = 2 << 20
	}
	if options.MaxRedirects <= 0 {
		options.MaxRedirects = 3
	}
	if strings.TrimSpace(options.UserAgent) == "" {
		options.UserAgent = defaultUserAgent
	}
	return options
}

func (f *Fetcher) Fetch(ctx context.Context, request Request) (Result, error) {
	normalized, err := normalizeURL(request.URL)
	if err != nil {
		return Result{}, err
	}
	client := f.newHTTPClient()

	page, contentType, finalURL, err := f.get(ctx, client, normalized)
	if err != nil {
		return Result{}, err
	}
	if positionID, ok := alibabaCampusPositionID(finalURL); ok {
		return f.fetchAlibabaCampusPosition(ctx, client, finalURL, positionID, page)
	}
	if positionID, ok := byteDanceCampusPositionID(finalURL); ok {
		return f.fetchByteDanceCampusPosition(ctx, client, finalURL, positionID)
	}
	if embedded, ok := extractEmbeddedJobPosting(page, finalURL); ok {
		return embedded, nil
	}

	title, text, err := extractDocument(contentType, page)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Title: title, Source: finalURL.String(), Provider: "generic-html"}, nil
}

func (f *Fetcher) newHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Transport: f.transport,
		Jar:       jar,
		Timeout:   f.options.RequestTimeout,
		CheckRedirect: func(request *http.Request, previous []*http.Request) error {
			if len(previous) > f.options.MaxRedirects {
				return fmt.Errorf("%w: more than %d redirects", ErrUpstreamFailed, f.options.MaxRedirects)
			}
			_, err := normalizeURL(request.URL.String())
			return err
		},
	}
}

func (f *Fetcher) get(ctx context.Context, client *http.Client, target *url.URL) ([]byte, string, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, "", nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	req.Header.Set("User-Agent", f.options.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.7")
	response, err := client.Do(req)
	if err != nil {
		return nil, "", nil, classifyRequestError(err)
	}
	body, err := readResponse(response, f.options.MaxResponseBytes)
	if err != nil {
		return nil, "", nil, err
	}
	return body, response.Header.Get("Content-Type"), response.Request.URL, nil
}

func (f *Fetcher) postJSON(ctx context.Context, client *http.Client, target, referer *url.URL, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	req.Header.Set("User-Agent", f.options.UserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Referer", referer.String())
	response, err := client.Do(req)
	if err != nil {
		return nil, classifyRequestError(err)
	}
	return readResponse(response, f.options.MaxResponseBytes)
}

func readResponse(response *http.Response, maxBytes int64) ([]byte, error) {
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("%w: HTTP %d", ErrUpstreamFailed, response.StatusCode)
	}
	if response.ContentLength > maxBytes {
		return nil, fmt.Errorf("%w: limit is %d bytes", ErrResponseLarge, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrUpstreamFailed, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: limit is %d bytes", ErrResponseLarge, maxBytes)
	}
	return data, nil
}

func normalizeURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrInvalidURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return nil, ErrInvalidURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: only http and https are supported", ErrInvalidURL)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("%w: embedded credentials are not supported", ErrInvalidURL)
	}
	parsed.Fragment = ""
	return parsed, nil
}

func classifyRequestError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, ErrBlockedURL) || errors.Is(err, ErrInvalidURL) || errors.Is(err, ErrUpstreamFailed) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrUpstreamFailed, err)
}

func safeDialContext(allowBenchmarkTunnel bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid address", ErrInvalidURL)
		}
		addresses, err := resolveAddresses(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, address := range addresses {
			if !publicAddress(address, allowBenchmarkTunnel) {
				return nil, fmt.Errorf("%w: private, loopback, link-local, or reserved address", ErrBlockedURL)
			}
		}
		dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
	}
}

func resolveAddresses(ctx context.Context, host string) ([]netip.Addr, error) {
	if literal, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{literal.Unmap()}, nil
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("%w: host did not resolve", ErrUpstreamFailed)
	}
	for index := range addresses {
		addresses[index] = addresses[index].Unmap()
	}
	return addresses, nil
}

var reservedPrefixes = mustPrefixes(
	"0.0.0.0/8", "100.64.0.0/10", "169.254.0.0/16", "192.0.0.0/24",
	"192.0.2.0/24", "192.88.99.0/24", "198.51.100.0/24", "203.0.113.0/24",
	"240.0.0.0/4", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64",
	"2001::/23", "2001:db8::/32", "2002::/16", "fec0::/10",
)

var benchmarkPrefix = netip.MustParsePrefix("198.18.0.0/15")

func publicAddress(address netip.Addr, allowBenchmarkTunnel bool) bool {
	address = address.Unmap()
	if benchmarkPrefix.Contains(address) {
		return allowBenchmarkTunnel
	}
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
		return false
	}
	for _, prefix := range reservedPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func mustPrefixes(raw ...string) []netip.Prefix {
	result := make([]netip.Prefix, 0, len(raw))
	for _, value := range raw {
		result = append(result, netip.MustParsePrefix(value))
	}
	return result
}
