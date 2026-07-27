#!/usr/bin/env bash
set -euo pipefail

repo_url="${FREEGENT_REPO_URL:-https://github.com/simonbalfe/freegent.git}"
install_dir="${FREEGENT_DIR:-$HOME/freegent}"

if ! command -v git >/dev/null 2>&1; then
  echo "git is required. Install git and run this script again."
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required. Install Docker Desktop and run this script again."
  exit 1
fi
if ! command -v go >/dev/null 2>&1; then
  echo "Go is required to install the freegent CLI. Install Go and run this script again."
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose is required. Update Docker Desktop and run this script again."
  exit 1
fi

if [ -d "$install_dir/.git" ]; then
  echo "Updating $install_dir"
  git -C "$install_dir" pull --ff-only
else
  echo "Cloning Freegent to $install_dir"
  git clone "$repo_url" "$install_dir"
fi

cd "$install_dir"

if [ -f .env ]; then
  echo "Keeping existing .env"
else
  cp .env.example .env
  echo
  echo "Enter provider keys. Leave a key blank to use the next fallback."
  read -r -p "OpenRouter API key: " openrouter_key
  read -r -p "Serper API key (recommended): " serper_key
  read -r -p "Exa API key: " exa_key
  read -r -p "Tavily API key: " tavily_key
  read -r -p "Apify API token (optional): " apify_key

  {
    printf 'OPENROUTER_API_KEY=%s\n' "$openrouter_key"
    printf 'SERPER_API_KEY=%s\n' "$serper_key"
    printf 'EXA_API_KEY=%s\n' "$exa_key"
    printf 'TAVILY_API_KEY=%s\n' "$tavily_key"
    printf 'APIFY_API_TOKEN=%s\n' "$apify_key"
  } > .env
  chmod 600 .env
fi

echo "Building and starting Freegent"
docker compose up -d --build

mkdir -p "$HOME/.local/bin"
go build -trimpath -o "$HOME/.local/bin/freegent" ./cmd/openclaygent-go

echo "Waiting for services"
for attempt in $(seq 1 30); do
  if curl -fsS http://localhost:8080/health >/dev/null 2>&1 && curl -fsS http://localhost:8081/healthz >/dev/null 2>&1; then
    echo
    echo "Freegent is ready: http://localhost:8080/dashboard"
    echo "CLI installed at $HOME/.local/bin/freegent"
    echo "CLI usage: freegent --help"
    exit 0
  fi
  sleep 2
done

echo "Services did not become healthy. Check:"
echo "  cd $install_dir && docker compose logs --tail=100"
exit 1
