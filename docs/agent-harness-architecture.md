# OfferPilot Agent Harness 与 Go 后端架构

> 状态：Implemented foundation + target evolution
> 日期：2026-08-11
> 范围：模拟面试、JD/简历摄取、知识检索、评估报告及 TypeScript 后端到 Go 后端的迁移

## 1. 结论

OfferPilot 的模拟面试不应再由“固定题单 + 正则缺陷检查”驱动。目标实现采用**确定性的 Go Harness 控制面**管理状态、权限、预算、证据、恢复和可观测性；LLM 只在受约束的角色和结构化输出内完成规划、出题、评估与总结。

一场面试必须持续回答四个问题：

1. 这道题为什么现在问？
2. 它对应哪条 JD 要求、简历声明或知识能力？
3. 候选人的回答提供了什么证据，仍缺什么证据？
4. 为什么继续追问、切换主题或结束？

这四项都要成为可持久化、可回放、可测试的数据，不能只存在于 prompt 或模型思维过程中。

架构图：

- [draw.io 源文件](./images/agent-harness-go-architecture.drawio)

### 1.1 当前落地状态

本文同时记录“已经运行的第一阶段实现”和“继续演进的目标架构”。架构图展示完整目标边界；下表是 2026-08-11 仓库代码的实际状态，不能把目标组件误读为已经上线。

| 能力 | 状态 | 当前实现 |
|---|---|---|
| Go 面试领域后端 | 已实现 | `backend/cmd/offerpilot-api`；`POST /api/interview` 提供 start / answer / report |
| Typed Harness Roles | 已实现 | Planner、Interviewer、Assessor、Reporter、Resume Matcher、Resume Diagnostician、Web Crawler；结构化 JSON、多模态输入、Function Tool 白名单、有界 Agent Loop、超时、有界并发、trace |
| 可观察执行轨迹 | 已实现 | `POST /api/interview/stream` 使用 NDJSON 实时输出安全步骤；浏览器断开不取消有界后台 run，Web 保留各轮排队、执行、完成/失败和耗时 |
| 自适应追问 | 已实现 | Assessor 语义评分驱动 prerequisite / follow-up / advance；知识与项目使用不同追问轴 |
| JD + 简历输入 | 已实现 | Web BFF 负责 PDF/DOCX/Markdown/TXT/TEX；URL 优先走 Go Provider/JSON-LD 快路径，未知动态站点进入 `web_crawler` 的受限 Function Tool Loop；Go 负责 Profile、Coverage 和 EvidenceRef |
| 知识检索 | 已实现 | 36 个 Markdown 文件解析为 404 个原子问答块；稳定 KB ID、BM25 top-K，问题与参考内容不可拆分 |
| 证据化评估 | 已实现 | claim verdict 为 supported / unverified / contradicted / not_in_material；材料外新增声明不会升级为已验证事实 |
| 持久化恢复 | 已实现 | SQLite WAL、migration、完整 session snapshot、乐观版本 CAS；`GET /api/v1/interviews/{id}` 可恢复公开 Profile、当前问题、历史轮次和进度 |
| Answer 幂等与事件账本 | Alpha 已实现 | 浏览器为同题重试复用 `clientAnswerId`；Go 按 principal / session / action / key 和 payload hash 去重，原子提交 snapshot、command result 和事件；事件 GET 只返回安全元数据 |
| 故障语义 | 已实现 | Agent 未配置、调用失败或 repair 后仍无效时返回可重试 503，不提交机械兜底评分；live 与 ready 分离 |
| 语音 | 已实现 | MiMo ASR/TTS；重置、卸载、完成和报告切换都会释放麦克风与 AudioContext |
| 持久 SSE replay 与 worker 接管 | 待演进 | command / event / invocation / checkpoint / lease / outbox 表结构已落地；`202 + runId`、`Last-Event-ID`、stale run 接管和 outbox dispatcher 尚未接入主执行链 |
| 独立 Go 文档解析服务 | 待演进 | 当前上传解析仍在 Next.js BFF；Go 已是面试领域和 Harness 的唯一主实现 |

### 1.2 运行时不变量

- 评分只来自 Assessor 的结构化语义输出；Go 不按答案长度、关键词、连接词或正则重新打分。
- 模型故障与候选人能力严格分离。没有有效 Assessment 就不提交 AnswerRecord，也不生成报告分数。
- 每个知识题 EvidenceRef 同时包含问题和对应参考内容；Assessor 不会只看到问题而丢失参考答案。
- 知识参考内容只进入 Assessor 私有上下文；候选人可见的题目、topic、反馈引用和执行轨迹只返回问题侧公开文本。
- Interviewer、Planner 和 Reporter 只接收动态公开投影；旧持久化会话和缓存报告也在模型调用与 HTTP 输出边界再次脱敏，自由文本复述参考内容时只允许一次 repair，仍不合格则失败关闭。
- `report_only` 不返回逐题 Assessment；混合面试总分先按各题适用 rubric 归一化为百分制，再按轮次等权聚合，避免项目题因字段更多而天然权重更高。
- 可观察执行轨迹不是模型私有思维链。轨迹只包含固定步骤、状态、Agent ID、聚合计数、耗时和安全决策摘要。
- 未覆盖能力显示为“未评估”，不显示成 0 分；readiness 同时受得分、JD/项目覆盖和明确矛盾约束。
- `ready` 至少需要 4 轮有效评估；同时有 JD 和简历时，知识与项目各至少覆盖 2 轮，JD 要达到 covered，项目要形成至少 2 轮深挖，避免少量高分样本造成虚假就绪。
- 简历和面试回答都属于候选人陈述；`supported` 仅表示本次材料内一致，不代表外部事实核验。

## 2. 迁移前基线审计与断层

本节记录本次改造开始前的 TypeScript 基线，用于解释为什么需要 Go Harness。这里的 `Map` 会话、固定题单和正则诊断已经不是当前主链路；当前实现以第 1.1 节和代码为准。

### 2.1 面试链路与 Agent Harness 脱节

- `web/src/app/api/interview/route.ts` 在 Next.js 进程内维护 `Map` 会话，重启即丢失。
- 默认面试只有 5 道静态题；`answer -> next` 是固定下标推进，不依据回答调整。
- 回答诊断以长度、连接词、口头禅等正则为主，不能判断知识正确性、项目真实性、技术取舍和回答之间的矛盾。
- Web 面试 API 没有调用后端 `AgentLoop`，也没有使用知识检索、Sub-Agent、Memory 或 Permission Gate。
- `mock_interview`、`generate_followup`、`diagnose_answer` 等工具彼此独立，没有共享的面试状态、覆盖计划或证据账本。
- 通用 Session、Realtime Session 和 Web Interview Session 是三套不同状态模型，无法统一恢复与审计。

