package webcrawler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestFetcherUsesAlibabaPositionDetailAPI(t *testing.T) {
	var calls int
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if request.Method != http.MethodGet || request.URL.Path != "/campus/position/199907640058" {
				t.Fatalf("first request = %s %s", request.Method, request.URL)
			}
			html := `<html><head><title>阿里巴巴校园招聘</title><script>
window.__sysconfig = { __token__: "csrf-token", channelCodeMap: { campus: "campus-channel" } };
</script></head><body><div id="app"></div></body></html>`
			return response(request, http.StatusOK, "text/html", html, http.Header{
				"Set-Cookie": []string{"SESSION=session-value; Path=/; HttpOnly"},
			}), nil
		case 2:
			if request.Method != http.MethodPost || request.URL.Path != "/position/detail" || request.URL.Query().Get("_csrf") != "csrf-token" {
				t.Fatalf("detail request = %s %s", request.Method, request.URL)
			}
			if cookie, err := request.Cookie("SESSION"); err != nil || cookie.Value != "session-value" {
				t.Fatalf("detail cookie=%#v err=%v", cookie, err)
			}
			var payload map[string]string
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["id"] != "199907640058" || payload["channel"] != "campus-channel" {
				t.Fatalf("detail payload=%#v", payload)
			}
			body := `{"success":true,"content":{"id":199907640058,"name":"Agent Infra工程师","batchName":"2027届应届生","categoryName":"技术类","workLocations":["北京","杭州"],"circleNames":["阿里云"],"description":"负责构建 AI Agent 全生命周期核心基础设施，包括任务调度、状态管理、安全部署以及高并发服务化交付。","requirement":"扎实的计算机基础，熟悉 Go 或 Python、云原生与 Agent 框架；有 Agent 平台和多 Agent 协同经验加分。"}}`
			return response(request, http.StatusOK, "application/json", body, nil), nil
		default:
			t.Fatalf("unexpected request %d: %s", calls, request.URL)
			return nil, nil
		}
	})
	fetcher := newFetcherWithTransport(Options{}, transport)
	result, err := fetcher.Fetch(context.Background(), Request{URL: "https://campus-talent.alibaba.com/campus/position/199907640058"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || result.Title != "Agent Infra工程师" || result.Provider != "alibaba-campus" {
		t.Fatalf("calls=%d result=%#v", calls, result)
	}
	for _, expected := range []string{"岗位职责", "岗位要求与加分项", "2027届应届生", "北京、杭州"} {
		if !strings.Contains(result.Text, expected) {
			t.Fatalf("result text missing %q: %s", expected, result.Text)
		}
	}
}

func TestFetcherUsesByteDancePositionDetailAPI(t *testing.T) {
	var calls int
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if request.Method != http.MethodGet || request.URL.Path != "/campus/position/7628936427621927173/detail" {
				t.Fatalf("page request = %s %s", request.Method, request.URL)
			}
			return response(request, http.StatusOK, "text/html", `<html><head><title>字节跳动校园招聘</title></head><body><div id="app"></div></body></html>`, nil), nil
		case 2:
			if request.Method != http.MethodGet || request.URL.Path != "/api/v1/job/posts/7628936427621927173" {
				t.Fatalf("detail request = %s %s", request.Method, request.URL)
			}
			if request.Header.Get("Referer") == "" || !strings.Contains(request.Header.Get("Accept"), "application/json") {
				t.Fatalf("detail headers = %#v", request.Header)
			}
			body := `{"code":0,"message":"ok","data":{"job_post_detail":{"id":"7628936427621927173","title":"面向大模型与AI Agent的AI云原生基础设施关键技术研究-计算","description":"负责构建面向大模型与 AI Agent 的云原生基础设施，覆盖算力调度、存储、网络、向量检索与可观测性。","requirement":"2027届博士毕业，熟悉 C++、Python 或 Go；具备机器学习、计算机网络基础，有顶会论文者优先。","code":"A112453A","city_info":{"name":"杭州","i18n_name":"杭州"},"city_list":[{"name":"杭州","i18n_name":"杭州"}],"job_category":{"name":"后端","i18n_name":"后端"},"recruit_type":{"name":"正式","i18n_name":"正式","parent":{"name":"校招","i18n_name":"校招"}}}}}`
			return response(request, http.StatusOK, "application/json", body, nil), nil
		default:
			t.Fatalf("unexpected request %d: %s", calls, request.URL)
			return nil, nil
		}
	})
	result, err := newFetcherWithTransport(Options{}, transport).Fetch(context.Background(), Request{
		URL: "https://jobs.bytedance.com/campus/position/7628936427621927173/detail?spread=5YNTDRM",
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || result.Provider != "bytedance-campus" || result.Title == "" {
		t.Fatalf("calls=%d result=%#v", calls, result)
	}
	for _, expected := range []string{"岗位职责", "岗位要求与加分项", "校招 · 正式 · 后端", "工作地点：杭州", "职位编号：A112453A"} {
		if !strings.Contains(result.Text, expected) {
			t.Fatalf("result text missing %q: %s", expected, result.Text)
		}
	}
}

func TestFetcherExtractsGenericHTMLAndRejectsPageShell(t *testing.T) {
	t.Run("content", func(t *testing.T) {
		transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := `<html><head><title>Backend Engineer</title><style>ignore</style></head><body><nav>menu</nav><main><h1>Backend Engineer</h1><p>Build reliable distributed services and own production observability.</p><p>Experience with Go, SQL, queues, caching, and Kubernetes is required.</p></main></body></html>`
			return response(request, http.StatusOK, "text/html", body, nil), nil
		})
		result, err := newFetcherWithTransport(Options{}, transport).Fetch(context.Background(), Request{URL: "https://jobs.example/42"})
		if err != nil {
			t.Fatal(err)
		}
		if result.Title != "Backend Engineer" || strings.Contains(result.Text, "menu") || !strings.Contains(result.Text, "Kubernetes") {
			t.Fatalf("unexpected result: %#v", result)
		}
	})

	t.Run("shell", func(t *testing.T) {
		transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return response(request, http.StatusOK, "text/html", `<html><head><title>Recruitment</title></head><body><div id="app"></div></body></html>`, nil), nil
		})
		_, err := newFetcherWithTransport(Options{}, transport).Fetch(context.Background(), Request{URL: "https://jobs.example/42"})
		if err != ErrNoContent {
			t.Fatalf("Fetch() error=%v, want ErrNoContent", err)
		}
	})
}

