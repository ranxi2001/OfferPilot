package resumediagnosis

import (
	"context"
	"strings"
	"testing"

	"offerpilot/backend/internal/harness"
	"offerpilot/backend/internal/llm"
)

type recordingVisionClient struct {
	result   Result
	messages []llm.Message
	images   []llm.ImageInput
	vision   bool
}

func (client *recordingVisionClient) ChatJSON(_ context.Context, messages []llm.Message, out any) error {
	client.messages = append([]llm.Message(nil), messages...)
	*(out.(*Result)) = client.result
	return nil
}

func (client *recordingVisionClient) ChatJSONWithImages(_ context.Context, messages []llm.Message, images []llm.ImageInput, out any) error {
	client.messages = append([]llm.Message(nil), messages...)
	client.images = append([]llm.ImageInput(nil), images...)
	client.vision = true
	*(out.(*Result)) = client.result
	return nil
}

func TestResumeDiagnosticianUsesTextAndPageImagesThroughHarness(t *testing.T) {
	client := &recordingVisionClient{result: validMultimodalResult()}
	runtime, err := harness.NewRuntime(client, harness.Options{})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := NewAgent(runtime, AgentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registered, exists := runtime.Agent(AgentID)
	if !exists || !strings.Contains(registered.SystemPrompt, "不得把整份简历当成一个段落") {
		t.Fatalf("Agent prompt lacks section constraint: %#v", registered)
	}
	content := strings.Repeat("教育背景\n工作与实习经历\n开源贡献\n项目实践\n", 50)
	result, err := agent.Diagnose(context.Background(), Request{
		Content: content,
		Images:  []string{"data:image/jpeg;base64,page1", "data:image/jpeg;base64,page2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !client.vision || len(client.images) != 2 || result.Agent != AgentID || result.TraceID == "" {
		t.Fatalf("vision=%v images=%d result=%#v", client.vision, len(client.images), result)
	}
	if len(result.Diagnosis) != 4 || result.Mode != "multimodal" || result.Layout.Score != 8 {
		t.Fatalf("result=%#v", result)
	}
	if !strings.Contains(client.messages[1].Content, "教育背景") || !strings.Contains(client.messages[1].Content, `"imageCount":2`) {
		t.Fatalf("reference context missing text/image count: %s", client.messages[1].Content)
	}
}

func TestResumeDiagnosticianSupportsTextOnlyDocuments(t *testing.T) {
	result := validMultimodalResult()
	result.Mode = "text_only"
	result.Layout = LayoutAssessment{Score: 0, Summary: "未提供页面图片，本次仅诊断文字内容。", Issues: []string{}, Suggestions: []string{}}
	client := &recordingVisionClient{result: result}
	runtime, err := harness.NewRuntime(client, harness.Options{})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := NewAgent(runtime, AgentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	output, err := agent.Diagnose(context.Background(), Request{Content: strings.Repeat("完整简历内容", 400)})
	if err != nil {
		t.Fatal(err)
	}
	if client.vision || output.Mode != "text_only" {
		t.Fatalf("vision=%v output=%#v", client.vision, output)
	}
}

func validMultimodalResult() Result {
	sections := []SectionDiagnosis{
		{Section: "教育背景", Score: 8, Evidence: []string{"中国科学院大学计算机技术硕士"}, Issues: []string{"联合培养关系表达略拥挤"}, Suggestions: []string{"压缩院系信息并突出研究方向"}, Rewrite: "中国科学院大学计算机技术硕士，聚焦 Agent Runtime、云原生调度与分布式系统性能评测。"},
		{Section: "专业技能", Score: 7, Evidence: []string{"Go、Python 与 Kubernetes 工程实践"}, Issues: []string{"部分熟悉项缺少项目证据"}, Suggestions: []string{"将技能绑定到代表项目和结果"}, Rewrite: "Go/Kubernetes：用于 Agent Sandbox 控制器、运行时适配与多集群调度开发，并通过 E2E 和性能指标验证。"},
		{Section: "工作与实习经历", Score: 9, Evidence: []string{"Warm Pool 将 p50 从 7.315 秒降至 436/565 毫秒"}, Issues: []string{}, Suggestions: []string{"补充本人负责边界和压测环境"}, Rewrite: "参与双层沙箱控制面设计并负责 Warm Pool 容量调优，在本地 10 并发压测下将端到端 p50 从 7.315 秒降至 436/565 毫秒。"},
		{Section: "开源与项目", Score: 8, Evidence: []string{"AgentCube 11 个 PR、Karmada 7 个 PR 已合入"}, Issues: []string{"项目数量较多导致主线分散"}, Suggestions: []string{"按目标岗位保留三项最高相关成果"}, Rewrite: "AgentCube/Karmada 核心贡献者：围绕 Agent Runtime 与多集群调度合入 18 个上游 PR，覆盖 API 适配、状态持久化和 CI 稳定性。"},
	}
	return Result{
		OverallScore: 82,
		Summary:      "候选人的 Agent Infra 与云原生工程证据突出，量化结果可信，但技能和项目数量偏多，需要进一步聚焦求职主线。",
		Strengths:    []string{"Agent Runtime 与 Kubernetes 经历形成清晰技术主线", "上游 PR 和性能数据提供了可核验工程证据"},
		Risks:        []string{"两页信息密度较高，部分次要项目削弱核心经历"},
		Diagnosis:    sections,
		Layout: LayoutAssessment{
			Score: 8, Summary: "双页结构清晰、标题层级稳定，但正文密度较高。",
			Issues: []string{"项目条目较密"}, Suggestions: []string{"压缩低相关项目并增加段间留白"},
		},
		Mode: "multimodal",
	}
}
