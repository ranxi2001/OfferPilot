# Changelog

## [Unreleased]

## [0.4.1] - 2026-08-29

This release replaces the rule-based resume diagnosis with a real multimodal
Harness Agent that evaluates both resume content and PDF layout.

### Added

- Add the multimodal `resume_diagnostician` Harness Agent, combining extracted
  resume text with up to three rendered PDF page images for evidence-grounded
  content and layout diagnosis.
- Add authenticated `POST /api/v1/resume/diagnose` with bounded image and body
  limits, plus structured vision support in the OpenAI-compatible LLM boundary.

### Fixed

- Preserve PDF section and bullet line breaks instead of flattening an entire
  resume into one paragraph.
- Replace the Next.js rule-based resume diagnosis and generic template result
  with semantic sections, cited evidence, concrete issues, suggestions, and
  directly usable rewrites.

### Upgrade

- No database migration is required.
- Run `npm --prefix web install` to install the native canvas runtime used to
  render PDF pages, then restart both the Go API and Next.js Web service.

## [0.4.0] - 2026-08-29

This release replaces mechanical JD matching and static URL scraping with
typed Harness Agents, adds robust dynamic-job crawling, and fixes Chinese PDF
resume extraction across development and production builds.

### Added

- Add the Go Harness `web_crawler` Agent with provider/embedded-data fast paths
  and a bounded fallback loop over allowlisted `inspect_web_page` and
  `fetch_web_resource` Function Tools, with safe tool-level execution traces.
- Add authenticated `POST /api/v1/crawl` and keep Next.js `/api/parse-url` as a
  thin BFF proxy to the Go Agent.
- Add the evidence-weighted `resume_matcher` Harness Agent and authenticated
  `POST /api/v1/match` endpoint for semantic JD/resume matching.

### Fixed

- Fetch dynamic Alibaba campus position details through the public page's
  Cookie/CSRF-protected detail endpoint instead of returning the JavaScript
  shell title as the JD.
- Fetch ByteDance campus position details from its public job-post endpoint,
  including title, responsibilities, requirements, location, type, and job ID.
- Reuse the interview setup `MaterialInput` controls in JD matching, so both
  workflows share upload, paste, URL crawling, editing, and error states.
- Load the official PDF.js Node build with local packed CMaps and standard
  fonts so Chinese CID-font resumes retain their Chinese text during upload.
- Replace regex keyword-intersection matching and the frontend mock fallback
  with the typed `resume_matcher` Harness Agent, evidence mappings, and a
  validated four-dimension score breakdown.

### Security

- Restrict crawler URLs to HTTP(S), block credentials and private, loopback,
  link-local, reserved, and redirected internal destinations, and bound
  redirects, request time, and response size.

### Upgrade

- No database migration is required.
- Run `npm install` and `npm --prefix web install` to install the pinned
  `pdfjs-dist` runtime assets, then restart both the Go API and Next.js Web.
- Crawler and matcher timeout settings are optional; existing deployments use
  the documented defaults when the new variables are absent.

## [0.3.3] - 2026-08-13

This patch release restores multi-turn context in conversational diagnosis, makes long streamed answers readable, and refreshes the Agent interview knowledge base.

### Fixed

- Preserve client-provided session IDs in the Go chat endpoint and consume the canonical session ID returned by the SSE stream. Later voice or text answers now include the previous interviewer question and assistant response instead of starting an isolated session.
- Add a two-turn backend regression test that verifies the complete conversation history reaches the model on the second request.
- Pause streamed-answer auto-scroll as soon as the user scrolls upward by mouse, touch, or keyboard; disable browser scroll anchoring and provide a jump-to-latest control.

### Changed

- Synchronize 13 interview dimensions from `zero2Agent` commit `124d39d4e2c0f49ac304de100797c171ce017991`, increasing the Go knowledge loader from 404 to 486 entries while preserving OfferPilot-specific content.

### Upgrade

