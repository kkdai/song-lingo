# One container: the Go API (backend/) on localhost and the Next.js front end that forwards /api/* to it.
# Song data lives in a Cloud Storage volume mounted at SONG_DATA_DIR.

FROM golang:1.26 AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/song-lingo ./cmd/song-lingo

FROM node:22-slim AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# BACKEND_URL is baked into the /api rewrite at build time; the default matches BACKEND_PORT below.
RUN mkdir -p public && npm run build

FROM node:22-slim
# Go reads the system CA bundle for HTTPS (Node ships its own).
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=backend /out/song-lingo /usr/local/bin/song-lingo
WORKDIR /app
COPY config/ config/
COPY --from=web /app/web/.next/standalone web/
COPY --from=web /app/web/.next/static web/.next/static
COPY --from=web /app/web/public web/public
COPY --from=web /app/web/scripts web/scripts
WORKDIR /app/web

ENV NODE_ENV=production \
    HOSTNAME=0.0.0.0 \
    SONG_LINGO_ROOT=/app \
    SONG_DATA_DIR=/data \
    BACKEND_PORT=8081 \
    NEXT_TELEMETRY_DISABLED=1
CMD ["node", "scripts/start.mjs"]
