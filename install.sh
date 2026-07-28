#!/usr/bin/env bash
set -euo pipefail

repo_url="${FREEGENT_REPO_URL:-https://github.com/simonbalfe/freegent.git}"
install_dir="${FREEGENT_DIR:-$HOME/freegent}"

host_cli_platform() {
  case "$(uname -s)" in
    Darwin) cli_goos=darwin ;;
    Linux) cli_goos=linux ;;
    *)
      echo "Unsupported operating system: $(uname -s)"
      exit 1
      ;;
  esac

  case "$(uname -m)" in
    arm64 | aarch64) cli_goarch=arm64 ;;
    x86_64 | amd64) cli_goarch=amd64 ;;
    *)
      echo "Unsupported CPU architecture: $(uname -m)"
      exit 1
      ;;
  esac
}

has_env_value() {
  local key="$1"
  awk -v key="$key" '
    {
      line = $0
      sub(/\r$/, "", line)
      if (line !~ "^[[:space:]]*" key "[[:space:]]*=") {
        next
      }
      sub("^[[:space:]]*" key "[[:space:]]*=[[:space:]]*", "", line)
      sub(/[[:space:]]+#.*$/, "", line)
      sub(/^[[:space:]]+/, "", line)
      sub(/[[:space:]]+$/, "", line)
      if ((line ~ /^".*"$/) || (line ~ /^'\''.*'\''$/)) {
        line = substr(line, 2, length(line) - 2)
      }
      if (length(line) > 0) {
        found = 1
      }
    }
    END {
      exit found ? 0 : 1
    }
  ' .env
}

if ! command -v git >/dev/null 2>&1; then
  echo "git is required. Install git and run this script again."
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required. Install Docker Desktop and run this script again."
  exit 1
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required. Install curl and run this script again."
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

if [ -f .env ] && [ "${FREEGENT_REFRESH_KEYS:-0}" != "1" ]; then
  echo "Keeping existing .env"
else
  [ -f .env ] || cp .env.example .env
  echo
  echo "Enter provider keys. Leave a key blank to use the next fallback."
  openrouter_key="${FREEGENT_OPENROUTER_API_KEY:-}"
  serper_key="${FREEGENT_SERPER_API_KEY:-}"
  exa_key="${FREEGENT_EXA_API_KEY:-}"
  tavily_key="${FREEGENT_TAVILY_API_KEY:-}"
  apify_key="${FREEGENT_APIFY_API_TOKEN:-}"
  if [ "${FREEGENT_NONINTERACTIVE:-0}" != "1" ] && [ -r /dev/tty ]; then
    [ -n "$openrouter_key" ] || read -r -p "OpenRouter API key: " openrouter_key </dev/tty
    [ -n "$serper_key" ] || read -r -p "Serper API key (recommended): " serper_key </dev/tty
    [ -n "$exa_key" ] || read -r -p "Exa API key: " exa_key </dev/tty
    [ -n "$tavily_key" ] || read -r -p "Tavily API key: " tavily_key </dev/tty
    [ -n "$apify_key" ] || read -r -p "Apify API token (optional): " apify_key </dev/tty
  fi

  {
    printf 'OPENROUTER_API_KEY=%s\n' "$openrouter_key"
    printf 'SERPER_API_KEY=%s\n' "$serper_key"
    printf 'EXA_API_KEY=%s\n' "$exa_key"
    printf 'TAVILY_API_KEY=%s\n' "$tavily_key"
    printf 'APIFY_API_TOKEN=%s\n' "$apify_key"
  } > .env
  chmod 600 .env
fi

if ! has_env_value OPENROUTER_API_KEY; then
  echo "OPENROUTER_API_KEY is required. Run again and provide it."
  exit 1
fi
if ! has_env_value SERPER_API_KEY \
  && ! has_env_value EXA_API_KEY \
  && ! has_env_value TAVILY_API_KEY; then
  echo "At least one search key is required: SERPER_API_KEY, EXA_API_KEY, or TAVILY_API_KEY."
  exit 1
fi

echo "Pulling and starting prebuilt Freegent images"
if ! docker compose pull; then
  echo "Prebuilt images could not be pulled. Building locally instead."
  docker compose build
fi
docker compose up -d --force-recreate

host_cli_platform
if [ -w /usr/local/bin ]; then
  cli_path=/usr/local/bin/freegent
else
  mkdir -p "$HOME/.local/bin"
  cli_path="$HOME/.local/bin/freegent"
fi
cli_build_dir="$(mktemp -d)"
cleanup_cli_build() {
  rm -rf "$cli_build_dir"
}
trap cleanup_cli_build EXIT
echo "Building native CLI for $cli_goos/$cli_goarch"
docker build \
  --target cli-artifact \
  --build-arg CLI_GOOS="$cli_goos" \
  --build-arg CLI_GOARCH="$cli_goarch" \
  --output "type=local,dest=$cli_build_dir" \
  .
install -m 0755 "$cli_build_dir/freegent" "$cli_path"

mkdir -p "$HOME/.codex/skills/freegent" "$HOME/.claude/skills/freegent"
cp SKILL.md "$HOME/.codex/skills/freegent/SKILL.md"
cp SKILL.md "$HOME/.claude/skills/freegent/SKILL.md"

echo "Waiting for services"
for attempt in $(seq 1 30); do
  running_services="$(docker compose ps --status running --services)"
  if curl -fsS http://localhost:8080/health >/dev/null 2>&1 \
    && curl -fsS http://localhost:8081/healthz >/dev/null 2>&1 \
    && printf '%s\n' "$running_services" | grep -qx postgres \
    && printf '%s\n' "$running_services" | grep -qx api \
    && printf '%s\n' "$running_services" | grep -qx worker \
    && printf '%s\n' "$running_services" | grep -qx openextract \
    && docker compose exec -T worker sh -c 'test -n "$OPENROUTER_API_KEY"' \
    && "$cli_path" --help >/dev/null 2>&1; then
    echo
    echo "Freegent is ready: http://localhost:8080/dashboard"
    echo "CLI installed at $cli_path"
    if [[ ":$PATH:" != *":$(dirname "$cli_path"):"* ]]; then
      echo "Add $(dirname "$cli_path") to PATH to call freegent from any shell."
    fi
    echo "Codex and Claude skill installed."
    echo "CLI usage: freegent --help"
    exit 0
  fi
  sleep 2
done

echo "Services did not become healthy. Check:"
echo "  cd $install_dir && docker compose logs --tail=100"
exit 1
