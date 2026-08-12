<div align="center">

# OfferPilot

**Make every AI / LLM engineering interview grounded, actionable, and measurable.**

An end-to-end AI interview agent for JD and resume analysis, adaptive mock interviews, voice diagnosis, and evidence-grounded reports.

[![CI](https://github.com/ranxi2001/OfferPilot/actions/workflows/ci.yml/badge.svg)](https://github.com/ranxi2001/OfferPilot/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ranxi2001/OfferPilot?include_prereleases&label=release)](https://github.com/ranxi2001/OfferPilot/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](./backend)
[![Node.js](https://img.shields.io/badge/Node.js-24-5FA04E?logo=nodedotjs&logoColor=white)](./package.json)

[中文](./README.md) · [Quick Start](#-quick-start) · [Features](#-features) · [Architecture](#-architecture) · [Changelog](./CHANGELOG.md)

</div>

![OfferPilot product interface](./assets/offerpilot-banner.jpg)

> 🧩 **Job application companion: [OfferPilot-plugin](https://github.com/zlr930/OfferPilot-plugin)**
>
> Automatically fill resumes and online application forms with less repetitive data entry. OfferPilot prepares you for the interview; the companion extension helps you get there.

### ✨ Why OfferPilot?

- **Evidence-grounded**: reads the JD and resume together to generate questions, follow-ups, and assessments around real experience.
- **End-to-end workflow**: covers text diagnosis, voice answers, adaptive mock interviews, and Markdown / PDF reports.
- **Auditable agents**: exposes safe execution events and decision summaries without leaking private reasoning or sensitive material.
- **Production-oriented**: built on a Go typed Agent Harness and Next.js Web/BFF, without LangChain or LangGraph.

OfferPilot is also a practical implementation of the `zero2Agent` learning system, turning agent engineering concepts, interview knowledge, and architecture into a working application. The recommended server-backed deployment keeps LLM / ASR / TTS provider access behind the protected Go API.

## 🎉 v0.3.0 General Availability

OfferPilot's primary backend has moved from TypeScript to **Go**. The Go API now owns the typed Agent Harness, interview orchestration, per-question knowledge retrieval, SQLite persistence, and MiMo ASR / TTS. Next.js continues to provide the Web/BFF and PDF, DOCX, and URL extraction, while the legacy TypeScript API remains only as a migration rollback path.

- **More reliable**: idempotent answer commits, bounded execution after disconnects, session snapshot recovery, and a schema v3 execution ledger.
- **More trustworthy**: typed JD/resume evidence, per-question retrieval isolation, and constrained Interviewer, Assessor, and Reporter agents.
- **More observable**: safe execution traces, explicit readiness, stable error semantics, and an offline evaluation gate.
- **Complete voice workflow**: MiMo TTS by default, transient ASR retries, and failed-recording re-analysis.

[Full changelog](./CHANGELOG.md#030---2026-08-12) · [Deployment guide](./docs/deployment.md) · [Upgrade from Alpha](./docs/v0.3.0-alpha.2-release-verification.md)

## 🎬 Demo

### Voice Answer Diagnosis

The Web UI supports recording or uploading an audio answer, transcribing it with Mimo ASR, then sending the transcript into the existing diagnosis agent. The audio is kept in the UI for replay and download so the same recording can be reused during testing.

![Voice diagnosis demo](./assets/demo1.png)

### Auditable Execution Trace

Audio and mock-interview work is shown as a dedicated execution timeline instead of being rendered as duplicate chat messages. It preserves queued, running, completed, and failed steps with durations and safe decision summaries. Private model reasoning, prompts, JD/resume bodies, and knowledge reference answers are intentionally excluded.

![Thought process card](./assets/cot.png)

### Complete Mock-Interview Agent Trace

Each answer keeps a continuous safe event history from receipt and validation through assessment, coverage planning, evidence retrieval, question generation, and persistence. This makes the Harness path and step timings directly inspectable.

![Mock interview Agent execution trace](./assets/mock-interview-agent-trace.png)

### Evidence-Grounded Answer Feedback

Per-turn feedback presents supported points, remaining gaps, material checks, next-step strategy, and question evidence in one view so the basis for scoring and follow-up is visible.

![Evidence-grounded mock interview feedback](./assets/mock-interview-evidence-feedback.png)

### Markdown Report Output

Assistant answers render GitHub-Flavored Markdown, including tables. Each diagnosis response can be copied or saved as a `.md` file.

![Markdown diagnosis demo](./assets/demo2.png)

An exported sample report is available in [demo.md](./assets/demo.md).

## v0.3.0-alpha.2 Changes

- Interview question playback now uses MiMo TTS as the primary path with the official `mimo_default` voice; browser speech is only a fallback for service or playback failures.
- The Go API retries transient ASR failures such as EOF, timeouts, connection resets, `429`, and `5xx` responses up to three attempts. Ordinary `4xx` responses and canceled requests are not retried.
- The current question's original WAV stays in page memory after a failed transcription. “Re-analyze recording” reuses the exact audio and duration without asking the candidate to answer again.
- The Go API and Next.js BFF expose stable retry semantics while hiding provider URLs, credentials, EOF details, and internal response bodies.
- This patch has no database migration and continues to use schema v3. See [Alpha.2 release verification](./docs/v0.3.0-alpha.2-release-verification.md) for deployment and rollback boundaries.

## v0.3.0-alpha.1 Changes

- Added grounded typed Profile extraction for JD requirements, responsibilities, resume projects, ownership, and metrics.
- Scoped knowledge retrieval and private evidence independently for every interview question.
- Added stable browser `clientAnswerId` values and atomic SQLite answer commits. Identical retries replay one result; changed payloads and second answers conflict instead of being scored twice.
- Added durable command, event, model invocation, checkpoint, lease, and outbox persistence foundations with schema v3 migration.
- Added a deterministic CI Eval Harness with 30 cases, 90 globally unique questions, 121/121 valid evidence references, full mode/seniority matrix coverage, and zero known privacy-marker hits in 120 public fields.
- Detached bounded Go Harness runs from browser stream cancellation and added public session snapshot/event metadata recovery endpoints.
- This is an Alpha. Persistent SSE `Last-Event-ID` replay, stale worker takeover, production model quality studies, and complete server-side UI trace reconstruction remain future work.
- User-visible timelines contain safe execution facts and decision summaries, never private model chain-of-thought, prompts, reference answers, or raw JD/resume content.

See [Alpha release verification](./docs/v0.3.0-alpha.1-release-verification.md) before migrating or rolling back a deployment.

## v0.2.0 Changes

- Made Go the primary HTTP and Harness backend; `npm run serve:legacy` keeps the TypeScript API as a rollback path.
- Added JD and resume upload/paste/URL input with knowledge, project, and mixed interview modes.
- Replaced fixed question lists and mechanical `next` calls with atomic answer assessment plus adaptive follow-up.
- Moved semantic scoring into a typed Assessor; Go validates schema/evidence and applies deterministic policy only.
- Added claim verdicts: `supported`, `unverified`, `contradicted`, and `not_in_material`.
- The Go knowledge loader currently parses 404 question blocks from 36 Markdown files instead of trusting the stale 29-row database.
- Added the [Agent Harness and Go backend architecture](./docs/agent-harness-architecture.md) with editable draw.io source.
- Added real API testing path with `.env` auto-loading for CLI and API server.
- Added configurable OpenAI-compatible provider settings:
  - `OPENAI_API_KEY`
  - `OPENAI_BASE_URL`
  - `OPENAI_MODEL`
- Set the default chat model to `gpt-5.5`.
- Added Mimo audio integration:
  - ASR model: `mimo-v2.5-asr`
  - TTS model: `mimo-v2.5-tts`
  - official base URL: `https://api.xiaomimimo.com/v1`
- Added backend audio APIs:
  - `POST /api/transcribe`
  - `POST /api/tts`
- Added frontend proxy routes:
  - `web/src/app/api/transcribe`
  - `web/src/app/api/tts`
- Added browser-side WAV recording, because Mimo ASR expects `wav` or `mp3`.
- Added upload-audio diagnosis flow.
- Added process/thought-chain card for audio diagnosis.
- Added recording playback and download.
- Added Markdown table rendering with `remark-gfm`.
- Added answer actions: copy response and save as `.md`.
- Fixed diagnostician sub-agent recursion by disabling tools for the diagnostician sub-agent and limiting it to one iteration.
- Added Docker env pass-through for OpenAI-compatible and Mimo config.

## 🚀 Features

| Module | Capability | Status |
| --- | --- | --- |
| Interview diagnosis | Question + answer -> score, gaps, improvement plan | Done |
| Voice answer diagnosis | Record/upload audio -> ASR -> diagnosis | Done |
| Markdown report | Render tables, copy, save `.md` | Done |
| JD analysis | Extract skill stack, seniority signal, preparation focus | Done |
| Resume optimization | STAR, quantification, keywords, rewrite suggestions | Done |
| Resume-JD matching | Coverage, missing items, targeted packaging | Done |
| Adaptive mock interview | JD + resume evidence, semantic assessment, dynamic follow-up, report | Done |
| Realtime interview | TTS question, text/WAV answer, per-turn feedback | Done |
| Multi-agent runtime | Specialist sub-agents with concurrency pool | Done |
| Knowledge search | Atomic Markdown question blocks + in-memory BM25 | Done |

## 🏗️ Architecture

```text
backend/
  cmd/offerpilot-api/  Go API composition and graceful shutdown
  internal/harness/    typed agents, bounded concurrency, traces
  internal/interview/  evidence, assessment, policy, report aggregate
  internal/knowledge/  question-level Markdown parser and BM25 search
  internal/httpapi/    auth, CORS, SSE, limits, Web compatibility DTOs
  internal/llm/        OpenAI-compatible structured model gateway
  internal/speech/     MiMo ASR/TTS

web/                   Next.js UI/BFF and document extraction
src/                   legacy TypeScript CLI/API during migration
```

See [Agent Harness and Go backend architecture](./docs/agent-harness-architecture.md) for the full design.
See the [v0.3.0 optimization plan](./docs/v0.3.0-optimization-plan.md) for prioritized work, acceptance metrics, and release gates.

## Model And Audio Configuration

Recommended setup:

- Text model: use the OpenAI-compatible endpoint from [ai.tosky.top](https://ai.tosky.top/) with `gpt-5.5` as the default model.
- Audio models: use the Xiaomi [MiMo Open Platform](https://platform.xiaomimimo.com?ref=6ENEDG), especially the MiMo V2.5 family.
  - ASR: `mimo-v2.5-asr`
  - TTS: `mimo-v2.5-tts`
  - TTS cost reference: about RMB 0.01 per minute.
  - Referral code: `6ENEDG`
  - Registration link: [https://platform.xiaomimimo.com?ref=6ENEDG](https://platform.xiaomimimo.com?ref=6ENEDG)
  - With the referral code, both sides receive RMB 10 API trial credit, first order gets 10% off, and trial credit is valid for 40 days.

Create `.env` from `.env.example` and fill in the keys you need.

```env
OPENAI_API_KEY=sk-...
OPENAI_BASE_URL=https://api.ai.tosky.top/v1
OPENAI_MODEL=gpt-5.5

MIMO_API_KEY=sk-...
MIMO_BASE_URL=https://api.xiaomimimo.com/v1
MIMO_ASR_MODEL=mimo-v2.5-asr
MIMO_TTS_MODEL=mimo-v2.5-tts

ANTHROPIC_API_KEY=sk-ant-...
DEEPSEEK_API_KEY=sk-...
```

Notes:

- The Go backend currently uses an OpenAI-compatible text endpoint; the default chat model is `gpt-5.5`.
- Claude and DeepSeek remain available through the legacy CLI/API during migration.
- OpenAI-compatible chat requests use `OPENAI_BASE_URL`.
- Mimo ASR/TTS uses the official `https://api.xiaomimimo.com/v1` base URL.
- Mimo ASR is implemented through `/chat/completions` with `input_audio`, following the official Mimo documentation.
- Browser recording is encoded as WAV before upload.

## ⚡ Quick Start

This project uses Go 1.26 and Node.js 24. `better-sqlite3` is now legacy-only and remains compatible with Node.js 24.

```bash
npm install
cd web && npm install && cd ..
cp .env.example .env
```

Run the API server:

```bash
npm run serve
```

Use the legacy TypeScript API only for rollback:

```bash
npm run serve:legacy
```

Run the Web UI:

```bash
cd web
npm install
npm run dev
```

Open:

```text
http://localhost:3000
```

API health check:

```text
http://localhost:3001/health
http://localhost:3001/health/live
http://localhost:3001/health/ready
http://localhost:3000/api/health
```

`/health/live` only reports process liveness. Deployments and traffic gates must use `/health/ready`; it returns `503` when the model is unavailable, and interviews never commit a mechanical fallback score.

## CLI Usage

Interactive session:

```bash
npm start
```

Single diagnosis:

```bash
npm run diagnose -- -q "What is a ReAct Agent?" -a "It reasons, calls tools, observes results, and iterates."
```

Build the knowledge base:

```bash
npm run build-kb
```

Generate embeddings:

```bash
npm run embed
```

## Web Voice Diagnosis Flow

1. Click the microphone button in the chat input.
2. Speak your answer.
3. Click stop.
4. OfferPilot saves the recording in the process card.
5. The browser uploads WAV audio to `/api/transcribe`.
6. The server calls Mimo ASR.
7. The transcript is shown in the process card.
8. The transcript is sent to the diagnosis agent.
9. The response can be copied or saved as Markdown.

You can also upload an existing audio file with the attachment button.

## Docker

```bash
docker compose up -d
```

Services:

```text
API: http://localhost:3001
Web: http://localhost:3000
```

`docker-compose.yml` passes through OpenAI-compatible and Mimo environment variables.

Production deployment details are in [docs/deployment.md](./docs/deployment.md).

## Verification

Recent local verification:

```bash
npm run build
npm run test:go
npx vitest run tests/unit tests/e2e
npm --prefix web run build
git diff --check
```

Expected result:

```text
Go and legacy TypeScript builds pass
Go backend tests pass
Unit and E2E tests pass
Next.js production build passed
diff whitespace check passes
```

## Relationship With zero2Agent

OfferPilot uses the zero2Agent knowledge system as its interview knowledge source and applies the engineering ideas in a complete product-like agent:

```text
zero2Agent theory and interview knowledge
        |
        v
OfferPilot implementation
        |
        v
agent loop, tools, sessions, memory, web UI, ASR diagnosis
```

## License

[MIT](./LICENSE)
