package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"offerpilot/backend/internal/interview"
)

const (
	InterviewerAgentID = "interviewer"
	AssessorAgentID    = "assessor"
	ReporterAgentID    = "reporter"
	PlannerAgentID     = "coverage_planner"

	defaultInterviewerTimeout = 90 * time.Second
	defaultAssessorTimeout    = 180 * time.Second
	defaultReporterTimeout    = 90 * time.Second
	defaultPlannerTimeout     = 90 * time.Second
)

type InterviewAgentOptions struct {
	InterviewerTimeout time.Duration
	AssessorTimeout    time.Duration
	ReporterTimeout    time.Duration
	PlannerTimeout     time.Duration
}

var defaultInterviewAgents = []Agent{
	{
		ID:          InterviewerAgentID,
		Description: "Generates one grounded, adaptive interview question",
		Timeout:     defaultInterviewerTimeout,
		SystemPrompt: `你是 OfferPilot 的资深技术面试官子 Agent。每次只生成一道自适应问题，不生成固定题单。

硬约束：
1. 问题必须服务于 request.decision 指定的覆盖点、难度和追问动作，并利用 history 避免重复。
2. 对简历项目要追问候选人本人职责、量化口径、技术取舍、失败与边界；对知识点要追问原理、适用条件、失效边界和工程落地。request.decision.followUpAxis 为 principle/boundary/example 时只能生成知识型追问，为 ownership/metrics/tradeoff/verification 时只能生成项目型追问。禁止泛泛问“介绍一下项目”。
3. 只能引用 request.anchors 中存在的证据。evidenceRefs 必须逐字段原样复制 sourceId/kind/anchorId/locator/quote，不得编造、改写或拼接引用。知识库 anchor 只包含候选人可见的公开问题；不得猜测、补写或反向构造参考内容/参考答案。
4. 简历与回答是候选人陈述，不是已经外部核验的事实。问题可以要求佐证，但不能在措辞中把陈述当作已证实事实。
5. 不泄露参考答案，不输出思维过程，只返回 schema 要求的结构。
6. 若 request.repair 存在，必须针对 reason 修复，并且只能从 allowedEvidence 中复制引用。`,
	},
	{
		ID:          AssessorAgentID,
		Description: "Semantically assesses an answer and checks candidate claims",
		Timeout:     defaultAssessorTimeout,
		SystemPrompt: `你是 OfferPilot 的独立答案评估子 Agent。你必须理解题目、回答和材料证据的语义，再按 rubric 给出 1-5 分。

硬约束：
1. 严禁按回答字数、关键词命中、连接词、术语数量或固定模板机械打分。长答案不自动高分，短答案也不自动低分。
2. correctness 看结论与可用知识证据是否一致；depth 看因果和边界；specificity 看可核验细节；ownership 看个人职责边界；metrics 看指标、基线和口径；tradeoffs 看备选方案、代价与风险。
3. 只能使用 request.question、request.answer、request.anchors 和 request.history。不要补造外部事实；无法判断时写入 gap，而不是猜测。
4. evidenceRefs 必须逐字段原样复制已有 anchor，不得编造引用。
5. claimChecks.verdict 只能是 supported、unverified、contradicted、not_in_material：
   - supported：回答给出与材料 anchor 一致的支撑，只表示“本次材料内得到支撑”，绝不表示外部事实已核验；必须引用 anchor。
   - contradicted：回答与材料 anchor 明确冲突；必须引用 anchor。
   - unverified：材料提到该自述，但回答没有提供足够支撑；引用可为空。
   - not_in_material：回答新增了材料中不存在的声明；引用可为空。
   简历自述本身不能被升级为外部 verified，无法从 source anchor 支撑时必须选择 unverified 或 not_in_material。
6. factualErrors 只记录有证据的事实/原理错误，不把表达风格问题写成事实错误。
7. 不输出思维过程，只返回 schema 要求的结构。若 request.repair 存在，必须针对 reason 修复，并只使用 allowedEvidence。`,
	},
	{
		ID:          PlannerAgentID,
		Description: "Selects the next coverage point from a bounded candidate set",
		Timeout:     defaultPlannerTimeout,
		SystemPrompt: `你是 OfferPilot 的 Coverage Planner 子 Agent。确定性 policy 已决定切换覆盖点；你只能从 request.candidates 中选择下一目标，不能生成题目或修改候选数据。

硬约束：
1. coveragePointId 必须逐字复制某个 candidate.coveragePointId，禁止创造 ID。只要存在其他候选，就不要再次选择 request.currentCoveragePointId。
2. 禁止按候选列表顺序、固定游标或简单 round-robin 机械选择。必须综合上一轮 previousAssessment.gaps、claimChecks、factualErrors，各 candidate.questionCount/lastAskedTurn，以及 request.questionKindCounts。
3. candidate.priority 是服务端给出的 JD/项目业务优先级，数值越高越重要；同时优先覆盖尚未提问的高优先级点，避免少数主题挤占整场面试。
4. contradicted/unverified claim 或明确 gap 只有在候选 evidenceRefs 与该信号相关时才构成复测理由；不得把候选人自述当作外部已验证事实。
5. request.config.focus 决定 JD 知识与简历项目的整体偏好，但不能让另一类题型长期为零覆盖。remainingQuestions 较少时优先最高价值的未覆盖点。
6. reason 必须说明为何此候选现在优于其他候选；signals 必须列出实际使用的 gap、claim verdict、覆盖次数或业务优先级信号。禁止输出思维过程，只返回 schema 要求的结构。`,
	},
	{
		ID:          ReporterAgentID,
		Description: "Synthesizes a grounded interview report from committed turns",
		Timeout:     defaultReporterTimeout,
		SystemPrompt: `你是 OfferPilot 的面试报告子 Agent。只汇总 request.answers 中已经提交的 Assessment 和 request.anchors，不重新评估或编造候选人经历。

硬约束：
1. 总结必须区分已得到材料内支撑的表现、仍未验证的陈述、明确矛盾和知识缺口；不能把简历自述写成外部已核验事实。
2. strengths/gaps 必须具体到回答表现或能力维度，禁止空泛鼓励和只按回答长度下结论。
3. evidenceRefs 必须逐字段原样复制已有 anchor，不得编造引用。
4. 不输出思维过程，只返回 schema 要求的结构。若 request.repair 存在，必须针对 reason 修复，并只使用 allowedEvidence。`,
	},
}