### 2.2 JD/简历能力没有接入面试

- 仓库已有 PDF/DOCX/TXT/Markdown 文本提取、JD 分析和简历匹配入口，但面试启动页不能同时上传 JD 与简历。
- 解析结果没有形成带版本的 Candidate Profile 和 Job Profile。
- 简历项目中的“主导、优化、提升、支持高并发”等声明没有转成可逐项验证的 Claim Ledger。
- 出题器不知道当前问题是在验证岗位要求、简历项目、通用知识还是行为能力。

### 2.3 知识源与运行库不一致

按 `knowledge/README.md` 的分类清单，知识文件曾以“约 385 题”作为人工内容基线；本次 Go 解析器实际扫描 36 个 Markdown 文件并得到 404 个独立 `Q` 题块。审计时旧 `data/agent.db` 的 `knowledge` 表只有 29 条，而且没有可用的结构化 `question` 行。这不是一个可以靠写死题量掩盖的问题。

目标系统必须动态观测并暴露：

- Markdown 文件数；
- 成功解析的问题数；
- 解析失败数与文件名；
- 数据库题目数；
- FTS/向量索引数；
- 内容清单 hash、索引版本和最后同步时间；
- 文件、数据库、索引之间的差额及 readiness 状态。

“约 385 题”和本次观测到的 404 题都不能成为代码里的固定常量。运行时真值必须来自扫描、解析和数据库对账。

### 2.4 数据语义不统一

- SQLite 默认时间使用 Unix 秒，Web/Node 多处使用 `Date.now()` 的 Unix 毫秒。
- 会话、消息、面试轮次和审计日志之间没有统一事件序号或状态版本。
- 知识导入使用随机 UUID，同一文件重复构建时难以稳定 upsert 和识别删除项。
- 模型调用、工具调用和状态迁移没有共同的幂等键，故障后可能重复出题或重复记分。

## 3. 目标与非目标

### 3.1 目标

- 支持同时上传或粘贴 JD 与简历，并生成可修订、可追溯的 Profile。
- 同一场面试混合覆盖岗位知识、简历项目深挖、系统设计、行为问题和薄弱项复测。
- 根据上一轮回答动态追问，而不是预先机械生成完整题单。
- 每个问题和评分都引用 `EvidenceRef`，明确来源与定位。
- 通过 Claim Ledger 跟踪“已声明、已佐证、存在矛盾、仍未验证”。
- 会话可暂停、恢复、重放；进程崩溃或模型超时不能破坏状态。
- Go 后端成为领域逻辑和 Agent Harness 的唯一实现，Next.js 保留展示与 BFF/代理职责。
- 模型、检索和工具均可替换，并具有预算、权限、超时和审计边界。
- 面试质量通过离线 eval、在线指标和人工复核持续回归。

### 3.2 非目标

- 不让 LLM 自主做录用、淘汰或薪酬决定。
- 不把模型的“置信度”当作候选人陈述真实性证明。
- 第一阶段不引入分布式工作流引擎、独立向量数据库或复杂微服务拆分。
- 第一阶段不要求实时双向流式语音；已有 ASR/TTS 可作为独立适配器继续使用。
- 不在 Go 重写时逐文件照搬 TypeScript 代码和既有缺陷。
- 不允许 Agent 直接执行 SQL、读写任意文件、访问任意 URL 或改变权限策略。

## 4. 系统边界

### 4.1 边界内

- 文档上传、格式验证、文本提取和 Profile 构建；
- 知识文件解析、索引、检索和运行时对账；
- 面试会话、问题、回答、评估、覆盖度和报告；
- Harness 的模型调度、工具执行、策略检查、上下文组装、checkpoint 和事件流；
- SQLite 持久化、结构化日志、指标和 trace；
- OpenAI-compatible/Anthropic 等模型适配，ASR/TTS 适配。

### 4.2 边界外

- 浏览器负责文件选择、录音采集、播放和界面状态展示；
- Next.js 负责静态页面、同源 BFF 和对 Go API/SSE 的转发，不承载面试领域状态；
- 外部模型服务负责生成和推理，但不拥有会话真值；
- 文件内容、JD、简历、网页抓取结果和知识文件全部视为不可信输入。

### 4.3 运行拓扑

初期保持单体部署：

```text
Browser -> Next.js Web/BFF -> Go API/Harness -> SQLite
                                  |           -> knowledge files/index
                                  |           -> temporary document storage
                                  +-----------> LLM / Embedding / ASR / TTS
```

Go 服务可以单进程运行，但包边界按领域与适配器拆分。只有当容量和部署数据证明有必要时，再拆文档解析、检索或模型网关服务。

## 5. Go 包分层

建议目录如下：

```text
backend/
├── cmd/offerpilot-api/             # 组装依赖、启动 HTTP、优雅退出
├── internal/
│   ├── domain/
│   │   ├── interview/              # Session、Turn、Question、状态机、覆盖策略
│   │   ├── profile/                # JobProfile、CandidateProfile、Claim Ledger
│   │   ├── evidence/               # EvidenceRef、定位、引用约束
│   │   ├── assessment/             # Rubric、Assessment、聚合规则
│   │   └── policy/                 # 面试与权限策略值对象
│   ├── application/
│   │   ├── ingestion/              # 上传、解析、Profile 构建用例
│   │   ├── interviews/             # 开始、回答、暂停、恢复、报告用例
│   │   ├── knowledge/              # 同步、检索、对账用例
│   │   └── reporting/              # 报告投影
│   ├── harness/
│   │   ├── orchestrator/           # 单步运行、状态迁移、角色调度
│   │   ├── roles/                  # Planner/Interviewer/Assessor/Reporter
│   │   ├── tools/                  # Registry、schema 校验、Executor
│   │   ├── guard/                  # 权限、预算、速率、prompt 注入边界
│   │   ├── context/                # 上下文选择、压缩、引用装配
│   │   ├── checkpoint/             # 恢复点、lease、重放
│   │   └── events/                 # 领域事件与 SSE 投影
│   ├── ports/
│   │   ├── model.go                # Chat/Structured Output/Embedding 接口
│   │   ├── repositories.go         # 领域仓储接口
│   │   ├── retrieval.go            # 知识检索接口
│   │   ├── documents.go            # 文档解析接口
│   │   ├── speech.go               # ASR/TTS 接口
│   │   └── clock.go                # 可测试时钟和 ID 生成器
│   ├── adapters/
│   │   ├── http/                   # REST、SSE、middleware、OpenAPI
│   │   ├── sqlite/                 # 仓储、事务、migration
│   │   ├── llm/                    # Provider adapters
│   │   ├── retrieval/              # FTS5、embedding、rerank
│   │   ├── documents/              # PDF、DOCX、TXT、Markdown
│   │   └── speech/                 # ASR/TTS provider
│   └── observability/              # log、metric、trace、redaction
├── migrations/
├── prompts/                        # 带版本的模板与 JSON Schema
├── evals/                          # 固定样本、rubric、阈值
└── openapi/
```