func TestFetcherExtractsEmbeddedJobPostingWithoutModelFallback(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `<html><head><title>Careers</title><script type="application/ld+json">{
  "@context":"https://schema.org","@type":"JobPosting","title":"Platform Engineer",
  "description":"<p>Build and operate a reliable distributed compute platform for AI workloads.</p>",
  "qualifications":"<ul><li>Strong Go and Kubernetes experience.</li><li>Experience with observability and incident response.</li></ul>",
  "employmentType":"FULL_TIME","identifier":{"value":"JOB-42"},
  "hiringOrganization":{"name":"Example Cloud"},
  "jobLocation":{"address":{"addressLocality":"Shanghai","addressCountry":"CN"}}
}</script></head><body><div id="root"></div></body></html>`
		return response(request, http.StatusOK, "text/html", body, nil), nil
	})
	result, err := newFetcherWithTransport(Options{}, transport).Fetch(context.Background(), Request{URL: "https://jobs.example/JOB-42"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "embedded-json" || result.Title != "Platform Engineer" {
		t.Fatalf("result=%#v", result)
	}
	for _, expected := range []string{"岗位职责", "岗位要求与加分项", "Example Cloud", "Shanghai CN", "JOB-42"} {
		if !strings.Contains(result.Text, expected) {
			t.Fatalf("result text missing %q: %s", expected, result.Text)
		}
	}
}

func TestPublicAddressPolicyBlocksInternalAndReservedNetworks(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "192.0.2.1", "::1", "fc00::1"} {
		if publicAddress(netip.MustParseAddr(raw), false) {
			t.Fatalf("publicAddress(%s)=true", raw)
		}
	}
	if !publicAddress(netip.MustParseAddr("1.1.1.1"), false) {
		t.Fatal("public address was blocked")
	}
	if publicAddress(netip.MustParseAddr("198.18.0.1"), false) || !publicAddress(netip.MustParseAddr("198.18.0.1"), true) {
		t.Fatal("benchmark tunnel policy mismatch")
	}
}

