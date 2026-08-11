FROM node:24-slim AS deps
WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci --omit=dev

FROM node:24-slim AS build
WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
COPY models.yml /app/models.yml
RUN npm run build

FROM node:24-slim AS runtime
WORKDIR /app/web

ENV NODE_ENV=production
ENV PORT=3000
ENV OFFERPILOT_PROJECT_ROOT=/app
ENV OFFERPILOT_CONFIG_PATH=/app/config/.env

COPY --chown=node:node --from=deps /app/web/node_modules ./node_modules
COPY --chown=node:node --from=build /app/web/.next ./.next
COPY --chown=node:node --from=build /app/web/public ./public
COPY --chown=node:node --from=build /app/web/package.json ./package.json
COPY --chown=node:node --from=build /app/web/next.config.mjs ./next.config.mjs
COPY --chown=node:node --from=build /app/models.yml /app/models.yml

RUN mkdir -p /app/config && chown node:node /app/config

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD node -e "fetch('http://127.0.0.1:' + (process.env.PORT || '3000') + '/api/health').then((r) => process.exit(r.ok ? 0 : 1)).catch(() => process.exit(1))"

USER node
CMD ["npm", "run", "start"]