依赖方向必须保持：

```text
adapters -> application -> domain
                 |
                 +-------> harness -> domain + ports
ports <- adapters
```

`domain` 不依赖 HTTP、SQLite、具体模型 SDK 或 prompt 文本。`application` 负责事务和用例编排；`harness` 负责模型参与的受控决策；`adapters` 只实现端口。

## 6. Harness 组件与职责

Harness 不是“再套一层 prompt”，也不是让多个 Agent 自由聊天。它是模型外部的确定性控制平面。

| 组件 | 职责 | 必须持久化/观测 |
|---|---|---|
| Run Coordinator | 取得 session lease，执行一个可恢复步骤，提交状态迁移 | run ID、起止时间、结果、错误类 |
| State Machine | 校验事件能否作用于当前状态，执行 CAS 更新 | state、state version、transition reason |
| Role Router | 按状态调用 Planner、Interviewer、Assessor 或 Reporter | role、model、prompt version |
| Model Gateway | Provider 路由、超时、重试、限流、熔断和用量统计 | invocation ID、token、latency、finish reason |
| Structured Output Validator | 用 JSON Schema 校验并做有限修复，禁止静默猜测缺失字段 | schema version、validation errors、repair count |
| Tool Registry/Executor | 工具白名单、参数校验、执行、结果大小限制 | tool call/result ID、duration、decision |
| Policy/Permission Guard | 角色权限、用户确认、数据范围、预算和风险判断 | policy version、allow/deny reason |
| Context Assembler | 按预算选择 Profile、Claim、最近轮次和知识证据 | context manifest、选中/淘汰项、token estimate |
| Memory Manager | 写入确定事实、候选人偏好、薄弱项和摘要 | memory provenance、版本、失效关系 |
| Checkpoint/Replay | 在外部调用前后保存恢复点，重启后继续 | checkpoint、lease owner、attempt |
| Event Publisher | 事务性写入事件并投影为 SSE | session sequence、event type、delivery state |
| Observability | 统一日志、指标、trace 和隐私脱敏 | trace ID、request ID、session hash |

关键规则：

1. 模型返回的是**提案**，不是已执行事实。
2. 所有提案先通过 schema、领域约束和权限策略，再写入状态。
3. 一次 Harness step 最多产生一个可见问题或一个终态，不允许重试时重复出题。
4. 模型自由文本不得直接修改 Session、Claim 或 Assessment。
5. 每次决策都记录使用的输入引用、策略版本、prompt 版本和模型版本。

## 7. Adaptive Interview 状态机

### 7.1 状态

```text
draft
  -> profiling
  -> ready
  -> planning
  -> asking
  -> awaiting_answer
  -> assessing
  -> deciding
       -> asking
       -> paused
       -> completing
  -> completed

任意运行态 -> failed_recoverable -> 原状态
任意非终态 -> cancelled
```

| 当前状态 | 接受事件 | 下一状态 | 约束 |
|---|---|---|---|
| `draft` | `ProfileSubmitted` | `profiling` | JD/简历可为文本或文件，至少有一个面试目标 |
| `profiling` | `ProfileBuilt` | `ready` | 每个抽取项必须带 EvidenceRef |
| `ready` | `InterviewStarted` | `planning` | 固化 profile revision 与 policy version |
| `planning` | `PlanAccepted` | `asking` | 生成覆盖目标，不预生成整场固定题单 |
| `asking` | `QuestionCommitted` | `awaiting_answer` | 每题有 intent、target 和 generation reason |
| `awaiting_answer` | `AnswerAccepted` | `assessing` | question ID 唯一且答案幂等 |
| `assessing` | `AssessmentCommitted` | `deciding` | 评分项必须引用回答或知识证据 |
| `deciding` | `FollowUpSelected` | `asking` | 未验证高价值目标优先，受追问上限约束 |
| `deciding` | `PauseRequested` | `paused` | checkpoint 已提交 |
| `paused` | `ResumeRequested` | `deciding` | 恢复同一 profile/policy revision |
| `deciding` | `CompletionSelected` | `completing` | 达到预算、覆盖或用户结束条件 |
| `completing` | `ReportCommitted` | `completed` | 报告来自已提交 Assessment，不重新编造事实 |

状态更新使用 `expected_state_version` 做 compare-and-swap。非法转换返回 `409 state_conflict`，不能靠 handler 内的 if/else 静默修正。

### 7.2 每轮自适应流程

1. **Observe**：读取固定版本的 Profile、Claim Ledger、Coverage Matrix、最近轮次和剩余预算。
2. **Retrieve**：围绕待验证目标检索知识条目，不按整份简历做无差别 top-k。
3. **Plan**：选择本轮意图和目标，给出机器可读 `selection_reason`。
4. **Generate**：Interviewer 生成一个主问题；Harness 检查重复、泄题、长度和引用。
5. **Assess**：回答提交后，Assessor 按题目 rubric 输出证据、缺口、矛盾和分项评分。
6. **Decide**：确定追问、切换目标、复测或结束，并提交 checkpoint。

下一目标可以按以下可解释特征排序，权重放在版本化 Policy 中：

```text
priority = role_importance
         + evidence_gap
         + claim_risk
         + uncertainty
         + planned_coverage_debt
         + weakness_recheck_bonus
         - repetition_penalty
         - fatigue_penalty
```

模型可以建议各特征，最终归一化、边界检查和排序由 Go 代码完成。系统至少限制：总时长、总题数、单目标追问数、连续同维度题数、模型 token 与费用预算。

### 7.3 问题类型

