<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://capsule-render.vercel.app/api?type=waving&color=gradient&customColorList=6,11,20&height=180&section=header&text=OfferPilot&fontSize=42&fontColor=fff&animation=fadeIn&fontAlignY=36&desc=🚀%20AI%20全链路求职辅导%20Agent&descSize=16&descAlignY=56" />
  <source media="(prefers-color-scheme: light)" srcset="https://capsule-render.vercel.app/api?type=waving&color=gradient&customColorList=6,11,20&height=180&section=header&text=OfferPilot&fontSize=42&fontColor=222&animation=fadeIn&fontAlignY=36&desc=🚀%20AI%20全链路求职辅导%20Agent&descSize=16&descAlignY=56&descColor=444" />
  <img src="https://capsule-render.vercel.app/api?type=waving&color=gradient&customColorList=6,11,20&height=180&section=header&text=OfferPilot&fontSize=42&fontColor=fff&animation=fadeIn&fontAlignY=36&desc=🚀%20AI%20全链路求职辅导%20Agent&descSize=16&descAlignY=56" width="100%" alt="OfferPilot" />
</picture>

**纯手写 Agent Loop，不依赖 LangChain / LangGraph，完整实现 10 层 Harness 工程架构**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](LICENSE) [![Node.js](https://img.shields.io/badge/Node.js-18+-339933?style=flat-square&logo=node.js&logoColor=white)](https://nodejs.org) [![TypeScript](https://img.shields.io/badge/TypeScript-5.5-3178C6?style=flat-square&logo=typescript&logoColor=white)](https://www.typescriptlang.org) [![Claude](https://img.shields.io/badge/Claude-API-cc785c?style=flat-square&logo=anthropic&logoColor=white)](https://anthropic.com) [![Next.js](https://img.shields.io/badge/Next.js-14-000000?style=flat-square&logo=next.js&logoColor=white)](https://nextjs.org) [![SQLite](https://img.shields.io/badge/SQLite-FTS5-003B57?style=flat-square&logo=sqlite&logoColor=white)](https://sqlite.org)

[快速开始](#-快速开始) · [功能模块](#-功能模块) · [架构设计](#-架构设计) · [与 zero2Agent 的关系](#-与-zero2agent-的关系)

</div>

---

## 📖 项目简介

OfferPilot 是 [zero2Agent](https://github.com/ranxi2001/zero2Agent) 教程的**毕业实战项目**——将教程中学到的 Agent 工程知识，落地为一个完整可用的产品。

反过来，zero2Agent 教程体系也是本项目的**知识数据库**：385+ 道真实大厂面试题、Agent 工程原理深度拆解、框架对比分析，全部作为 OfferPilot 的检索知识源。

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

# 启动 Web UI
cd web && npm install && npm run dev
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
| 🔍 知识检索 | SQLite FTS5 + embedding 双通道 |
| 🖥️ 前端 | Next.js 14 + shadcn/ui + SSE 流式 |

---

## 📁 目录结构

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
├── hooks/           # Hook 管线（pre-tool / post-tool）
└── db/              # SQLite 持久层
knowledge/           # 面试题知识库（Markdown）
web/                 # Next.js Web UI
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

- 🔧 **纯手写 Agent Loop** — 不依赖任何 Agent 框架，逐层手写实现
- 🏗️ **10 层 Harness 架构** — 工业级分层设计，每层职责清晰
- 🔄 **多模型路由** — Claude / GPT-4o / DeepSeek 自动切换
- 🧠 **上下文压缩** — 5 层管理 + 3 级压缩，长对话不丢失
- 🤖 **Sub-agent 运行时** — 并发池 + 7 种专业角色
- 🎙️ **实时语音面试** — TTS 提问 + 8 种缺陷规则引擎
- 🌐 **Web UI** — Next.js 14 + SSE 流式 + 暗色主题
- ✅ **完整测试** — 10 个测试文件 / 50 个用例全通过

---

## 📄 License

[MIT](LICENSE) © [ranxi2001](https://github.com/ranxi2001)

---

<div align="center">

**如果觉得有帮助，请给个 ⭐ Star！**

[![Star History](https://img.shields.io/github/stars/ranxi2001/OfferPilot?style=social)](https://github.com/ranxi2001/OfferPilot)

</div>