func TestScriptRootRelativeAPICandidateResolvesAgainstJobPage(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://cdn.example/app.js" {
			t.Fatalf("resource request=%s", request.URL)
		}
		return response(request, http.StatusOK, "application/javascript", `const detail = "/api/jobs/42";`, nil), nil
	})
	observation, err := newFetcherWithTransport(Options{}, transport).FetchResource(context.Background(), ResourceRequest{
		URL: "https://cdn.example/app.js", Referer: "https://jobs.example/positions/42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observation.CandidateURLs) != 1 || observation.CandidateURLs[0] != "https://jobs.example/api/jobs/42" {
		t.Fatalf("candidate URLs=%v", observation.CandidateURLs)
	}
}

func TestScriptInspectorComposesBundledAPIBaseAndEndpoint(t *testing.T) {
	cdn, _ := url.Parse("https://cdn.example/app.js")
	jobSite, _ := url.Parse("https://jobs.example/positions/42")
	candidates := discoverTextURLs(`const base = "/api/v1"; const detail = id => base + "/job/posts/" + id;`, cdn, jobSite)
	found := false
	for _, candidate := range candidates {
		if candidate == "https://jobs.example/api/v1/job/posts/" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("composed API candidate missing: %v", candidates)
	}
}

func TestScriptInspectorFindsHighConfidenceDetailTemplate(t *testing.T) {
	jobSite, _ := url.Parse("https://jobs.example/positions/42")
	templates := discoverAPITemplates(`const base = "/api/v1"; const api = { getPositionDetail:function(id){return base + "/job/posts/" + id} };`, jobSite)
	if len(templates) != 1 || templates[0] != "https://jobs.example/api/v1/job/posts/{id}" {
		t.Fatalf("API templates=%v", templates)
	}
}

func TestScriptScannerPrioritizesEntryAndTrailingBundles(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/positions/42" {
			var page strings.Builder
			page.WriteString("<html><head>")
			for index := 0; index < 15; index++ {
				page.WriteString(fmt.Sprintf(`<script src="/static/%d.js"></script>`, index))
			}
			page.WriteString("</head><body><div id=\"app\"></div></body></html>")
			return response(request, http.StatusOK, "text/html", page.String(), nil), nil
		}
		body := `const chunk = true;`
		if request.URL.Path == "/static/14.js" {
			body = `const endpoint = "/api/jobs/42";`
		}
		return response(request, http.StatusOK, "application/javascript", body, nil), nil
	})
	scan, err := newFetcherWithTransport(Options{}, transport).ScanScripts(context.Background(), Request{
		URL: "https://jobs.example/positions/42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.ScriptsScanned) != 12 {
		t.Fatalf("scripts scanned=%d, want 12: %v", len(scan.ScriptsScanned), scan.ScriptsScanned)
	}
	if !strings.Contains(scan.Content, "/api/jobs/42") || len(scan.CandidateURLs) != 1 || scan.CandidateURLs[0] != "https://jobs.example/api/jobs/42" {
		t.Fatalf("scan=%#v", scan)
	}
}

func response(request *http.Request, status int, contentType, body string, extra http.Header) *http.Response {
	header := make(http.Header)
	header.Set("Content-Type", contentType)
	for key, values := range extra {
		header[key] = append([]string(nil), values...)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