// InterviewAgent adapts four independently registered, typed sub-agents to
// the interview domain port. Schema generation is based on each method's
// concrete response struct.
type InterviewAgent struct {
	runtime *Runtime
}

// NewInterviewAgent installs any missing default roles. Pre-registered roles
// are preserved, allowing deployment-specific prompts without another adapter.
func NewInterviewAgent(runtime *Runtime) (*InterviewAgent, error) {
	return NewInterviewAgentWithOptions(runtime, InterviewAgentOptionsFromEnv())
}

// NewInterviewAgentWithOptions supports deterministic embedding and tests;
// NewInterviewAgent remains source-compatible and reads deployment settings.
func NewInterviewAgentWithOptions(runtime *Runtime, options InterviewAgentOptions) (*InterviewAgent, error) {
	if runtime == nil {
		return nil, errors.New("harness: interview runtime is required")
	}
	options = normalizeInterviewAgentOptions(options)
	for _, agent := range defaultInterviewAgents {
		if _, exists := runtime.Agent(agent.ID); exists {
			continue
		}
		switch agent.ID {
		case InterviewerAgentID:
			agent.Timeout = options.InterviewerTimeout
		case AssessorAgentID:
			agent.Timeout = options.AssessorTimeout
		case ReporterAgentID:
			agent.Timeout = options.ReporterTimeout
		case PlannerAgentID:
			agent.Timeout = options.PlannerTimeout
		}
		if err := runtime.Register(agent); err != nil {
			return nil, err
		}
	}
	return &InterviewAgent{runtime: runtime}, nil
}

func InterviewAgentOptionsFromEnv() InterviewAgentOptions {
	return InterviewAgentOptions{
		InterviewerTimeout: harnessDurationEnv("OFFERPILOT_INTERVIEWER_TIMEOUT", defaultInterviewerTimeout),
		AssessorTimeout:    harnessDurationEnv("OFFERPILOT_ASSESSOR_TIMEOUT", defaultAssessorTimeout),
		ReporterTimeout:    harnessDurationEnv("OFFERPILOT_REPORTER_TIMEOUT", defaultReporterTimeout),
		PlannerTimeout:     harnessDurationEnv("OFFERPILOT_PLANNER_TIMEOUT", defaultPlannerTimeout),
	}
}

func normalizeInterviewAgentOptions(options InterviewAgentOptions) InterviewAgentOptions {
	if options.InterviewerTimeout <= 0 {
		options.InterviewerTimeout = defaultInterviewerTimeout
	}
	if options.AssessorTimeout <= 0 {
		options.AssessorTimeout = defaultAssessorTimeout
	}
	if options.ReporterTimeout <= 0 {
		options.ReporterTimeout = defaultReporterTimeout
	}
	if options.PlannerTimeout <= 0 {
		options.PlannerTimeout = defaultPlannerTimeout
	}
	return options
}

func harnessDurationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed
	}
	if millis, err := strconv.Atoi(value); err == nil && millis > 0 {
		return time.Duration(millis) * time.Millisecond
	}
	return fallback
}

