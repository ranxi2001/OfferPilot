package jobmatch

import (
	"context"
	"strings"
	"testing"

	"offerpilot/backend/internal/harness"
	"offerpilot/backend/internal/llm"
)

type scriptedMatchClient struct {
	results  []Result
	calls    int
	messages [][]llm.Message
}

func (client *scriptedMatchClient) ChatJSON(_ context.Context, messages []llm.Message, out any) error {
	client.messages = append(client.messages, append([]llm.Message(nil), messages...))
	result := out.(*Result)
	*result = client.results[min(client.calls, len(client.results)-1)]
	client.calls++
	return nil
}

func TestResumeMatcherUsesHarnessSemanticRoleAndEvidenceContract(t *testing.T) {
	client := &scriptedMatchClient{results: []Result{validResult()}}
	runtime, err := harness.NewRuntime(client, harness.Options{})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := NewAgent(runtime, AgentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registered, exists := runtime.Agent(AgentID)
	if !exists || !strings.Contains(registered.SystemPrompt, "禁止做关键词交集") || !strings.Contains(registered.SystemPrompt, "mustHave") {
		t.Fatalf("resume matcher prompt lacks semantic scoring constraints: %#v", registered)
	}
	request := Request{
		JD:     "负责大规模 AI 基础设施、异构算力调度和高性能网络，要求博士学历。",
		Resume: "计算机技术硕士，参与 Agent Sandbox、Kubernetes 调度和开源项目，具备量化性能数据。",
	}
	result, err := agent.Match(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent != AgentID || result.TraceID == "" || len(result.TraceIDs) != 1 || result.Score != 72 {
		t.Fatalf("result=%#v", result)
	}
	if client.calls != 1 || len(client.messages) != 1 || len(client.messages[0]) != 2 {
		t.Fatalf("calls=%d messages=%#v", client.calls, client.messages)
	}
	contextMessage := client.messages[0][1].Content
	if !strings.Contains(contextMessage, request.JD) || !strings.Contains(contextMessage, request.Resume) {
		t.Fatalf("JD/resume missing from Harness reference context: %s", contextMessage)
	}
}

func TestResumeMatcherRepairsInvalidBreakdownOnce(t *testing.T) {
	invalid := validResult()
	invalid.Score = 99
	client := &scriptedMatchClient{results: []Result{invalid, validResult()}}
	runtime, err := harness.NewRuntime(client, harness.Options{})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := NewAgent(runtime, AgentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := agent.Match(context.Background(), Request{JD: "完整职位描述", Resume: "完整候选人简历"})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 || len(result.TraceIDs) != 2 || result.Score != 72 {
		t.Fatalf("calls=%d result=%#v", client.calls, result)
	}
	if !strings.Contains(client.messages[1][1].Content, "score must equal") {
		t.Fatalf("repair reason missing: %s", client.messages[1][1].Content)
	}
}

func validResult() Result {
	return Result{
		Score: 72,
		Matched: []string{
			"Agent 基础设施：具备 Sandbox 与 Runtime 上游开发经验",
			"云原生调度：有 Kubernetes 控制器和多集群调度实践",
			"工程证据：提供性能指标、合入 PR 和端到端测试结果",
		},
		Missing: []string{"学历门槛：JD 要求博士，简历当前为硕士在读"},
		Suggestions: []string{
			"突出异构算力调度与高性能网络之间可迁移的系统经验",
			"面试前准备 RDMA 与拥塞控制基础，诚实说明尚无直接项目经历",
		},
		Level:     "博士专项校招研发岗位",
		Focus:     []string{"AI 云原生基础设施", "异构算力调度", "系统性能与稳定性"},
		Summary:   "工程实践与 Agent Infra 方向高度相关，但博士学历和高性能网络研究存在明确差距。",
		Breakdown: Breakdown{MustHave: 29, Responsibilities: 21, EvidenceQuality: 16, Bonus: 6},
		Evidence: []Evidence{
			{Requirement: "Agent 基础设施", ResumeEvidence: "参与 Agent Sandbox 与 Runtime 上游开发", Verdict: "matched"},
			{Requirement: "云原生调度", ResumeEvidence: "Kubernetes 控制器和多集群调度实践", Verdict: "matched"},
			{Requirement: "博士学历", ResumeEvidence: "简历为计算机技术硕士在读", Verdict: "missing"},
		},
	}
}
