FROM golang:1.26-bookworm AS build
WORKDIR /src/backend

COPY backend/go.* ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/offerpilot-api ./cmd/offerpilot-api

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system offerpilot \
    && useradd --system --gid offerpilot --home-dir /app offerpilot

WORKDIR /app
COPY --from=build /out/offerpilot-api ./offerpilot-api
COPY knowledge/ ./knowledge/

RUN mkdir -p /app/data /app/config \
    && chown -R offerpilot:offerpilot /app \
    && chown 1000:1000 /app/config
USER offerpilot

ENV PORT=3001
ENV KNOWLEDGE_DIR=/app/knowledge
ENV DB_PATH=/app/data/offerpilot.db
EXPOSE 3001

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl --fail --silent --show-error http://127.0.0.1:3001/health/ready || exit 1

CMD ["./offerpilot-api"]
