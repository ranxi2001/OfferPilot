# OfferPilot

基于 Harness 工程架构的 AI Agent 面试辅导系统。纯手写 Agent Loop，不依赖 LangChain / LangGraph。

**GitHub**: [https://github.com/ranxi2001/OfferPilot](https://github.com/ranxi2001/OfferPilot)

## 项目定位

全链路求职辅导 Agent，覆盖从 JD 分析到面试复盘的完整闭环：

| 模块 | 能力 |
|------|------|
| **面试诊断** | 输入面试题 + 回答 → 评分 + 差距分析 + 改进建议 |
| **JD 分析** | 贴入 JD → 技术栈提取 + 职级判断 + 面试准备重点 |
| **简历优化** | 段落级诊断：量化度 / STAR 结构 / 关键词覆盖 |
| **简历-JD 匹配** | 关键词覆盖率 + 缺失项 + 定向包装建议 |
| **模拟面试** | 按维度 + 难度生成个性化题目序列 |
| **实时面试模拟** | TTS 提问 → 实时缺陷检测 → 逐题反馈 → 总结报告 |

- 内置 385+ 道真实面试题知识库（来自 zero2Agent 项目）
- 支持"新手答 vs 高手答"对比、追问模拟、学习路径推荐

## 技术栈

| 层 | 选型 |
|---|---|
| LLM 调用 | @anthropic-ai/sdk + openai SDK（直接调用，无框架） |
| 支持模型 | Claude / GPT-4o / DeepSeek |
| 运行时 | Node.js + TypeScript (ES2022) |
| 数据库 | better-sqlite3（会话 / 记忆 / 缓存） |
| 知识检索 | SQLite FTS5 + embedding 双通道 |
| 前端 | Next.js 14 + shadcn/ui + SSE 流式 |

## 架构：10 层 Harness

```
┌─────────────────────────────────────────┐
│              Agent Loop                  │
├─────────────────────────────────────────┤
│  Query Engine  │  Context  │  Memory    │
├─────────────────────────────────────────┤
│  Tools  │  Skills  │  Permission        │
├─────────────────────────────────────────┤
│  Session  │  Command  │  Hook           │
├─────────────────────────────────────────┤
│           Sub-agent Runtime             │
└─────────────────────────────────────────┘
```

## 快速开始

```bash
# 安装依赖
npm install

# 配置 API Key（至少配一个）
cp .env.example .env
# 编辑 .env 填入你的 key

# 启动交互式诊断
npm start

# 单次诊断
npm run diagnose -- -q "Agent 的 ReAct 循环是什么" -a "就是让模型思考然后行动"
```

## 目录结构

```
src/
├── agent/           # Agent Loop 核心循环
├── query-engine/    # LLM 调用层（3 Provider + 重试 + 路由）
│   └── providers/   # Claude / OpenAI / DeepSeek
├── tools/           # Tool 注册与执行
│   └── builtin/     # 13 个内置工具
├── realtime/        # 实时面试模拟（TTS + 缺陷检测）
├── context/         # 5 层上下文 + 3 级压缩
├── memory/          # 用户画像记忆
├── permission/      # 风险分级权限控制
├── session/         # 会话状态机 + Checkpoint
├── command/         # 命令解析器
│   └── handlers/
├── hooks/           # Hook 管线
│   ├── pre-tool/
│   └── post-tool/
└── db/              # SQLite 持久层
knowledge/           # 面试题知识库（Markdown）
```

## 开发进度

- [x] Query Engine（3 Provider + 统一 StreamEvent + 重试 + 路由）
- [x] Tools（13 个内置工具 + Registry）
- [x] Permission（风险分级 + 审计日志）
- [x] Context（5 层管理 + 3 级压缩）
- [x] Session（状态机 + Checkpoint/Rewind）
- [x] Memory（内存存储 + 查询）
- [x] Agent Loop（Tool 调用循环 + 流式输出）
- [x] CLI 入口（交互模式 + 单次模式 + build-kb）
- [x] Knowledge Base（Markdown 解析 + FTS5 双通道检索）
- [x] Hooks（pre-tool / post-tool 管线 + 2 个内置 hook）
- [x] Command 解析器（5 个内置命令）
- [x] Sub-agent 运行时（并发池 + 7 角色）
- [x] DB 持久化（SQLite WAL + 完整 schema）
- [x] JD 分析 + 简历优化 + 简历-JD 匹配
- [x] 模拟面试（题目生成 + 维度/难度控制）
- [x] 实时面试模拟（TTS + 8 种缺陷规则引擎 + 会话状态机）
- [x] Web UI（Next.js 14 + SSE 流式 + 暗色主题）
- [x] 测试（10 个测试文件 / 50 个用例全通过）

## 设计文档

详细设计见 [zero2Agent/final-project](https://github.com/ranxi2001/zero2Agent/tree/main/final-project)，共 12 篇：

1. PRD → 2. 系统架构 → 3. Query Engine → 4. Tools & Skills → 5. 知识库 → 6. Context & Memory → 7. Permission & Session → 8. Hook & Command → 9. Sub-agent → 10. 语音输入 → 11. 部署演示 → 12. Web UI

## License

MIT
