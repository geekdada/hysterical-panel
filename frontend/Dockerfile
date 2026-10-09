# .output is plain JS, so build once on the host arch instead of under QEMU.
FROM --platform=$BUILDPLATFORM node:22-alpine AS build

ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0

WORKDIR /app

COPY VERSION package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json ./frontend/
RUN corepack enable && corepack install
RUN pnpm install --frozen-lockfile --filter hysterical-panel-frontend

WORKDIR /app/frontend

COPY frontend/ ./
COPY backend/openapi.json /app/backend/

ARG VITE_API_BASE_URL=
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL

RUN pnpm api:types && pnpm build

FROM node:22-alpine AS run

RUN apk add --no-cache wget \
    && addgroup -S panel \
    && adduser -S -D -H -h /app -s /sbin/nologin -G panel panel

WORKDIR /app

COPY --from=build --chown=panel:panel /app/frontend/.output ./.output

ENV NODE_ENV=production
ENV PORT=3000
ENV HOST=0.0.0.0

EXPOSE 3000

USER panel

CMD ["node", ".output/server/index.mjs"]