- `resume_project_deep_dive`：职责、规模、架构、关键决策、故障、量化结果；
- `knowledge_probe`：概念、原理、边界、失败模式；
- `system_design`：需求澄清、容量、数据模型、取舍、演进；
- `behavioral`：STAR 证据、冲突、复盘、影响；
- `follow_up`：只针对上一回答中的具体陈述或缺口；
- `counterfactual`：改变约束，验证是否真正理解取舍；
- `debugging`：给出症状与有限证据，观察定位路径；
- `weakness_recheck`：稍后换一种表达复测薄弱点。

“拷打”应体现为深度和证据要求，而不是攻击性措辞。Policy 对问题做职业相关性、歧视风险、隐私越界和重复度检查。

## 8. Profile 与 Claim Ledger

### 8.1 Profile 是版本化事实视图

`InterviewProfile` 绑定：

- `JobProfile`：岗位名称、职级信号、必选/加分要求、职责、技术主题、业务约束；
- `CandidateProfile`：经历、项目、技能、教育、时间线、量化指标、个人职责；
- `DocumentRevision`：原始文档 hash、解析器版本、文本版本和来源；
- `ProfileRevision`：抽取模型、prompt/schema 版本、人工修订记录。

Profile 中每个非推导字段都必须引用原文。无法定位来源的内容只能标记为 `inferred`，不能伪装成简历或 JD 原文。

### 8.2 Claim Ledger

Claim 是待验证的原子陈述，例如：“候选人在项目 A 中主导了 RAG 检索重构，并将 P95 延迟降低 40%”。

建议字段：

```go
type Claim struct {
    ID              string
    ProfileID       string
    Subject         string
    Predicate       string
    Object          string
    ClaimType       string // ownership, scale, metric, skill, decision, outcome
    Importance      int    // 1..5，来自岗位相关性与声明强度
    Status          string // asserted, supported, contradicted, unverified, retracted
    SourceEvidence  []EvidenceRef
    SupportEvidence []EvidenceRef
    Conflicts       []EvidenceRef
    Revision        int64
}
```

状态含义：

- `asserted`：JD、简历或候选人做出了声明；
- `supported`：回答提供了具体、内部一致的支持证据；不等于外部事实已核验；
- `contradicted`：同一会话中的陈述存在可定位矛盾；
- `unverified`：重要但尚未得到足够信息；
- `retracted`：候选人明确修正了此前陈述。

Claim 更新采用“追加证据 + 新 revision”，不覆盖原始记录。Assessment 只能提出状态变更，领域服务检查引用完整性后提交。

### 8.3 项目深挖覆盖面

每个高重要性项目至少考虑以下轴，但不要求机械地全部发问：

- 个人职责与团队边界；
- 原始问题、约束和成功指标；
- 关键架构与数据流；
- 方案选择及被放弃方案；
- 规模、容量和性能数据；
- 故障、排查与恢复；
- 测试、评估和上线验证；
- 结果归因与复盘。

Coverage Matrix 记录每个 `requirement_id / claim_id / competency` 的计划权重、已问次数、证据强度、最近评分与是否需要复测。

## 9. 核心领域契约

### 9.1 EvidenceRef

```go
type EvidenceRef struct {
    ID          string
    Kind        string // jd, resume, answer, knowledge, tool_result, human_note
    SourceID    string
    Revision    int64
    Locator     Locator // page/section/start/end/turn_id
    Quote       string  // 最小必要原文，设长度上限
    ContentHash string
    CreatedAt   time.Time
}
```

约束：

- `Quote + Locator + ContentHash` 必须能重新定位到指定 revision；
- 对 JD/简历的解释引用原文，对知识正确性的判断引用知识条目；
- `answer` 引用必须包含 turn ID；
- 来源删除后保留 hash 和最小审计信息，按隐私策略处理 quote；
- 模型输出未知 EvidenceRef ID 时整次输出无效，不能创建“幽灵引用”。

### 9.2 Question

```go
type Question struct {
    ID                 string
    SessionID          string
    TurnNo             int
    Type               string
    Intent             string
    Text               string
    TargetRequirementIDs []string
    TargetClaimIDs     []string
    Competencies       []string
    RetrievedEvidence  []EvidenceRef
    ExpectedSignals    []string
    FollowUpOf         *string
    SelectionReason    string
    PromptVersion      string
    ModelInvocationID  string
    CreatedAt          time.Time
}
```

问题提交前校验：目标非空、文本只含一个主问题、未泄露参考答案、与近期问题不过度重复、引用存在、符合 Policy。

### 9.3 Assessment

```go
type Assessment struct {
    ID             string
    SessionID      string
    QuestionID     string
    AnswerID       string
    RubricVersion  string
    Scores         map[string]int // 1..5: correctness, depth, specificity, ownership, metrics, tradeoffs
    Observations   []Observation
    MissingSignals []string
    Contradictions []Contradiction
    ClaimProposals []ClaimUpdateProposal
    Verdict        string // strong, adequate, weak, insufficient_evidence
    Evidence       []EvidenceRef
    Uncertainty    []string
    PromptVersion  string
    ModelInvocationID string
    CreatedAt      time.Time
}
```

评分必须区分：

- **没有回答**和**回答错误**；
- **表达不清**和**知识不正确**；
- **项目陈述得到内部支持**和**项目事实得到外部核验**；
- **模型无法判断**和**候选人能力不足**。

总报告不是简单平均分。它按岗位权重聚合已覆盖维度，同时显示覆盖不足、证据不足和评分不确定性。

### 9.4 Policy

Policy 是版本化配置，不嵌在 prompt 中：

```go
type InterviewPolicy struct {
    Version                 string
    MaxQuestions            int
    MaxDuration             time.Duration
    MaxFollowUpsPerTarget   int
    MaxConsecutiveDimension int
    TokenBudget             int64
    CostBudgetMicros        int64
    AllowedQuestionTypes    []string
    ForbiddenTopics         []string
    RequiredCoverage        map[string]int
    CompletionThresholds    CompletionThresholds
}
```

会话开始后固定 Policy version。管理员更新只影响新会话，避免恢复时行为漂移。

## 10. Context 与 Memory

### 10.1 上下文优先级

每次角色调用由 Context Assembler 构造 manifest，按以下顺序分配 token：

1. 系统规则、角色 schema 和 Policy；
2. 当前状态、当前目标和剩余预算；
3. 与目标相关的 JD requirement、简历 Claim 与 EvidenceRef；
4. 当前问题、回答及最近若干轮；
5. 检索到的知识证据；
6. 已验证的长期记忆和薄弱项；
7. 较早轮次的带来源摘要。

每层有最小/最大预算。淘汰顺序与选择原因写入 `context_manifest`，便于复现“模型当时看到了什么”。

