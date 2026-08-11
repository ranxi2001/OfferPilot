# Changelog

本文件记录 OfferPilot 各版本面向用户和部署者的主要变化。项目遵循
[Semantic Versioning](https://semver.org/)，在 `1.0.0` 之前仍可能调整 API，
但持久化数据变更必须提供向前迁移和回滚说明。

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

[0.2.0]: https://github.com/ranxi2001/OfferPilot/releases/tag/v0.2.0
