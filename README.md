<div align="center">

<img src="./assets/offerpilot-banner.jpg" alt="OfferPilot - AI Interview Diagnosis Agent" width="100%" />

**纯手写 Agent Loop，不依赖 LangChain / LangGraph，完整实现 10 层 Harness 工程架构**

[![CI](https://github.com/ranxi2001/OfferPilot/actions/workflows/ci.yml/badge.svg)](https://github.com/ranxi2001/OfferPilot/actions) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](LICENSE) [![Node.js](https://img.shields.io/badge/Node.js-20+-339933?style=flat-square&logo=node.js&logoColor=white)](https://nodejs.org/) [![TypeScript](https://img.shields.io/badge/TypeScript-5.5+-3178c6?style=flat-square&logo=typescript&logoColor=white)](https://www.typescriptlang.org/) [![Claude API](https://img.shields.io/badge/Claude-API-FF6B35?style=flat-square)](https://console.anthropic.com/) [![Next.js](https://img.shields.io/badge/Next.js-14+-000000?style=flat-square&logo=next.js&logoColor=white)](https://nextjs.org/)

[快速开始](#-快速开始) · [功能模块](#-功能模块) · [架构设计](#-架构设计) · [与 zero2Agent 的关系](#-与-zero2agent-的关系)

</div>

---

## 📖 项目简介

OfferPilot 是 [zero2Agent](https://github.com/ranxi2001/zero2Agent) 教程的**毕业实战项目**——将教程中学到的 Agent 工程知识，落地为一个完整可用的产品。

反过来，zero2Agent 教程体系也是本项目的**知识数据库**：385+ 道真实大厂面试题、Agent 工程原理深度拆解、框架对比分析，全部作为 OfferPilot 的检索知识库。

> **教程孵化项目，项目反哺教程** —— 学以致用的完整闭环。

---

## ✨ 功能模块

| 模块 | 能力 | 状态 |
|:---:|:---|:---:|
| 🎯 **面试诊断** | 输入面试题 + 回答 → 评分 + 差距分析 + 改进建议 | ✅ |
| 📋 **JD 分析** | 贴入 JD → 技术栈提取 + 职级判断 + 面试准备重点 | ✅ |
| 📝 **简历优化** | 段落级诊断：量化度 / STAR 结构 / 关键词覆盖 | ✅ |
| 🔗 **简历-JD 匹配** | 关键词覆盖率 + 缺失项 + 定向包装建议 | ✅ |
| 🎲 **模拟面试** | 按维度 + 难度生成个性化题目序列 | ✅ |
| 🎙️ **实时面试模拟** | TTS 提问 → 实时缺陷检测 → 逐题反馈 → 总结报告 | ✅ |

---

## 🚀 快速开始

```bash
# 克隆项目
git clone https://github.com/ranxi2001/OfferPilot.git
cd OfferPilot

# 安装依赖
npm install

# 配置 API Key（至少配一个）
cp .env.example .env
# 编辑 .env 填入你的 key

# 启动交互式诊断
npm start

# 单次诊断
npm run diagnose -- -q "Agent 的 ReAct 循环是什么" -a "就是让模型思考然后行动"

# 构建知识库索引
npm run build-kb

# 启动 API 服务器（供 Web UI 调用）
npm run serve

# 启动 Web UI
cd web && npm install && npm run dev
```

### Docker 部署

```bash
# 配置环境变量
echo "ANTHROPIC_API_KEY=sk-xxx" > .env
echo "OFFERPILOT_API_KEY=your-secret" >> .env

# 一键启动 API + Web
docker compose up -d
# API: http://localhost:3001  Web: http://localhost:3000
```

---

## 🏗️ 架构设计

```
┌───────────────────────────────────────────────┐
│             🔄 Agent Loop                     │
├───────────────────────────────────────────────┤
│  🔍 Query Engine  │  📦 Context  │  🧠 Memory │
├───────────────────────────────────────────────┤
│  🛠️ Tools  │  ⚡ Skills  │  🔒 Permission     │
├───────────────────────────────────────────────┤
│  💾 Session  │  ⌨️ Command  │  🪝 Hook        │
├───────────────────────────────────────────────┤
│            🤖 Sub-agent Runtime               │
└───────────────────────────────────────────────┘
```

### 技术栈

| 层级 | 选型 |
|:---|:---|
| 🧠 LLM 调用 | `@anthropic-ai/sdk` + `openai` SDK（直接调用，无框架） |
| 🤖 支持模型 | Claude / GPT-4o / DeepSeek |
| ⚙️ 运行时 | Node.js + TypeScript (ES2022) |
| 💾 数据库 | better-sqlite3（会话 / 记忆 / 缓存） |
| 🔍 知识检索 | SQLite FTS5 + OpenAI embedding 向量检索 |
| 🖥️ 前端 | Next.js 14 + shadcn/ui + SSE 流式 |

---

## 📁 目录结构

```
src/
├── agent/           # Agent Loop 核心循环（abort/budget/并行 tool）
├── query-engine/    # LLM 调用层（3 Provider + 重试 + 路由）
│   └── providers/   # Claude / OpenAI / DeepSeek / Mock
├── tools/           # Tool 注册与执行
│   └── builtin/     # 14 个内置工具（含 dispatch_sub_agent）
├── sub-agent/       # Sub-agent 运行时（并发池 + mini loop）
├── realtime/        # 实时面试模拟（TTS + 缺陷检测）
├── knowledge/       # 知识检索（FTS5 + embedding 向量）
├── context/         # 5 层上下文 + LLM 摘要压缩
├── memory/          # 用户画像记忆（SQLite 持久化）
├── permission/      # 风险分级权限控制 + 用户确认
├── session/         # 会话状态机 + Checkpoint（SQLite 持久化）
├── command/         # 命令解析器
├── hooks/           # Hook 管线（pre-tool / post-tool）
├── db/              # SQLite 持久层
├── logger.ts        # 结构化 JSON 日志
└── server.ts        # HTTP API 服务器（SSE + 心跳 + Auth）
knowledge/           # 面试题知识库（7 维度 Markdown）
web/                 # Next.js Web UI
tests/
├── unit/            # 10 个单元测试文件
└── e2e/             # 端到端测试
```

---

## 🔗 与 zero2Agent 的关系

```
+---------------------------+           +---------------------------+
|      zero2Agent           |           |       OfferPilot          |
|        (Tutorial)         |           |    (Hands-on Project)     |
+---------------------------+           +---------------------------+
| Agent Engineering Theory  | --------> | Architecture Impl         |
| Framework Deep Dive       | --------> | Hand-written, No Framework|
| 385+ Interview Questions  | --------> | Knowledge Retrieval Source |
| 12 Design Documents       | --------> | Layer-by-Layer Guide      |
+---------------------------+           +---------------------------+
              ^                                     |
              |            Feedback Loop            |
              +------------------------------------+
                 - Validates tutorial correctness
                 - Drives tutorial iteration
```

| | [zero2Agent](https://github.com/ranxi2001/zero2Agent) | OfferPilot |
|:---|:---|:---|
| 🎯 定位 | Agent 工程教程体系 | 教程的毕业实战项目 |
| 📚 内容 | 原理讲解 + 框架拆解 + 面试题深度解析 | 完整产品级 Agent 系统 |
| 🔄 关系 | 提供知识库 & 设计文档 | 验证教程 & 驱动迭代 |
| 🌐 地址 | [onefly.top/zero2Agent](https://onefly.top/zero2Agent) | 本仓库 |

**详细设计文档**（12 篇）见 👉 [zero2Agent/final-project](https://github.com/ranxi2001/zero2Agent/tree/main/final-project)

---

## 📊 项目特性

- 🔧 **纯手写 Agent Loop** — 不依赖任何 Agent 框架，支持 abort/budget/并行 tool 执行
- 🏗️ **10 层 Harness 架构** — 工业级分层设计，每层职责清晰
- 🔄 **多模型路由** — Claude / GPT-4o / DeepSeek 自动切换 + fallback
- 🧠 **上下文压缩** — 5 层管理 + LLM 摘要压缩 + 实体提取
- 🤖 **Sub-agent 运行时** — 并发池 + 7 种专业角色 + mini tool loop
- 💾 **全链路持久化** — Session / Memory / 知识库均写入 SQLite
- 🔍 **双通道知识检索** — FTS5 全文搜索 + embedding 向量检索
- 🎙️ **实时语音面试** — TTS 提问 + 8 种缺陷规则引擎
- 🌐 **Web UI + API** — Next.js 14 + SSE 流式 + API Key 认证 + 心跳
- 🐳 **容器化部署** — Dockerfile + docker-compose 一键启动
- ✅ **完整测试 + CI** — 11 个测试文件 / 55 个用例 + GitHub Actions

---

## 📄 License

[MIT](LICENSE) © [ranxi2001](https://github.com/ranxi2001)

---

<div align="center">

**如果觉得有帮助，请给个 ⭐ Star！**

[![Star History](https://img.shields.io/github/stars/ranxi2001/OfferPilot?style=social)](https://github.com/ranxi2001/OfferPilot)

</div>