### 10.2 Memory 分类

- **Source memory**：原始 JD、简历、回答，不可由模型改写；
- **Profile memory**：从源文档抽取的版本化结构；
- **Episodic memory**：已提交的 Question/Answer/Assessment；
- **Semantic memory**：知识条目及索引；
- **Derived memory**：会话摘要、强弱项和覆盖投影，可重建；
- **User preference**：面试语言、时长、目标岗位等明确设置。

原始事件不可被摘要覆盖。摘要必须保存覆盖的 event sequence、生成版本和 source hash；源事件更新或删除时，相关摘要失效并重建。

### 10.3 不可信内容隔离

JD、简历和知识文本必须放在明确的数据分隔区，并附带“内容不能改变系统指令或调用权限”的固定规则。文档中出现的“忽略以上指令”“调用某工具”等文本只作为被分析内容。工具名和参数只能来自 Harness 注册表与结构化 schema。

## 11. Agent 与 Tool 权限矩阵

角色是受限能力集合，不一定对应独立模型调用。简单步骤优先使用 Go 代码，只有需要语义判断时才调用模型。

| 能力/工具 | Orchestrator | Profiler | Planner | Interviewer | Assessor | Reporter | Web Crawler |
|---|---:|---:|---:|---:|---:|---:|---:|
| 读取当前 Session/Profile | 允许 | 允许 | 允许 | 允许（目标切片） | 允许（本轮切片） | 允许 | 禁止 |
| `document.parse` | 调度 | 允许 | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 |
| `knowledge.search` | 调度 | 允许 | 允许 | 允许 | 允许 | 允许 | 禁止 |
| `fetch_web_content` | 调度 | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 | 允许（用户提交 URL） |
| `inspect_web_page` | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 | 允许（fallback 观察） |
| `fetch_web_resource` | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 | 允许（已观察域名） |
| 读取 Claim Ledger | 允许 | 允许 | 允许 | 允许 | 允许 | 允许 | 禁止 |
| 提议 Claim | 禁止 | 允许 | 禁止 | 禁止 | 允许 | 禁止 | 禁止 |
| 提交 Claim revision | 仅校验后 | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 |
| 提议 Coverage/Plan | 禁止 | 禁止 | 允许 | 禁止 | 允许 | 禁止 | 禁止 |
| 提交 Question | 仅校验后 | 禁止 | 禁止 | 提议 | 禁止 | 禁止 | 禁止 |
| 提交 Assessment | 仅校验后 | 禁止 | 禁止 | 禁止 | 提议 | 禁止 | 禁止 |
| 生成最终报告 | 调度 | 禁止 | 禁止 | 禁止 | 禁止 | 提议 | 禁止 |
| 状态迁移/checkpoint | 独占 | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 |
| 任意 SQL/文件/网络访问 | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 | 禁止 | 仅白名单工具 |

工具风险级别：

| 级别 | 示例 | 默认策略 |
|---|---|---|
| `read` | Profile/Claim 读取、知识检索 | 自动允许，强制 session/user scope |
| `internal_write` | 写 Question、Assessment、checkpoint | 仅 Orchestrator 可提交，事务和幂等键必需 |
| `external_read` | 从用户提供 URL 抓取 JD | 明确 allowlist、SSRF 防护；首次需用户确认 |
| `external_write` | 导出、分享、发送报告 | 默认禁止，必须逐次确认 |
| `destructive` | 删除 Profile/会话/原文 | Agent 永不允许，只能由显式用户 API 发起 |

模型不能看到数据库凭据、API key、绝对文件路径或内部错误堆栈。工具结果设大小、类型和敏感字段过滤上限。

## 12. HTTP 与事件 API 契约

所有新接口放在 `/api/v1`，由 OpenAPI 生成或校验 DTO。错误统一为：

```json
{
  "error": {
    "code": "state_conflict",
    "message": "session is not awaiting an answer",
    "request_id": "req_...",
    "retryable": false,
    "details": {}
  }
}
```

### 12.1 上传 JD 与简历

`POST /api/v1/interview-profiles` 使用 `multipart/form-data`：

- `jd_file` 或 `jd_text` 二选一；
- `resume_file` 或 `resume_text` 二选一；
- `target_role`、`language` 为可选字段；
- Header：`Idempotency-Key`；
- 支持 PDF、DOCX、TXT、Markdown；按 magic bytes 验证，不信任扩展名。

响应可同步返回 `201`，较慢解析返回 `202`：

```json
{
  "profile_id": "prof_...",
  "revision": 1,
  "status": "processing",
  "documents": [
    {"document_id": "doc_...", "kind": "jd", "status": "parsed"},
    {"document_id": "doc_...", "kind": "resume", "status": "processing"}
  ],
  "created_at": "2026-08-10T14:55:12.381Z"
}
```

配套接口：

- `GET /api/v1/interview-profiles/{profile_id}`：返回解析状态、Job/Candidate 摘要、Claim 数和 warning；
- `PATCH /api/v1/interview-profiles/{profile_id}`：人工修订，要求 `expected_revision`；
- `DELETE /api/v1/interview-profiles/{profile_id}`：显式删除原文及派生数据，生成审计记录。

### 12.2 创建面试

`POST /api/v1/interview-sessions`：

```json
{
  "profile_id": "prof_...",
  "profile_revision": 1,
  "mode": "adaptive",
  "policy_id": "backend-senior-45m",
  "focus": ["resume_project", "knowledge", "system_design"],
  "language": "zh-CN"
}
```

返回 `session_id`、`state`、`state_version`、首个事件游标和固定的 profile/policy revision。Profile 尚未 ready 时返回 `409 profile_not_ready`，不自动退化成无 JD/简历的静态题单。

### 12.3 获取问题与提交回答

- `GET /api/v1/interview-sessions/{id}`：当前状态、进度、活动 question、覆盖摘要；
- `POST /api/v1/interview-sessions/{id}/answers`：提交文字或 ASR transcript；
- `POST /api/v1/interview-sessions/{id}:pause`；
- `POST /api/v1/interview-sessions/{id}:resume`；
- `POST /api/v1/interview-sessions/{id}:complete`；
- `POST /api/v1/interview-sessions/{id}:cancel`。

回答请求：

```json
{
  "question_id": "q_...",
  "client_answer_id": "device-uuid-...",
  "expected_state_version": 7,
  "content": {"type": "text", "text": "..."},
  "client_metrics": {"recording_duration_ms": 48320}
}
```

