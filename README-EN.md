# OfferPilot

OfferPilot is an AI interview diagnosis agent for AI Agent / LLM engineering interviews. It is a hand-written Agent Loop project, not a LangChain / LangGraph wrapper, and is designed as a practical companion project for the `zero2Agent` learning path.

It supports text diagnosis, resume/JD analysis, multi-provider LLM routing, sub-agent execution, streaming Web UI, and voice answer diagnosis with ASR.

![OfferPilot banner](./assets/offerpilot-banner.jpg)

## Demo

### Voice Answer Diagnosis

The Web UI supports recording or uploading an audio answer, transcribing it with Mimo ASR, then sending the transcript into the existing diagnosis agent. The audio is kept in the UI for replay and download so the same recording can be reused during testing.

![Voice diagnosis demo](./assets/demo1.png)

### Thought Process Card

Audio handling is shown as a dedicated process card instead of being rendered as a duplicate user message. The card tracks transcription, diagnosis status, transcript, audio playback, and recording download.

![Thought process card](./assets/cot.png)

### Markdown Report Output

Assistant answers render GitHub-Flavored Markdown, including tables. Each diagnosis response can be copied or saved as a `.md` file.

![Markdown diagnosis demo](./assets/demo2.png)

An exported sample report is available in [demo.md](./assets/demo.md).

## What Changed Today

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

## Features

| Module | Capability | Status |
| --- | --- | --- |
| Interview diagnosis | Question + answer -> score, gaps, improvement plan | Done |
| Voice answer diagnosis | Record/upload audio -> ASR -> diagnosis | Done |
| Markdown report | Render tables, copy, save `.md` | Done |
| JD analysis | Extract skill stack, seniority signal, preparation focus | Done |
| Resume optimization | STAR, quantification, keywords, rewrite suggestions | Done |
| Resume-JD matching | Coverage, missing items, targeted packaging | Done |
| Mock interview | Generate personalized interview sequence | Done |
| Realtime interview engine | TTS text, defect rules, session report | Backend skeleton |
| Multi-agent runtime | Specialist sub-agents with concurrency pool | Done |
| Knowledge search | SQLite FTS5 + optional embeddings | Done |

## Architecture

```text
src/
  agent/            Agent Loop with tool execution and token budget
  query-engine/     Provider routing, streaming, retry, collectors
  query-engine/
    providers/      Claude, OpenAI-compatible, DeepSeek, Mock
  tools/            Tool registry and built-in interview tools
  sub-agent/        Sub-agent runtime with concurrency pool
  realtime/         ASR/TTS integration and realtime interview helpers
  knowledge/        Markdown knowledge parser, FTS search, embeddings
  context/          Layered context and compression
  memory/           Session-scoped memory store
  permission/       Tool risk gate and audit records
  session/          Session state and message history
  command/          CLI slash command parser
  hooks/            Pre/post tool hooks
  db/               SQLite persistence
  server.ts         HTTP API server with SSE

web/
  src/app/          Next.js App Router pages and API proxy routes
  src/components/   Chat UI, sidebar, input, message rendering
```

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

- The default chat model is `gpt-5.5`.
- OpenAI-compatible chat requests use `OPENAI_BASE_URL`.
- Mimo ASR/TTS uses the official `https://api.xiaomimimo.com/v1` base URL.
- Mimo ASR is implemented through `/chat/completions` with `input_audio`, following the official Mimo documentation.
- Browser recording is encoded as WAV before upload.

## Quick Start

Use Node.js 20 or 22. Node.js 24 may force native rebuilds for `better-sqlite3` on Windows.

```bash
npm install
cp .env.example .env
```

Run the API server:

```bash
npm run serve
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
```

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

## Verification

Recent local verification:

```bash
npm run build
npm test -- --run
cd web && npm run build
```

Expected result:

```text
11 test files passed
55 tests passed
Next.js production build passed
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

MIT
