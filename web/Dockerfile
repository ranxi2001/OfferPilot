FROM node:22-slim AS deps
WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci --omit=dev

FROM node:22-slim AS build
WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
COPY models.yml /app/models.yml
RUN npm run build

FROM node:22-slim AS runtime
WORKDIR /app/web

ENV NODE_ENV=production
ENV PORT=3000
ENV OFFERPILOT_PROJECT_ROOT=/app

COPY --from=deps /app/web/node_modules ./node_modules
COPY --from=build /app/web/.next ./.next
COPY --from=build /app/web/public ./public
COPY --from=build /app/web/package.json ./package.json
COPY --from=build /app/web/next.config.mjs ./next.config.mjs
COPY --from=build /app/models.yml /app/models.yml

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD node -e "fetch('http://127.0.0.1:' + (process.env.PORT || '3000') + '/api/health').then((r) => process.exit(r.ok ? 0 : 1)).catch(() => process.exit(1))"

CMD ["npm", "run", "start"]
