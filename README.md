<div align="center">

# OfferPilot

**让每一次 AI / LLM 工程面试，都有证据、有反馈、有进步。**

从 JD 与简历分析，到自适应模拟面试、语音诊断和能力报告的一站式 AI 面试 Agent。

[![CI](https://github.com/ranxi2001/OfferPilot/actions/workflows/ci.yml/badge.svg)](https://github.com/ranxi2001/OfferPilot/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ranxi2001/OfferPilot?include_prereleases&label=release)](https://github.com/ranxi2001/OfferPilot/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](./backend)
[![Node.js](https://img.shields.io/badge/Node.js-24-5FA04E?logo=nodedotjs&logoColor=white)](./package.json)

[English](./README-EN.md) · [快速开始](#-快速开始) · [功能模块](#-功能模块) · [架构概览](#-架构概览) · [版本记录](./CHANGELOG.md)

</div>

![OfferPilot 产品界面](./assets/offerpilot-banner.jpg)

> 🧩 **求职投递搭档：[OfferPilot-plugin](https://github.com/zlr930/OfferPilot-plugin)**
>
> 自动填写简历与网申表单，减少重复录入。OfferPilot 帮你准备面试，配套插件帮你更轻松地完成投递。

### ✨ 为什么选择 OfferPilot？

- **证据驱动**：同时读取 JD 与简历，围绕真实经历生成问题、追问与评分。
- **完整闭环**：覆盖文本诊断、语音回答、自适应模拟面试和 Markdown / PDF 报告。
- **可审计 Agent**：展示安全的执行轨迹与决策摘要，不暴露私有思维链和敏感材料。
- **工程化后端**：Go typed Agent Harness + Next.js Web/BFF，不依赖 LangChain / LangGraph。

项目也是 `zero2Agent` 学习体系的实战项目，将 Agent 工程知识、面试题库和架构设计落地为可运行系统。推荐使用 server-backed 部署：Next.js Web 通过受保护的 Go API 调用 LLM / ASR / TTS provider。

## 🎉 v0.3.0 正式版

OfferPilot 的后端已从 TypeScript 切换为 **Go**。Go API 现在承载 typed Agent Harness、面试编排、逐题知识检索、SQLite 持久化以及 MiMo ASR / TTS；Next.js 负责 Web/BFF 和 PDF、DOCX、URL 文档解析。

- **更可靠**：回答幂等提交、浏览器断开后有界执行、会话快照恢复和 schema v3 执行账本。
- **更可信**：JD 与简历类型化证据、逐题检索隔离、受约束的 Interviewer / Assessor / Reporter。
- **更可观测**：安全执行轨迹、明确的 readiness、稳定错误语义和离线 Eval 门禁。
- **更完整的语音体验**：MiMo TTS 主链路、瞬时 ASR 故障重试和失败录音重新分析。

[查看完整更新记录](./CHANGELOG.md#030---2026-08-12) · [部署说明](./docs/deployment.md) · [从 Alpha 升级](./docs/v0.3.0-alpha.2-release-verification.md)

## 🎬 Demo

### 录音回答诊断

前端支持直接录音或上传音频。系统会把录音转成 WAV，调用 Mimo ASR 转写，再把转写文本送入现有面试诊断 Agent。录音会保留在页面里，方便回放和下载复测。

![录音诊断 Demo](./assets/demo1.png)

### 可审计处理轨迹

录音与模拟面试处理不会再伪装成重复的用户消息，而是单独展示可审计执行轨迹。轨迹保留排队、执行、完成/失败、耗时和安全的决策摘要；模型私有原始思维文本、Prompt、简历/JD 正文和知识参考答案不会进入轨迹。

![可审计处理轨迹](./assets/cot.png)

### 完整模拟面试执行轨迹

每轮回答从接收、校验、评估、覆盖规划、证据检索、出题到持久化都会保留连续的安全事件，便于核对 Agent Harness 实际执行了什么以及每一步耗时。

![模拟面试 Agent 执行轨迹](./assets/mock-interview-agent-trace.png)

### 证据化回答反馈

逐轮反馈把成立点、待补漏洞、材料核对、下一步策略和本题证据放在同一视图中，让追问依据和评分边界可以直接检查。

![模拟面试证据化回答反馈](./assets/mock-interview-evidence-feedback.png)

### Markdown 诊断报告

诊断结果支持 GitHub-Flavored Markdown，包含表格渲染。每条回答尾部提供复制和保存 `.md` 文档的快捷操作。

![Markdown 诊断报告](./assets/demo2.png)

导出的示例报告见：[demo.md](./assets/demo.md)。

## v0.3.0-alpha.2 更新

- 面试题播报改用 MiMo TTS 主链路，默认官方音色 `mimo_default`；浏览器语音仅在
  服务不可用或音频无法播放时兜底。
- 语音转写发生瞬时故障时，Go API 会对 EOF、超时、连接重置、`429` 和 `5xx`
  最多尝试 3 次；普通 `4xx` 和已取消请求不会重试。
- 当前题的原始 WAV 录音只缓存在页面内存中。转写仍失败时可点击“重新分析录音”，
  复用完全相同的录音和时长，无需重新回答。
- Go API 与 Next.js BFF 统一返回可重试语义，并屏蔽 provider URL、密钥、EOF 和
  内部响应正文。
- 本补丁不包含数据库 migration，继续使用 schema v3。部署与回滚边界见
  [Alpha.2 发布验证](./docs/v0.3.0-alpha.2-release-verification.md)。

## v0.3.0-alpha.1 更新

- JD 与简历先抽取为带原文 `EvidenceRef` 的类型化 Profile，再由 Planner 按岗位
  必备项、职责、项目、个人贡献和量化指标安排深挖路径。
- 知识题改为逐题检索：当前覆盖点、上一题和回答差距共同组成 query，每道题只把
  自己绑定的 evidence bundle 交给 Interviewer 与 Assessor。
- Answer 请求使用稳定 `clientAnswerId`；相同请求重试复用已提交结果，改写 payload
  或重复回答已提交问题会返回冲突，避免刷新和网络重试造成重复计分。
- SQLite schema v3 新增 command、追加事件、模型 invocation、checkpoint、lease 和
  outbox 账本，为后续完整断线恢复提供持久化基础。
- 浏览器断开后 Go Harness 会继续有界执行；同一标签页刷新可从公开 snapshot 恢复
  当前题、反馈、进度和历史轮次，并保留经过白名单过滤的完整安全执行时间线。
- 新增离线 Eval Harness 并接入 CI：固定 30 个案例、90 道全局唯一问题，覆盖
  `knowledge / projects / mixed`、三个职级和全部 9 种组合；当前 corpus 的 121 个
  evidence reference 全部有效，120 个公开字段隐私扫描命中为 0。
- 版本仍是 Alpha：当前不承诺 SSE `Last-Event-ID` 重放、跨进程 worker 接管或
  跨设备轨迹同步。用户看到的是连续的安全执行事实与决策摘要，不是模型私有原始
  思维链。
- 升级会自动把面试数据库迁移到 schema v3。生产升级前必须先做一致性备份；迁移
  与可恢复回滚步骤见 [Alpha 发布验证](./docs/v0.3.0-alpha.1-release-verification.md)。

## 🚀 功能模块

| 模块 | 能力 | 状态 |
| --- | --- | --- |
| 面试诊断 | 输入问题和回答，输出评分、差距、改进建议 + 可审计执行轨迹 | 已完成 |
| 录音回答诊断 | 录音/上传音频 → ASR → 诊断 | 已完成 |
| 自适应模拟面试 | JD + 简历证据 → Agent 出题 → 语义评估 → 动态追问 → 证据化报告 | 已完成 |
| 简历分析 | 段落级诊断：STAR 结构、量化度、技术决策、个人贡献 | 已完成 |
| JD 匹配 | 关键词覆盖率、缺失项、职级判断、定向包装建议 | 已完成 |
| 能力雷达 | 7 维度评分 + 学习路径推荐 + 诊断历史追踪 | 已完成 |
| 报告导出 | Markdown / PDF 一键导出诊断报告 | 已完成 |
| 多 Agent 协作 | 专家子 Agent + 并发池 | 已完成 |
| 知识检索 | Markdown 原子问答块 + 内存 BM25 top-K（向量/重排为后续演进） | 已完成 |

## 🏗️ 架构概览

```text
backend/
  cmd/offerpilot-api/  Go API 装配与优雅退出
  internal/harness/    typed 子 Agent、并发边界、trace、结构化输出
  internal/interview/  面试聚合、证据、评估、策略与报告
  internal/knowledge/  Markdown 逐题解析与 BM25 检索
  internal/httpapi/    鉴权、CORS、SSE、限额与前端兼容投影
  internal/llm/        OpenAI-compatible 模型网关
  internal/speech/     MiMo ASR/TTS

web/
  src/app/            Next.js App Router、BFF、PDF/DOCX/URL 解析
  src/components/     面试作战台、材料输入、Chat 与报告 UI
```

完整设计与迁移约束见 [Agent Harness 与 Go 后端架构](./docs/agent-harness-architecture.md)。
下一阶段的优先级、验收指标和发布门禁见 [v0.3.0 优化方案](./docs/v0.3.0-optimization-plan.md)。

## 模型与音频配置

推荐配置：

- 文本模型：推荐使用 [ai.tosky.top](https://ai.tosky.top/) 提供的 OpenAI 兼容接口，默认模型为 `gpt-5.5`。
- 语音模型：推荐使用 [小米 MiMo 开放平台](https://platform.xiaomimimo.com?ref=6ENEDG) 的 MiMo V2.5 系列模型。
  - ASR：`mimo-v2.5-asr`
  - TTS：`mimo-v2.5-tts`
  - TTS 成本参考：约 1 分钟 1 分钱。
  - 邀请码：`6ENEDG`
  - 注册链接：[https://platform.xiaomimimo.com?ref=6ENEDG](https://platform.xiaomimimo.com?ref=6ENEDG)
  - 通过邀请码注册，双方各得 10 元 API 体验金，首单 9 折；体验金有效期 40 天。

复制 `.env.example` 为 `.env`，按需填写 key。

```env
OPENAI_API_KEY=sk-...
OPENAI_BASE_URL=https://api.ai.tosky.top/v1
OPENAI_MODEL=gpt-5.5

MIMO_API_KEY=sk-...
MIMO_BASE_URL=https://api.xiaomimimo.com/v1
MIMO_ASR_MODEL=mimo-v2.5-asr
MIMO_TTS_MODEL=mimo-v2.5-tts
MIMO_TTS_VOICE=mimo_default
```

说明：

- Go 后端使用 OpenAI-compatible 文本接口，默认聊天模型是 `gpt-5.5`。
- OpenAI 兼容模型走 `OPENAI_BASE_URL`。
- Mimo ASR/TTS 使用官方 `https://api.xiaomimimo.com/v1`。
- MiMo TTS 默认使用官方预置音色 `mimo_default`，可通过 `MIMO_TTS_VOICE` 覆盖。
- Mimo ASR 按官方文档通过 `/chat/completions` 的 `input_audio` 调用。
- 浏览器录音会先编码成 WAV，再上传给后端转写。

## ⚡ 快速开始

项目使用 Go 1.26 和 Node.js 24：Go 负责 API 与 Agent Harness，Node.js 仅用于 Next.js Web/BFF。

```bash
cp .env.example .env
cd web && npm install && cd ..
```

终端 1：启动 Go API：

```bash
cd backend
go run ./cmd/offerpilot-api
```

终端 2：启动 Web UI：

```bash
cd web
npm run dev
```

打开：

```text
http://localhost:3000
```

健康检查：

```text
http://localhost:3001/health/live
http://localhost:3001/health/ready
http://localhost:3000/api/health
```

`/health/live` 只表示进程存活；部署和流量入口必须使用 `/health/ready`。模型未配置时 readiness 返回 `503`，面试接口不会生成机械兜底评分。

## Web 录音诊断流程

1. 点击输入框左侧麦克风按钮。
2. 说出面试回答。
3. 再次点击停止录音。
4. OfferPilot 在可审计处理轨迹中保存录音处理状态。
5. 浏览器上传 WAV 到 `/api/transcribe`。
6. 后端调用 Mimo ASR。
7. 转写文本展示在可审计处理轨迹中。
8. 转写文本进入面试诊断 Agent。
9. 诊断结果支持复制或保存为 Markdown。

也可以通过附件按钮上传已有音频文件。

## Docker

```bash
docker compose up -d
```

服务地址：

```text
API: http://localhost:3001
Web: http://localhost:3000
```

`docker-compose.yml` 已透传 OpenAI 兼容模型和 Mimo 音频相关环境变量。

生产部署说明见：[docs/deployment.md](./docs/deployment.md)。

## 验证

最近一次本地验证命令：

```bash
cd backend && go test ./... && cd ..
npx vitest run tests/unit tests/e2e
npm --prefix web run build
git diff --check
```

预期结果：

```text
Go 后端测试通过
单元测试和 E2E 测试通过
Next.js 生产构建通过
diff whitespace 检查通过
```

## 与 zero2Agent 的关系

OfferPilot 使用 zero2Agent 的知识体系作为面试知识来源，并把 Agent 工程思想落地为完整应用：

```text
zero2Agent 理论与面试知识
        |
        v
OfferPilot 工程实现
        |
        v
Agent Loop、工具、会话、记忆、Web UI、ASR 诊断
```

## License

[MIT](./LICENSE)
