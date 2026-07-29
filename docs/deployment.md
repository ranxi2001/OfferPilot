# OfferPilot Server-Backed Deployment

OfferPilot's supported production path is the server-backed architecture:

```text
Browser -> Next.js Web -> Node API -> LLM / ASR / TTS providers
                         |
                         v
                      SQLite
```

Browser-direct BYOK mode is tracked separately in
[OfferPilot#1](https://github.com/ranxi2001/OfferPilot/issues/1) and is not a
release blocker for the current deployment path.

## Runtime Contract

Required for production:

- `OFFERPILOT_API_KEY`: shared bearer token used by the Web service when it
  calls the API service. Change the example value before deployment.
- `OFFERPILOT_ALLOWED_ORIGINS`: comma-separated browser origins allowed to call
  the API service, for example `https://offerpilot.example.com`.
- At least one text provider key, such as `OPENAI_API_KEY`,
  `ANTHROPIC_API_KEY`, or `DEEPSEEK_API_KEY`.
- `DB_PATH`: SQLite database path. In Docker Compose this is
  `/app/data/agent.db`.

Optional:

- `MIMO_API_KEY`, `MIMO_BASE_URL`, `MIMO_ASR_MODEL`, `MIMO_TTS_MODEL`: enable
  audio transcription and speech synthesis.
- `OFFERPILOT_SEED_KNOWLEDGE_ON_START`: defaults to `true`. On first startup,
  the API service seeds the SQLite knowledge table when it is empty.
- `KNOWLEDGE_DIR`: markdown knowledge directory. In the API image this is
  `/app/knowledge`.
- `OFFERPILOT_ENABLE_CONFIG_API`: defaults to `false` in Compose. Keep it off
  in production unless the deployment is private and authenticated.
- `OFFERPILOT_HEALTH_TIMEOUT_MS`: timeout used by the Web health route when it
  checks the API service.

## Docker Compose

Prepare `.env`:

```bash
cp .env.example .env
```

Edit `.env`:

- Replace `OFFERPILOT_API_KEY=change-me-in-production`.
- Set `OFFERPILOT_ALLOWED_ORIGINS` to the real Web origin.
- Fill in provider keys needed by your deployment.

Start both services:

```bash
docker compose up --build -d
```

Check health:

```bash
curl http://localhost:3001/health
curl http://localhost:3000/api/health
```

Expected API response:

```json
{"status":"ok"}
```

The Web health endpoint returns `200` only when the Web service is running and
the API health check succeeds.

## Data And Secrets

- SQLite state lives in the `app-data` Docker volume.
- The API service seeds the markdown knowledge base only when the database is
  empty, so repeated restarts do not duplicate seeded records.
- Provider keys stay in server environment variables. The browser never needs
  direct model-provider credentials in this deployment mode.
- Do not publish `.env`, generated SQLite databases, private resumes, audio,
  transcripts, or logs containing provider errors with credentials.

## Local Validation

Run these checks before shipping deployment changes:

```bash
npm run build
npx vitest run tests/unit tests/e2e
npm --prefix web run build
git diff --check
```

When Docker is available, also run:

```bash
docker build -t offerpilot-api:test .
docker build -f web/Dockerfile -t offerpilot-web:test .
docker compose config
```
