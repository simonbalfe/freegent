FROM oven/bun:1-slim

USER root

RUN apt-get update \
    && apt-get install -y --no-install-recommends xvfb xauth ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /srv

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
RUN bunx patchright install --with-deps chromium

COPY tsconfig.json *.ts ./
RUN bun run typecheck

ENV PORT=8081

EXPOSE 8081

HEALTHCHECK --interval=5s --timeout=3s --start-period=10s --retries=30 \
  CMD bun -e "const response = await fetch('http://localhost:8081/healthz'); process.exit(response.ok ? 0 : 1)"

CMD ["xvfb-run", "-a", "--server-args=-screen 0 1920x1080x24", "bun", "run", "server.ts"]
