# OfferPilot TODO - Review Findings

> 基于 2026-06-17 代码审查，按优先级排序

---

## ~~P0 - Agent Loop 核心缺陷~~ ✅ 已修复 (2026-06-17)

### ~~1. 循环耗尽无 fallback~~ ✅
- **修复**: 循环结束后检查 `finalText` 为空时返回兜底提示消息

### ~~2. tool 错误缺少 is_error 语义~~ ✅
- **修复**: `executeTool` 返回 `{ output, isError }`，Message 类型增加 `isError` 字段，ToolResult 增加 `isError` 字段

### ~~3. 缺少 abort/cancel 机制~~ ✅
- **修复**: AgentConfig 增加 `abortSignal?: AbortSignal`，循环每轮检查 `signal.aborted`

### ~~4. tool 串行执行，不支持并行~~ ✅
- **修复**: 使用 `Promise.all` 并行执行同一轮的 tool calls

### ~~5. permission requiresUserConfirm 被忽略~~ ✅
- **修复**: 新增 `onPermissionRequest` 回调，loop 中检测 `requiresUserConfirm` 并等待用户确认

### ~~6. 无 token 用量追踪~~ ✅
- **修复**: 新增 `UsageStats` 接口、`trackUsage()` 方法、`getUsage()` 公开方法，支持 `maxBudgetTokens` 超限终止

---

## ~~P1 - Sub-Agent 系统断连~~ ✅ 已修复 (2026-06-17)

### ~~7. sub-agent 未接入主循环~~ ✅
- **修复**: `app.ts` 中初始化 SubAgentRuntime，注册所有角色，通过 `dispatch_sub_agent` tool 接入主循环

### ~~8. sub-agent 无自身 tool loop~~ ✅
- **修复**: `runtime.ts` 的 `execute` 方法内嵌 mini agent loop，支持多轮 tool_use 迭代（maxIterations 可配置）

### ~~9. 无父子上下文传递~~ ✅
- **修复**: `SubAgentTask` 增加 `context?: string` 字段，dispatch 时作为前置消息注入子代理对话

---

## ~~P1 - 数据持久化层为空壳~~ ✅ 已修复 (2026-06-17)

### ~~10. Memory 纯内存无持久化~~ ✅
- **修复**: MemoryStore 构造函数接收可选 DB 实例，add/query/remove 操作同步写入 SQLite，启动时从 DB 加载

### ~~11. Session 纯内存无持久化~~ ✅
- **修复**: SessionManager 构造函数接收可选 DB 实例，create/transition/addMessage 同步写入，启动时恢复 sessions + messages

### ~~12. 压缩后 messages 未写回 session~~ ✅
- **修复**: AgentLoop 压缩后调用 `sessionManager.replaceMessages()` 同步更新 session 和 DB

---

## ~~P2 - Context 压缩质量差~~ ✅ 已修复 (2026-06-17)

### ~~13. summarizeOlderMessages 是截断不是摘要~~ ✅
- **修复**: 新增 `compressAsync()` 方法调用 LLM 生成真实摘要；同步 fallback 改为实体提取（主题/决策/偏好）+ 最近交流保留，不再硬截断

---

## ~~P2 - Query Engine 问题~~ ✅ 已修复 (2026-06-17)

### ~~14. model 为 undefined 时 router.resolve 崩溃~~ ✅
- **修复**: router.resolve 显式处理 undefined/空值，fallback 到第一个 provider 的 defaultModel，无 provider 时抛出清晰配置提示

### 15. countTokens hardcoded model ⏳
- **位置**: `src/query-engine/providers/claude.ts:84`
- **问题**: 无论实际用什么模型，countTokens 都用 `claude-sonnet-4-20250514`
- **修复**: 使用 params 传入的实际 model（不影响功能，低优先级）

---

## ~~P2 - Tool 系统问题~~ ✅ 已修复 (2026-06-17)

### ~~16. diagnose_answer 纯规则不过 LLM~~ ✅
- **修复**: 规则引擎做初筛打 baseline，通过 SubAgentRuntime 派遣 diagnostician 子代理做深度诊断，LLM 输出解析失败时自动回退规则结果

### ~~17. search_knowledge 数据库错误被静默吞~~ ✅
- **修复**: `catch {}` → `catch (err) { console.warn(...) }`，输出中增加 `source: 'database'` 标识数据来源

### ~~18. embedding search 未实现~~ ✅
- **修复**: 新增 `EmbeddingProvider` 接口 + `searchAsync()` 向量检索（cosine similarity）+ `indexEmbeddings()` 批量生成向量；无 provider 时自动 fallback 到 LIKE 搜索

### ~~19. realtime_interview sessions 全局变量~~ ✅
- **修复**: session key 改为 `${ctx.sessionId}:${interviewId}` 实现会话级隔离，导出 `clearInterviewSessions()` 供清理

---

## ~~P3 - Web/API 层~~ ✅ 已修复 (2026-06-17)

### ~~20. Web API 没有对接真实 AgentLoop~~ ✅
- **修复**: 新增 `src/server.ts` 独立 HTTP 后端（Node.js native），实现 `/api/chat` SSE 端点直接实例化 AgentLoop；`npm run serve` 启动

### ~~21. 无认证/session 管理~~ ✅
- **修复**: 后端 + Next.js route 均增加 Bearer token 验证（`OFFERPILOT_API_KEY` 环境变量），无 key 时跳过验证

### ~~22. SSE 无超时/心跳~~ ✅
- **修复**: 每 15s 发送 `: ping\n\n` 注释帧，连接销毁时自动清除 interval

---

## ~~P3 - 工程完备性~~ ✅ 已修复 (2026-06-17)

### ~~23. 缺 E2E 测试~~ ✅
- **修复**: 新增 `tests/e2e/agent-flow.test.ts`，5 个测试覆盖会话创建、Agent 运行、工具调用、token 追踪、fallback 消息

### ~~24. 缺 graceful shutdown~~ ✅
- **修复**: CLI 注册 SIGTERM/SIGINT handler（`setupGracefulShutdown`），server.ts 同样处理信号关闭 DB 和 HTTP server

### ~~25. 缺结构化 logging~~ ✅
- **修复**: 新增 `src/logger.ts`，JSON 结构化日志（level/msg/ts + context），支持 child logger、LOG_LEVEL 环境变量

### ~~26. Knowledge 目录内容缺失~~ ✅
- **修复**: 补充 `knowledge/README.md` 导入指南 + 创建 04-07 维度目录 + 新增 RAG/多Agent 示例题目

---

## 建议修复顺序

```
Phase 1 (Agent Loop 健壮性):  ✅ 已完成 (#1 → #2 → #3 → #6 → #4 → #5)
Phase 2 (Sub-Agent 激活):     ✅ 已完成 (#7 → #8 → #9 → #16)
Phase 3 (持久化):             ✅ 已完成 (#10 → #11 → #12 → #14)
Phase 4 (智能升级):           ✅ 已完成 (#13 → #18)
Phase 5 (Web 生产化):         ✅ 已完成 (#20 → #21 → #22)
Phase 6 (工程打磨):           ✅ 已完成 (#17 → #19 → #23 → #24 → #25 → #26)
```
