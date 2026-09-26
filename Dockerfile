# Next.js web app + the Python pipeline it shells out to (transcribe.py / annotate.py via `uv run`).
# Song data lives in a Cloud Storage volume mounted at SONG_DATA_DIR.
FROM node:22-slim

COPY --from=ghcr.io/astral-sh/uv:0.8 /uv /uvx /usr/local/bin/
ENV UV_PYTHON_INSTALL_DIR=/opt/uv-python \
    UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy

WORKDIR /app
COPY pyproject.toml uv.lock .python-version ./
RUN uv sync --frozen --no-dev --no-install-project
# The venv is complete; don't let `uv run` re-resolve at runtime.
ENV UV_NO_SYNC=1

COPY web/package.json web/package-lock.json web/
RUN cd web && npm ci

COPY . .
RUN cd web && npm run build

ENV NODE_ENV=production \
    SONG_DATA_DIR=/data \
    NEXT_TELEMETRY_DISABLED=1
WORKDIR /app/web
# next start listens on $PORT (Cloud Run sets it).
CMD ["node_modules/.bin/next", "start"]