服务端返回 `202` 和 committed answer/event ID。相同 `client_answer_id + question_id` 重试返回同一结果；同一 question 的不同答案 ID 返回 `409 answer_already_committed`，修正必须走显式 amendment 事件。

### 12.4 SSE

`GET /api/v1/interview-sessions/{id}/events`：

- 支持 `Last-Event-ID`；
- `id` 使用会话内单调递增 `sequence`；
- 事件包括 `profile.ready`、`plan.updated`、`question.created`、`answer.accepted`、`assessment.completed`、`session.paused`、`report.ready`、`run.failed`；
- 心跳是注释帧，不改变 sequence；
- SSE 断开只影响投递，不取消已提交的 Harness run；显式 cancel API 才改变领域状态。

### 12.5 当前 NDJSON 进度流

第一阶段已实现 `POST /api/interview/stream`。服务端逐行返回 `{type:"trace", trace:{...}}`，最后返回 `{type:"result", status, data}`；Next.js BFF 透传响应体并禁用代理缓冲。Web 按稳定事件 ID 聚合同一步骤，同时保留 queued、running、completed/failed 的状态转换和全部历史轮次。请求体读取完成后，Go 使用独立的五分钟有界 context 执行命令；浏览器断开只结束投递，不取消已经开始的 Harness run。

该流是同步命令的可观察进度，不是第 12.4 节目标中的持久化事件订阅：断线后当前客户端不能 replay 已错过的实时 trace，但使用同一 `clientAnswerId` 重试可以取得已经原子提交的结果。服务端不会把 Prompt、原始模型输出、JD/简历正文、候选人答案或知识参考内容写入 trace。持久 SSE、心跳和 `Last-Event-ID` 恢复仍属于下一阶段。

### 12.6 报告与运维

- `GET /api/v1/interview-sessions/{id}/report`：分项结论、证据引用、覆盖缺口、建议复习路径；
- `GET /health/live`：进程存活；
- `GET /health/ready`：数据库、migration、知识对账和必要 provider 状态；
- `GET /metrics`：受保护的 Prometheus 指标；
- `GET /api/v1/knowledge/status`：文件/解析/数据库/索引动态计数和版本。

## 13. 数据、幂等与恢复

### 13.1 主要表

建议至少包含：

- `documents`、`document_revisions`、`document_spans`；
- `interview_profiles`、`job_requirements`、`candidate_projects`；
- `claims`、`claim_revisions`、`evidence_refs`；
- `interview_sessions`、`interview_plans`、`coverage_items`；
- `questions`、`answers`、`assessments`；
- `session_events`、`checkpoints`、`run_leases`；
- `model_invocations`、`tool_invocations`；
- `idempotency_keys`、`outbox_events`；
- `knowledge_sources`、`knowledge_entries`、`knowledge_index_status`；
- `audit_log`。

所有领域表包含 `user_id/tenant_id`（即使当前为单用户，也保留隔离边界）、revision、创建/更新时间和软删除语义。外键开启，SQLite 使用 WAL 和 busy timeout。

### 13.2 稳定 ID 与知识同步

知识 entry ID 使用规范化相对路径、题目定位和内容 hash 派生，不再每次构建随机 UUID。同步流程：

1. 扫描 Markdown 并生成 source manifest；
2. 解析每个文件，记录成功、warning 和失败；
3. 在临时批次写入/更新稳定 ID；
4. 构建 FTS/embedding 索引；
5. 校验文件数、问题数、DB 数和索引数；
6. 单事务切换 active generation；
7. readiness 和指标暴露差额，不静默沿用陈旧的 29 条旧库。

解析失败不得把上一 generation 的有效数据先删除。删除源文件时，通过 manifest 确认后将对应 entry 标记 inactive，再在成功切换后清理。

### 13.3 幂等

- 所有创建或命令型 POST 接受 `Idempotency-Key`，作用域为 `user + route + key`；
- 文档用 SHA-256 去重，但同内容可被不同 Profile 引用；
- Answer 以 `session_id + question_id + client_answer_id` 唯一；
- Question 以 `session_id + turn_no` 唯一；
- Assessment 以 `answer_id + rubric_version` 唯一；
- 状态更新使用 `state_version` CAS；
- 模型调用先写 `model_invocations(status=pending, request_hash)`，结果校验后再与领域事件同事务提交；
- 工具调用包含稳定 `tool_call_id`，可重试工具必须声明幂等语义。

### 13.4 崩溃恢复

Run Coordinator 对 session 获取有期限 lease。每个外部调用前写 checkpoint，成功结果和下一状态在同一事务提交。进程重启后：

1. 回收过期 lease；
2. 找到 `pending/running` invocation；
3. 已有有效结果则继续提交；
4. 未知是否成功的只读调用可按 request hash 重试；
5. 不可幂等外部写操作进入 `manual_review`，绝不自动重复；
6. SSE 客户端用 event sequence 补齐遗漏事件。

`failed_recoverable` 保存原状态、错误类、attempt 和 `next_retry_at`。达到重试上限后暂停会话并向用户提供恢复入口，而不是生成一段兜底回答假装成功。

### 13.5 时间统一

时间规则必须一次性统一：

- Go 内部只使用 `time.Time` 的 UTC 值；入口立即 `UTC()`；
- API 中的时刻统一输出 RFC3339 UTC（例如 `2026-08-10T14:55:12.381Z`）；
- SQLite 时刻统一为 Unix **毫秒** `INTEGER`，列名显式使用 `*_at_ms`；
- 时长统一为整数毫秒，列名/字段名使用 `*_duration_ms` 或 `*_timeout_ms`；
- 排序使用服务端时间加会话内 sequence，不能依赖客户端时钟；
- 超时和耗时测量使用 Go 单调时钟；
- 测试注入 `Clock`，禁止领域代码直接散落调用 `time.Now()`；
- 旧 Unix 秒字段由一次性 migration 明确乘以 1000，迁移后不保留运行时猜测单位的逻辑。

## 14. 安全、隐私与日志脱敏

JD、简历、回答、录音和评估均可能包含个人信息或商业敏感信息。

### 14.1 输入与访问控制

- API 必须认证；所有仓储查询强制 user/tenant scope；
- 上传限制总大小、解压后大小、页数、字符数和处理时间；
- 校验 magic bytes，拒绝宏、可执行内容、路径穿越和 zip bomb；
- URL 导入仅允许 `https`，阻止 loopback、link-local、私网和 DNS rebinding；
- 文档解析在受限临时目录执行，完成后按 retention policy 删除；
- 模型和工具输入按最小必要原则选择，不默认发送完整简历；
- Profile 删除应级联清理原文、派生摘要、embedding 与缓存，并保留不含原文的合规审计记录。

