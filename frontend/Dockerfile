FROM node:24.21.0-alpine AS base
ENV NEXT_TELEMETRY_DISABLED=1
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@12.15.0 --activate

FROM base AS deps
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile --ignore-scripts

FROM base AS builder
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ARG NEXT_PUBLIC_AMAZON_LOCATION_REGION=
ARG NEXT_PUBLIC_AMAZON_LOCATION_MAP_KEY=
ARG NEXT_PUBLIC_MAP_STYLE_URL=
ENV NEXT_PUBLIC_AMAZON_LOCATION_REGION=${NEXT_PUBLIC_AMAZON_LOCATION_REGION} \
    NEXT_PUBLIC_AMAZON_LOCATION_MAP_KEY=${NEXT_PUBLIC_AMAZON_LOCATION_MAP_KEY} \
    NEXT_PUBLIC_MAP_STYLE_URL=${NEXT_PUBLIC_MAP_STYLE_URL}
RUN pnpm build

FROM node:24.21.0-alpine AS runner
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=4000 \
    HOSTNAME=0.0.0.0
WORKDIR /app

RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs

COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public

RUN mkdir -p /app/.next/cache && chown nextjs:nodejs /app/.next/cache
VOLUME ["/tmp", "/app/.next/cache"]

USER nextjs
EXPOSE 4000

HEALTHCHECK --interval=10s --timeout=3s --retries=5 --start-period=10s \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:4000/api/health || exit 1

CMD ["node", "server.js"]