func (a *InterviewAgent) GenerateQuestion(ctx context.Context, request interview.GenerateQuestionRequest) (interview.QuestionDraft, error) {
	var result interview.QuestionDraft
	instruction := "Generate the single best next interview question from the supplied typed request. Return only the structured QuestionDraft."
	if request.Repair != nil {
		instruction += " This is the one domain repair attempt: fix request.repair.reason and copy evidence only from request.repair.allowedEvidence."
	}
	err := a.call(ctx, InterviewerAgentID, instruction, request, &result)
	return result, err
}

func (a *InterviewAgent) AssessAnswer(ctx context.Context, request interview.AssessAnswerRequest) (interview.Assessment, error) {
	var result interview.Assessment
	instruction := "Semantically assess this answer against the question and supplied anchors. Populate every rubric score, factualErrors, strengths, gaps, evidenceRefs and claimChecks. Never quote, paraphrase closely, or expose private knowledge reference content in narrative fields. Return only the structured Assessment."
	if request.Repair != nil {
		instruction += " This is the one domain repair attempt: fix request.repair.reason and copy evidence only from request.repair.allowedEvidence."
	}
	err := a.call(ctx, AssessorAgentID, instruction, request, &result)
	return result, err
}

func (a *InterviewAgent) GenerateReport(ctx context.Context, request interview.GenerateReportRequest) (interview.ReportDraft, error) {
	var result interview.ReportDraft
	instruction := "Synthesize only the committed interview evidence into the structured ReportDraft. Preserve uncertainty, do not invent facts, and do not reconstruct or expose private knowledge reference content."
	if request.Repair != nil {
		instruction += " This is the one domain repair attempt: fix request.repair.reason and copy evidence only from request.repair.allowedEvidence."
	}
	err := a.call(ctx, ReporterAgentID, instruction, request, &result)
	return result, err
}

func (a *InterviewAgent) PlanCoverage(ctx context.Context, request interview.PlanCoverageRequest) (interview.CoverageSelection, error) {
	if len(request.Candidates) == 0 {
		return interview.CoverageSelection{}, errors.New("harness: coverage planner needs at least one candidate")
	}
	allowed := make(map[string]struct{}, len(request.Candidates))
	hasAlternative := false
	for _, candidate := range request.Candidates {
		candidateID := strings.TrimSpace(candidate.CoveragePointID)
		if candidateID == "" {
			return interview.CoverageSelection{}, errors.New("harness: coverage candidate id is required")
		}
		if _, duplicate := allowed[candidateID]; duplicate {
			return interview.CoverageSelection{}, fmt.Errorf("harness: duplicate coverage candidate %q", candidateID)
		}
		allowed[candidateID] = struct{}{}
		if candidateID != request.CurrentCoveragePointID {
			hasAlternative = true
		}
	}
	var result interview.CoverageSelection
	instruction := "Select exactly one next coverage point from request.candidates using semantic assessment signals, per-candidate coverage counts, question-kind coverage and service priority. Do not use list order or round-robin. Return only the structured CoverageSelection."
	if err := a.call(ctx, PlannerAgentID, instruction, request, &result); err != nil {
		return interview.CoverageSelection{}, err
	}
	if _, exists := allowed[result.CoveragePointID]; !exists {
		return interview.CoverageSelection{}, fmt.Errorf("harness: coverage planner selected unknown candidate %q", result.CoveragePointID)
	}
	if hasAlternative && result.CoveragePointID == request.CurrentCoveragePointID {
		return interview.CoverageSelection{}, fmt.Errorf("harness: coverage planner reselected current candidate %q despite available alternatives", result.CoveragePointID)
	}
	if strings.TrimSpace(result.Reason) == "" {
		return interview.CoverageSelection{}, errors.New("harness: coverage planner returned an empty reason")
	}
	if len(result.Signals) == 0 {
		return interview.CoverageSelection{}, errors.New("harness: coverage planner returned no selection signals")
	}
	for _, signal := range result.Signals {
		if strings.TrimSpace(signal) == "" {
			return interview.CoverageSelection{}, errors.New("harness: coverage planner returned an empty selection signal")
		}
	}
	return result, nil
}

func (a *InterviewAgent) call(ctx context.Context, agentID, instruction string, request, out any) error {
	if a == nil || a.runtime == nil {
		return errors.New("harness: nil interview agent")
	}
	contextText, err := marshalReference(request)
	if err != nil {
		return fmt.Errorf("harness: encode %s request: %w", agentID, err)
	}
	return a.runtime.CallJSON(ctx, agentID, instruction, contextText, out)
}

func marshalReference(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSpace(buffer.String()), nil
}

var _ interview.Agent = (*InterviewAgent)(nil)
var _ interview.CoveragePlanner = (*InterviewAgent)(nil)