### 14.2 Prompt 与工具安全

- 所有外部内容标记为 untrusted data；
- 结构化输出经 schema 和 EvidenceRef allowlist 校验；
- 工具调用只能来自当前角色白名单；
- 参数做类型、长度、枚举、scope 与路径/URL 校验；
- 模型不能自行提高预算、修改 Policy 或选择隐藏 provider；
- 错误信息对外分类，对内保留安全堆栈，不把 provider 原始响应直接返回浏览器。

### 14.3 日志脱敏

默认禁止记录：

- JD/简历原文、完整回答、prompt、模型完整输出；
- 音频字节、ASR 全文和文档解析全文；
- API key、Authorization/Cookie、上传文件名中的个人信息；
- tool input/output 的原始敏感字段。

允许记录：

- request/trace/run/invocation ID；
- 经过单向 hash 的 user/session 标识；
- 文本字符数、token 数、文件类型、题目类型、状态、耗时、错误类；
- EvidenceRef ID、内容 hash 前缀和 schema/prompt/model/policy 版本；
- 脱敏后的短错误摘要。

Redactor 在 logger sink 前统一执行，字段名和内容模式双重过滤。Metric label 不使用 user ID、session ID、文件名或题目文本，避免高基数和隐私泄露。开发环境也遵循同一默认值，只有显式、短时、受审计的 debug 开关可记录脱敏采样。

## 15. 测试与 Eval

### 15.1 确定性测试

- Domain unit：状态机合法/非法转换、Policy 上限、Claim revision、EvidenceRef 定位；
- Property test：任意事件序列不能产生两个 active question，终态不可继续答题；
- Application integration：事务、幂等重试、CAS 冲突、lease 过期、checkpoint 恢复；
- Repository contract：SQLite 实现与 fake 实现满足相同语义；
- HTTP/OpenAPI：请求/响应、错误 envelope、上传限制、SSE Last-Event-ID；
- Provider contract：超时、限流、无效 JSON、部分流、重复 tool call；
- Security：prompt injection、SSRF、zip bomb、越权 ID、日志敏感字段扫描；
- Time：fake clock 覆盖 UTC、毫秒迁移、超时和跨日排序；
- Knowledge reconciliation：临时知识目录的 parsed/DB/index 数动态一致，增删改可收敛；测试不把 385 或 404 写成永久断言。

### 15.2 面试质量 Eval

建立经过脱敏的固定 eval corpus，至少覆盖：

- 不同职级和技术方向的 JD/简历组合；
- 简历中有数字但缺少归因、夸大 ownership、时间线矛盾等 Claim；
- 正确、部分正确、流畅但错误、简短但正确、明确说“不知道”的回答；
- 应追问项目细节、应转知识题、应停止重复追问的多轮轨迹；
- 文档 prompt injection、错误知识引用、EvidenceRef 不存在；
- 中英文混合、ASR 噪声和长回答。

核心指标：

| 维度 | 指标 |
|---|---|
| Profile | requirement/claim 抽取 precision、recall、EvidenceRef 定位有效率 |
| 出题 | 岗位相关性、问题新颖度、目标覆盖率、单题清晰度、泄题率 |
| 追问 | 对上一回答具体引用率、有效深挖率、无关追问率、重复率 |
| 评估 | 正确性一致率、矛盾检测 precision/recall、证据覆盖率、校准误差 |
| 报告 | 结论可追溯率、覆盖缺口披露率、虚构事实率 |
| Harness | schema 首次通过率、恢复成功率、重复问题/评分次数 |
| 系统 | P50/P95 延迟、token/成本、错误率、SSE 重连成功率 |
| 检索 | Recall@k、MRR、无效引用率、文件/DB/索引差额 |

LLM-as-judge 只能作为一个信号。抽样由人工双盲评分，定期计算与 judge 的一致性；安全、引用有效性、状态正确性和幂等性必须由代码断言，不交给 judge。

### 15.3 发布门禁

- 状态机、幂等、恢复、安全测试 100% 通过；
- 固定 eval 的关键指标不低于已批准基线；
- 新版本无 EvidenceRef 的评分比例为 0；
- 知识 reconciliation 无未解释差额；
- shadow 流量中无重复可见问题、跨 session 数据和不可恢复状态；
- 性能、token 和成本在 Policy 阈值内。

## 16. TypeScript 到 Go 的 Strangler 迁移

迁移按能力切流，不做一次性替换。Next.js BFF 是临时路由层；每个面试 session 在创建时固定 `backend_owner=ts|go`，中途不能切换实现。

### Phase 0：冻结契约与建立基线

- 保存当前 TypeScript 行为、API 样本、旧 DB 29 条数据状态和知识目录动态清单；
- 定义 `/api/v1` OpenAPI、事件 schema、时间规范和错误码；
- 给现有链路补 request ID、session owner 和最小质量/延迟指标；
- Go 建立 health、config、migration、日志与 provider contract 骨架。

回滚：无流量进入 Go，删除部署即可，不改变旧 API。

### Phase 1：文档与 Profile 进入 Go

- 实现 JD+简历 multipart 上传、解析、EvidenceRef、Profile/Claim Ledger；
- Web 启动页改为调用新 Profile API；
- TypeScript 面试仍可读取 Go 提供的只读 Profile 摘要，或继续走旧的无 Profile 模式用于对照；
- 新表使用独立 schema version，migration 只做向前兼容的新增。

回滚：BFF 切回旧上传接口；保留 Go 数据，不执行降级 migration。

### Phase 2：知识摄取与检索进入 Go

- 用稳定 ID 重建知识 generation；
- 暴露动态 `knowledge/status`；
- 以 eval corpus 对比 TS 与 Go 的 top-k；
- Go 未 ready 时拒绝 adaptive interview，不静默退回 29 条旧库。

回滚：检索 feature flag 切回 TS；Go generation 保留并停止激活。

### Phase 3：Go Adaptive Interview 灰度

- Go 实现状态机、Planner/Interviewer/Assessor、checkpoint 和 SSE；
- 新建 session 按用户或百分比分桶选择 owner；
- 可对脱敏固定样本做 shadow 比较，但不对真实用户重复调用高成本模型；
- 报告同时校验 EvidenceRef 和 coverage。

回滚：停止创建新的 Go session；已存在 Go session 继续由 Go 服务完成或暂停，绝不能把中途状态交给 TS 猜测。