- No database migration or new environment variable is required. Restart both the Go API and Next.js Web service after upgrading.

本文件记录 OfferPilot 各版本面向用户和部署者的主要变化。项目遵循
[Semantic Versioning](https://semver.org/)，在 `1.0.0` 之前仍可能调整 API，
但持久化数据变更必须提供向前迁移和回滚说明。

## [0.3.2] - 2026-08-13

模拟面试新增可中途导出的完整 HTML 复盘档案，并提供稳定、版本化的 review 数据接口，
作为后续复盘 Agent、错题聚类、能力趋势和训练计划的接入点。

### Added

- 新增受鉴权的 `GET /api/v1/interviews/{interviewId}/review`。接口只返回已回答轮次、
  完整回答分析以及每题当时绑定的知识参考，不返回 Prompt、模型私有思维链、原始
  JD/简历正文或其他题目的知识答案。
- 模拟面试顶部和最终报告页新增“导出复盘”。导出的单文件 `.html` 包含题目、回答、
  语音转写、回答录音、评分分析、改进建议、标准答案和安全 Agent 执行轨迹。
- review schema 固定为 `1.0.0`，将 question、answer、feedback、reference、trace 和
  recording 划分为稳定数据边界，便于后续 Agent 工具复用。

### Security And Privacy

- 普通 snapshot 和面试响应继续隐藏标准答案；只有用户显式导出时才通过受保护的
  BFF 请求 review 数据。
- 标准答案严格限定为当前题已固化的 per-question evidence bundle，不做额外全库检索。
- 录音只保留在当前页面内存并以 data URL 嵌入导出文件，不上传到面试数据库。
- HTML 对用户回答、题目、分析和轨迹文本做转义，避免导出文件执行输入中的 HTML。

### Upgrade

- 本版本没有数据库 migration，可直接从 `v0.3.1` 升级或回滚。

## [0.3.1] - 2026-08-13

这是 `0.3.0` Go 主后端正式版之后的 Web 开发体验稳定性补丁，不改变 API、数据库
schema 或部署配置。

### Fixed

- 删除 Next.js 配置中迁移完成后遗留的 `better-sqlite3` external package 声明。
- Web 开发服务器启动前检查目标端口和 `.next/dev/lock`。发现残留 Next.js 进程时
  直接输出可执行的排障命令并退出，避免自动切换端口后继续争用同一开发锁。
- 补充端口占用、重复 `next dev`、静态资源失效和白屏的排查说明。

### Safety

- 预检脚本只读检查端口与锁，不会自动结束进程或删除缓存文件。
- 本版本没有数据库 migration，可直接从 `v0.3.0` 升级或回滚。

## [0.3.0] - 2026-08-12

`0.3.0` 是 OfferPilot Go 主后端的首个正式版本。经过两个 Alpha 版本验证后，Go API
正式接管 typed Agent Harness、面试编排、逐题知识检索、SQLite 持久化、HTTP API
以及 MiMo ASR/TTS。Next.js 继续负责 Web/BFF 和文档解析；旧 TypeScript API 仅作为
迁移期回滚入口保留。

### Highlights

- JD 与简历先抽取为带原文引用的类型化 Profile，再围绕岗位要求、项目责任和量化
  结果规划问题；Interviewer、Assessor 和 Reporter 只读取各自允许的证据。
- 知识检索改为逐题绑定，回答提交具备稳定 `clientAnswerId` 和 SQLite 事务幂等语义，
  避免网络重试导致重复计分或部分写入。
- 浏览器断开后 Go Harness 可继续有界执行，同一标签页可从公开 snapshot 恢复面试；
  安全轨迹不包含 Prompt、参考答案、简历/JD 正文或模型私有思维链。
- SQLite schema v3 提供 command、event、model invocation、checkpoint、lease 和
  outbox 执行账本，并保留从旧 schema 的事务内向前迁移。
- MiMo TTS 成为题目播报主链路；ASR 对瞬时网络与供应商故障有界重试，失败后可复用
  当前页面内存中的同一段 WAV 重新分析。
- CI 覆盖 TypeScript、Go、Web、Docker、依赖审计、版本一致性和固定离线 Eval。

### Upgrade And Rollback

- 从 `v0.3.0-alpha.2` 升级不包含新的数据库 migration，继续使用 schema v3。
- 从 `v0.2.0` 升级前必须停止写流量并备份 SQLite 数据库及其 WAL/SHM 文件；首次启动
  会在事务内迁移到 schema v3。
- 若 `.env` 显式配置 `MIMO_TTS_VOICE=alloy`，请改为 `mimo_default`。
- 详细部署、备份和回滚边界见
  [Alpha.1 发布验证](./docs/v0.3.0-alpha.1-release-verification.md) 与
  [Alpha.2 发布验证](./docs/v0.3.0-alpha.2-release-verification.md)。

## [0.3.0-alpha.2] - 2026-08-12

这是 `0.3.0` 的语音可靠性 Alpha 补丁，修复面试题播报音色和回答转写失败后必须
重新作答的问题。它不改变 Agent Harness、HTTP 成功响应或 SQLite schema。

### Added

- 当前题的 WAV、原始回答时长、面试 ID 和题目 ID 会保留在页面内存中；转写失败
  后提供“重新分析录音”，重试复用完全相同的 Blob，无需重新回答。
- Go ASR 客户端对 EOF、UnexpectedEOF、超时、连接重置、broken pipe、GOAWAY、
  `429` 和 `5xx` 最多尝试 3 次，并使用 `100ms / 200ms` 有界退避。

### Changed

- 模拟面试题播报优先使用 MiMo TTS，服务或播放失败时才回退浏览器语音。
- MiMo TTS 默认音色统一为官方预设 `mimo_default`，Go 主链路和旧 TypeScript
  回滚链路保持一致。
- ASR 请求取消和普通 `4xx` 不重试；TTS 保持单次 provider 请求，避免重复合成或
  重复计费。

### Security

- Go API 与 Next.js BFF 不再向浏览器回显 provider URL、密钥、EOF 或内部响应正文，
  只返回稳定的中文错误和 `retryable` 语义。
- 录音缓存不写入 `sessionStorage`、IndexedDB 或磁盘；切题、提交成功、重置或页面
  卸载时立即释放。

### Migration And Rollback

- 本版本没有数据库 migration，继续使用 schema v3，可直接回滚到
  `v0.3.0-alpha.1` 而无需数据库降级。
- 旧 `.env` 若显式配置 `MIMO_TTS_VOICE=alloy`，升级时必须改为
  `MIMO_TTS_VOICE=mimo_default` 并重启 API 与 Web；未配置该变量时自动使用新默认值。

### Verification

- 完整命令、测试计数、真实 MiMo ASR/TTS 冒烟和已知边界见
  [v0.3.0-alpha.2 发布验证](./docs/v0.3.0-alpha.2-release-verification.md)。

### Known Limitations

- 录音只保存在当前页面内存；刷新、关闭页面、切题、提交成功或重置后无法恢复。
- 旧版本已释放的录音不能由本补丁事后找回。

## [0.3.0-alpha.1] - 2026-08-11

这是 `0.3.0` 质量与故障语义工作流的首个 Alpha。它用于验证新的证据边界、
Profile 契约、Answer 幂等语义和持久化基础，不代表 `0.3.0` GA 的全部恢复、
模型质量和部署门禁已经完成。

### Added

- 新增 `backend/internal/profile` 类型化 Profile 契约。JD 与简历事实按原文锚点
  提取，每个事实必须携带可解析的 `EvidenceRef`；确定性提取器与可选 Agent
  提取端口共享同一套 grounding 校验。
- 面试规划开始消费类型化 Profile，用岗位必备项、职责、技术主题、项目、个人
  责任和量化指标生成覆盖点，不再只依赖松散文本标签。
- 新增逐题知识检索：检索 query 同时考虑当前覆盖点、上一题和上一轮差距，每道
  问题固定自己的 evidence bundle，Assessor 只能读取该题绑定的私有参考。
- SQLite schema 升级到版本 3，新增 durable command、追加事件、模型 invocation、
  checkpoint、run lease 和 outbox 表及其唯一约束和恢复查询接口。
- Answer 请求新增稳定的 `clientAnswerId`。幂等作用域固定为
  `(principal, interview, action, clientAnswerId)`，相同 ID 与相同 payload 可复用
  已提交结果；相同 ID 改写 payload 或用不同 ID 重答已提交问题会返回冲突。
- 新增公开 session snapshot 与事件元数据 API。Web 可在同一标签页刷新后恢复 Profile、
  当前问题、历史轮次、反馈和进度；执行轨迹按字段白名单写入有界
  `sessionStorage`，不保存答案、材料、Prompt 或私有推理。
- 新增可重复、离线且不调用 provider 的 Eval Harness，并接入 CI。内置
  `v0.3.0-alpha.1` corpus 包含 30 个案例、90 道全局唯一问题，覆盖知识、项目、
  混合三种模式和 junior、mid、senior 全部 9 种组合。

### Changed

- 知识检索从“会话开始时一次 top-K”调整为“每个覆盖点、每轮重新绑定”，并移除
  Assessor 从会话头部补入无关知识锚点的路径。
- Profile 事实采用 extractive Alpha 契约：字段值必须能在其引用原文中找到；无法
  绑定证据的推断不会进入事实字段。
- Answer 的 snapshot、command result 和 `answer.committed` 事件使用同一个 SQLite
  事务提交，避免只写入部分状态。
- NDJSON 请求读取完成后改用独立的五分钟有界 context 执行。浏览器断开只停止
  实时投递，不取消已经开始的 Go Harness run；刷新恢复会按安全事件游标等待终态。
- CI 现在同时断言根项目、Web lockfile、Go 运行版本和容器 readiness 返回的版本
  均为 `0.3.0-alpha.1`。

### Security

- Interviewer 和 Assessor 的知识上下文缩小到当前问题绑定的证据集合，降低跨主题
  参考答案进入下一题、评估或公开投影的风险。
- Profile 校验拒绝未知 source、anchor、locator、quote 和无法由 quote 支撑的事实。
- 离线 Eval 扫描问题及公开文本中的标准/案例私有 marker；finding 只输出 marker
  的 SHA-256 短指纹，不回显私有内容。

### Migration

- 首次用本版本启动 Go API 时，会在事务内把面试 SQLite schema 从版本 1 依次迁移
  到版本 3。版本 2 新建执行账本表；版本 3 重建 command 表，把幂等唯一键扩展到
  principal、session 和 action 维度。
- 版本 1 的 `interview_sessions` snapshot 保持可读写；已有版本 2 command 会保留
  ID、request hash、状态、结果和错误，并以空 `principal_id` 迁入版本 3。
- 升级前必须停止写流量并对 `DB_PATH` 指向的数据库做一致性备份。Docker 部署需
  备份 `app-data` volume；文件部署应把数据库及存在的 `-wal`、`-shm` 文件作为同一
  组处理，或使用 SQLite 在线备份机制。
- 本版本没有新增必填环境变量，也没有改变 npm 依赖图；两个 lockfile 的变化仅为
  根包版本元数据。

### Rollback

- migration 是 forward-only，不提供自动 down migration。不要手工删除 migration
  记录、账本表或 command 列。
- 需要回滚时，先停止 Alpha 写流量，另行保留当前数据库以便排障，再恢复升级前的
  一致性备份并部署不可变的 `v0.2.0` artifact。
- 恢复升级前备份会舍弃 Alpha 窗口内的新会话和新答案。没有备份时，直接让
  `v0.2.0` 写入 schema v3 数据库不属于本版本验证或支持的回滚路径。

### Verification

- 离线 Eval 在固定 corpus 上通过：30/30 案例、90/90 问题证据有效，121/121
  evidence reference 可解析，重复问题为 0，三种模式、三个职级和 9 个组合覆盖率
  均为 100%，120 个公开扫描字段的 privacy marker 命中为 0。
- schema 迁移测试覆盖 v1 snapshot 到 v3 的保持与后续写入，以及已发布 v2 command
  到 v3 幂等作用域的迁移。
- Answer 针对性测试覆盖 20 个相同并发提交只执行一次 Assessor、turn 和下一题，
  同键失败重试、冲突语义，以及事件写入失败时 snapshot 与 command 整体回滚。
- 完整候选版命令、证据边界和未覆盖项见
  [v0.3.0-alpha.1 发布验证](./docs/v0.3.0-alpha.1-release-verification.md)。

### Known Limitations

- 离线 Eval 验证结构、覆盖、证据引用、重复题和已知 privacy marker，不评价真实
  模型的问题相关性、深挖强度、事实正确性或延迟；真实 provider 重复运行和人工
  双盲评审仍是后续 Beta/RC 门禁。
- durable command/event/invocation/checkpoint/lease/outbox 是恢复基础。本 Alpha 已支持
  同标签页 snapshot 与有界安全轨迹恢复，但不承诺跨设备轨迹同步、跨进程 durable
  worker 接管、完整 event-sourced 重建或 SSE `Last-Event-ID` 重放。
- 用户可见执行轨迹只包含安全步骤、状态、耗时和决策摘要；不会展示模型私有原始
  思维链、Prompt、知识参考答案或 JD/简历正文。
- Profile 的生产默认路径仍是确定性、extractive 提取；语义 Profile Agent 的质量
  标注集和 F1 门禁尚未完成。
- 知识召回仍使用 BM25；混合检索、向量召回和 reranker 不在本 Alpha 中。
- SQLite 数据仍是单机、未加密的 session snapshot，没有账号/tenant 隔离、UI 删除
  和自动保留策略。

## [0.2.0] - 2026-08-11

### Added

- 新增 Go 主后端和 typed Agent Harness，由 Planner、Interviewer、Assessor、
  Reporter 分别承担覆盖规划、出题、评估和报告生成。
- 模拟面试支持同时上传、粘贴或通过 URL 获取 JD 与简历，并提供知识拷打、
  项目深挖和混合模式。
- 新增基于 SQLite WAL 和乐观版本 CAS 的面试会话持久化，进程重启后可恢复
  当前问题。
- 新增 NDJSON 实时执行轨迹，前端保留每次运行及步骤的
  `queued -> running -> completed/failed` 状态和耗时。
- Markdown 知识库按原子问答块加载；本版本仓库中的 36 个知识文件可解析为
  404 个题块，并通过 BM25 检索。
- 新增 Go API 的 `/health/live`、`/health/ready` 和运行版本字段。

### Changed

- 默认服务入口从 TypeScript API 切换为 Go API；TypeScript API 仅作为
  `npm run serve:legacy` 回滚路径保留。
- Node.js 运行时统一为 24，Go 工具链统一为 1.26。
- 固定题单和机械 `next` 被逐轮语义评估与自适应追问取代。
- 混合面试按每道题适用的 rubric 先归一化，再按轮次等权计算总分。
- `ready` 至少需要 4 轮有效评估；混合面试要求知识和项目各至少 2 轮，
  同时检查 JD 覆盖、项目深挖和明确矛盾。
- `report_only` 模式不再提前返回逐题 Assessment，最终报告统一给出反馈。

### Security

- 知识参考答案只进入 Assessor 私有上下文；Planner、Interviewer、Reporter、
  HTTP 响应和执行轨迹只使用候选人可见投影。
- 新增问题、评估、报告及旧缓存报告的跨知识锚点泄漏检测和失败关闭策略。
- URL 材料获取拒绝 loopback、私网和其他不允许的目标，避免 SSRF。
- Agent 调用失败、超时或修复后仍不合法时返回可重试错误，不提交机械兜底分数。
- Web 容器改为非 root 用户运行；Compose 默认只把 Go API 绑定到宿主机
  `127.0.0.1`。
- 修复并锁定受影响的前端构建依赖，根项目和 Web 的高危依赖审计均为 0。

### Verification

- Go 全量测试与 `go vet` 通过。
- Node/Vitest 共 66 项测试通过。
- Next.js 生产构建和 Docker Compose 配置检查通过。
- 已在实际模型供应商上完成一场同时包含 JD 与简历的 4 轮混合面试：知识题与
  项目题各 2 轮，全部回答成功提交并生成报告。脱敏记录见
  [v0.2.0 发布验证](./docs/v0.2.0-release-verification.md)。
- 已验证持久化旧会话可在 API 重启后继续；Assessor 超时的失败请求不会提交
  答案或机械兜底评分。
- 隐私合同测试覆盖问题、评估、报告、旧缓存报告和正向泄漏对照；另对该次真实
  会话的 8 个私有知识 reference 做规范化窗口扫描，未观察到报告字段泄漏命中。
- 本次真实供应商请求耗时约 8.4 至 170.8 秒；单场冒烟不能替代下一版本计划的
  固定质量 Eval、重复运行和延迟分阶段统计。

脱敏验收记录见 [v0.2.0 发布验证](./docs/v0.2.0-release-verification.md)。

### Known Limitations

- 执行轨迹是安全的步骤、状态、耗时和决策摘要，不包含模型私有原始思维链。
- PDF、DOCX、TXT、Markdown、TeX 和 URL 的文本提取仍位于 Next.js BFF，尚未
  迁入 Go。
- 当前恢复模型以版本化 SQLite aggregate 为主，尚未实现完整追加事件账本、
  answer 幂等键和断线事件重放。
- Go 主链路当前使用 OpenAI-compatible 文本接口；其他 provider 仍主要由旧
  TypeScript CLI/API 承载。
- 当前知识检索为 BM25；向量召回、重排和可量化的检索评测属于下一版本范围。
- 当前部署使用单机 SQLite 和共享 bearer token，没有账号或 tenant 隔离；JD、
  简历和回答以未加密 session snapshot 存盘，尚无 UI 删除和自动保留策略。
- 启用生产配置写入后，provider 密钥以明文 `.env` 保存在 `app-config` volume；
  默认保持配置 API 关闭，并依赖宿主机卷权限、磁盘加密和备份访问控制。
- `/health/ready` 验证模型配置存在，但不主动请求供应商；它不等价于供应商实时
  可达。
- 活跃会话必须继续使用创建它的 Go 或旧 TypeScript 后端，暂不支持中途切换或
  自动迁移。
- `report_only` 会延迟逐题反馈；慢供应商调用可能接近各角色 90/180 秒的超时
  上限。

下一版本计划见 [v0.3.0 优化方案](./docs/v0.3.0-optimization-plan.md)。

[0.4.0]: https://github.com/ranxi2001/OfferPilot/releases/tag/v0.4.0
[0.3.3]: https://github.com/ranxi2001/OfferPilot/releases/tag/v0.3.3
[0.3.2]: https://github.com/ranxi2001/OfferPilot/releases/tag/v0.3.2
[0.3.1]: https://github.com/ranxi2001/OfferPilot/releases/tag/v0.3.1
[0.3.0]: https://github.com/ranxi2001/OfferPilot/releases/tag/v0.3.0
[0.3.0-alpha.2]: https://github.com/ranxi2001/OfferPilot/releases/tag/v0.3.0-alpha.2
[0.3.0-alpha.1]: https://github.com/ranxi2001/OfferPilot/releases/tag/v0.3.0-alpha.1
[0.2.0]: https://github.com/ranxi2001/OfferPilot/releases/tag/v0.2.0
