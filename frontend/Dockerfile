FROM node:22-alpine AS build

RUN corepack enable && corepack prepare pnpm@latest --activate

WORKDIR /app/frontend

COPY VERSION /app/VERSION
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

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