### Phase 4：默认切 Go，保留 TS 逃生通道

- BFF 默认路由新 session 到 Go；
- 观察至少一个完整发布窗口的错误率、恢复率、质量 eval 和成本；
- TS 只接收明确 `backend_owner=ts` 的旧 session；
- 做故障演练：Go 重启、模型超时、DB busy、SSE 断线、索引不一致。

回滚：将新 session 比例设为 0；Go session 按 checkpoint 排空或暂停。

### Phase 5：退役 TS 领域后端

- 确认没有 TS owner 的活跃 session；
- 导出必要审计和历史数据，验证 Go 报告读取；
- 删除 Next.js 内存面试状态和重复规则诊断；
- Next.js 仅保留 UI/BFF，Node 24 继续作为前端构建运行时，与 Go 后端版本互不绑定。

回滚：在退役窗口内保留最后一个 TS 镜像与只读旧库；恢复仅用于旧 session，不重新启用双写。

### 16.1 数据与切流原则

- 不让 TS 和 Go 同时写同一个 session；
- 不对一项业务事实做无事务双写；需要同步时使用 outbox/event projector；
- schema migration 采用 expand -> migrate -> contract，contract 阶段至少晚一个稳定发布窗口；
- feature flag 由服务端决定并记录，客户端不能伪造 backend owner；
- 回滚优先停止新流量，保留已提交数据，禁止 destructive down migration；
- 每个阶段都有独立验收门，不以“Go 能返回 200”视为完成。

## 17. ADR

### ADR-001：Go 承载领域后端和 Harness

- 状态：Accepted
- 决策：新 Profile、Interview、Knowledge 与 Harness 逻辑使用 Go；Next.js 不保存领域状态。
- 原因：需要清晰并发控制、显式接口、可恢复 worker 和单一后端真值。
- 代价：短期维护 TS/Go 两套运行时，需建立 OpenAPI 与迁移门禁。

### ADR-002：确定性 Orchestrator，而非自由自治多 Agent

- 状态：Accepted
- 决策：状态迁移、权限、预算、幂等和提交由 Go 控制；模型角色只返回结构化提案。
- 原因：面试必须可解释、可回放、可恢复，不能依赖模型自觉遵循流程。
- 代价：需要更多领域 schema 和验证代码，但能稳定提升质量而非机械堆 Agent。

### ADR-003：SQLite 作为第一阶段事务存储

- 状态：Accepted
- 决策：继续 SQLite WAL，仓储通过 ports 隔离；先解决语义、索引和恢复问题。
- 原因：当前单机规模足够，减少迁移变量。
- 复审条件：多实例写入、持续 lock contention、租户规模或运维要求超出 SQLite 边界。

### ADR-004：Evidence-first Profile 与 Assessment

- 状态：Accepted
- 决策：Profile 字段、Claim 更新、Question 目标和 Assessment 观察都必须引用 EvidenceRef。
- 原因：防止简历项目拷打和知识评分变成无来源的模型印象。
- 代价：摄取、上下文和报告都要维护 locator 与 revision。

### ADR-005：版本化 REST 命令 + SSE 事件

- 状态：Accepted
- 决策：客户端用 REST 提交命令，用可重放 SSE 接收进展；不把 SSE 连接当作任务生命周期。
- 原因：适配现有 Web，支持断线恢复和清晰幂等语义。
- 复审条件：实时双向语音成为核心需求时评估 WebSocket/WebRTC，但领域事件不变。

### ADR-006：UTC、RFC3339 API、Unix 毫秒存储

- 状态：Accepted
- 决策：时刻与时长按第 13.5 节统一，旧 Unix 秒一次性迁移。
- 原因：消除 Node 秒/毫秒混用并使跨语言契约明确。

### ADR-007：按 session owner 的 Strangler 切流

- 状态：Accepted
- 决策：创建时固定 TS/Go owner，不迁移进行中的 session，不双写会话。
- 原因：两套状态机无法可靠逐轮互切；按会话回滚边界最清楚。

### ADR-008：知识量动态对账，不硬编码“385”

- 状态：Accepted
- 决策：文件、解析、DB、FTS、embedding 使用 generation manifest 对账并对外暴露。
- 原因：本次约 385 题与旧 DB 29 条的断层说明静态文档和启动时“非空即跳过”不可作为健康判断。

### ADR-009：模型输出必须结构化、版本化并校验

- 状态：Accepted
- 决策：关键角色返回 JSON Schema；保存 prompt/schema/model 版本和 request hash。
- 原因：只有这样才能稳定提交领域对象、回归 eval 和故障重放。

### ADR-010：原始事件不可变，摘要和报告可重建

- 状态：Accepted
- 决策：Question、Answer、Assessment 和 Claim revision 追加保存；摘要、coverage 和报告作为投影。
- 原因：支持审计、纠错、模型升级后的重算和隐私删除传播。

## 18. 第一阶段验收与下一切片

第一条端到端切片已经完成以下闭环：

1. 上传一份 JD 和一份简历；
2. 生成带 EvidenceRef 的 JobProfile、CandidateProfile 和逐轮 ClaimCheck；
3. 创建并持久化 Go-owned interview aggregate；
4. 围绕 JD requirement、知识题块和简历项目动态生成问题；
5. 提交回答，生成带材料/知识引用的 Assessment；
6. Harness 根据语义评分选择具体追问轴或下一覆盖点；
7. 重启 Go 服务后从 SQLite snapshot 恢复；
8. 生成区分已支持、未验证、矛盾和未覆盖项的报告；
9. `/health` 暴露动态知识题块数（本次为 404），`/health/live` 与 `/health/ready` 区分进程存活和 Harness 可接单。
10. NDJSON 轨迹实时展示安全执行步骤，失败时保留已完成过程与原答案并允许重试。
11. 同一 `clientAnswerId` 的并发重试只评估并提交一次；不同 payload 或同题不同 answer ID 返回 `409`。
12. 浏览器断开不取消后台 run；公开 snapshot API 可恢复 Profile、当前题、历史轮次和进度，事件游标 API 不暴露原始 payload。

下一切片是把现有同步命令升级为 `202 + runId`，将安全 trace 投影到持久 SSE，并接通 `Last-Event-ID`、stale run lease 接管、outbox dispatcher、完整 Claim Ledger revision，以及将文档二进制解析从 Next.js BFF 下沉到 Go。验收重点不是“Agent 调用了几次模型”，而是问题是否有针对性、证据是否可追溯、故障是否不会污染评分、状态是否能恢复，以及系统能否解释每一次追问。
