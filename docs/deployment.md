# OfferPilot Server-Backed Deployment

The supported production topology uses Node.js 24 for Next.js and Go 1.26 for
the API and Agent Harness:

```text
Browser -> Next.js Web/BFF -> Go API/Harness -> SQLite
                                  |          -> Markdown knowledge index
                                  +---------> OpenAI-compatible LLM / MiMo
```

The TypeScript API is a migration rollback path (`npm run serve:legacy`), not
the default server. New interview sessions must stay on the backend that
created them; do not switch an active session between Go and TypeScript.

## Runtime Contract

Required in production:

- `OFFERPILOT_API_KEY`: bearer token shared by the Web BFF and Go API.
- `OFFERPILOT_REQUIRE_AUTH=true`.
- `OFFERPILOT_ALLOWED_ORIGINS`: comma-separated browser origins.
- `OPENAI_API_KEY`, `OPENAI_BASE_URL`, and `OPENAI_MODEL`: structured
  Interviewer, Assessor, Reporter, and free-form chat.
- `KNOWLEDGE_DIR`: Markdown knowledge directory, `/app/knowledge` in Docker.
- `DB_PATH`: SQLite interview state, `/app/data/offerpilot.db` in Docker.

Optional:

- `MIMO_API_KEY`, `MIMO_BASE_URL`, `MIMO_ASR_MODEL`, `MIMO_TTS_MODEL`,
  `MIMO_TTS_VOICE`: WAV/MP3 transcription and speech synthesis.
- `OFFERPILOT_HARNESS_MAX_CONCURRENT`: maximum concurrent typed Agent calls;
  defaults to `4`.
- `OFFERPILOT_INTERVIEWER_TIMEOUT`, `OFFERPILOT_ASSESSOR_TIMEOUT`,
  `OFFERPILOT_REPORTER_TIMEOUT`, `OFFERPILOT_PLANNER_TIMEOUT`: wall-clock
  limits for each typed Agent. Defaults are `90s`, `120s`, `90s`, and `90s`.
- `OPENAI_TIMEOUT`: per-provider request attempt; defaults to `90s`.
- `OFFERPILOT_MAX_INTERVIEW_BODY_BYTES`: combined extracted JD/resume JSON
  limit; defaults to 2 MiB.
- `OFFERPILOT_ENABLE_CONFIG_API`: Next.js model-config editor; keep disabled
  for public deployments.
- `OFFERPILOT_HEALTH_TIMEOUT_MS`: Next.js timeout while checking the Go API.

Provider credentials stay in server environment variables. Never expose them
through browser bundles or client-side configuration.

`POST /api/interview/stream` returns newline-delimited JSON. Trace lines contain
only fixed stage labels, statuses, aggregate counts, Agent IDs, and durations;
the final line is `{ "type": "result", "status": <http-status>, "data": ... }`.
Prompts, answers, JD/resume text, knowledge excerpts, and provider response
bodies are intentionally excluded from trace events.

## Local Development

Install Go 1.26, Node.js 24, and dependencies, then create `.env`:

```bash
npm install
npm --prefix web install
cp .env.example .env
```

Run the Go API and Next.js Web in separate terminals:

```bash
npm run serve
npm --prefix web run dev
```

Check readiness:

```bash
curl http://localhost:3001/health/live
curl --fail http://localhost:3001/health/ready
curl http://localhost:3000/api/health
```

A healthy, fully configured API reports the dynamically parsed knowledge count:

```json
{
  "status": "ready",
  "service": "offerpilot-go",
  "live": true,
  "ready": true,
  "readiness": "ready",
  "harness": "ready",
  "modelConfigured": true,
  "speechConfigured": true,
  "knowledgeEntries": 404
}
```

`knowledgeEntries` is an observed value, not a permanent assertion. It changes
when Markdown files change. A missing LLM key makes `/health/ready` return 503
with `harness: not_ready`; interview requests fail closed and do not commit a
fallback assessment.

## Docker Compose

Prepare and edit `.env`, replacing the example API token and provider keys:

```bash
cp .env.example .env
docker compose up --build -d
```

The API image is a multi-stage Go build. The Web image uses Node.js 24. Compose
waits for the Go health check before starting Web traffic.

## Data And Recovery

- The `app-data` volume stores `/app/data/offerpilot.db`.
- Interview writes use optimistic versions, so two answers for the same active
  question cannot both commit.
- The Markdown knowledge index is rebuilt from the mounted/image content at
  startup and exposes the resulting count in health output.
- Do not publish `.env`, SQLite files, private resumes, transcripts, audio, or
  provider error logs.
- To roll back, route only newly created sessions to `serve:legacy`; allow or
  pause existing Go-owned sessions instead of translating state mid-interview.

## Release Validation

```bash
npm run build
npm run test:go
npx vitest run tests/unit tests/e2e
npm --prefix web run build
git diff --check
```

When Docker is available:

```bash
docker build -t offerpilot-api:test .
docker build -f web/Dockerfile -t offerpilot-web:test .
docker compose config
```
