FROM oven/bun:1-slim AS typecheck

WORKDIR /srv

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY tsconfig.json *.ts ./
RUN bun run typecheck

FROM oven/bun:1-slim

USER root
WORKDIR /srv

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile --production --backend=copyfile \
    && bunx patchright install --with-deps --no-shell --no-progress chromium \
    && apt-get update \
    && apt-get install -y --no-install-recommends xauth ca-certificates \
    && find /root/.cache/ms-playwright -maxdepth 1 -type d -name 'ffmpeg-*' -exec rm -rf {} + \
    && rm -rf /root/.bun/install/cache /var/lib/apt/lists/*

COPY --from=typecheck /srv/*.ts ./

ENV PORT=8081

EXPOSE 8081

HEALTHCHECK --interval=5s --timeout=3s --start-period=10s --retries=30 \
  CMD bun -e "const response = await fetch('http://localhost:8081/healthz'); process.exit(response.ok ? 0 : 1)"

CMD ["xvfb-run", "-a", "--server-args=-screen 0 1920x1080x24", "bun", "run", "server.ts"]
