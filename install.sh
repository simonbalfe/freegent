#!/usr/bin/env bash
set -euo pipefail

install_dir="${FREEGENT_DIR:-$HOME/freegent}"
api_image="ghcr.io/simonbalfe/freegent-api:${FREEGENT_IMAGE_TAG:-latest}"
api_url="http://localhost:${FREEGENT_PORT:-8080}"
openextract_url="http://localhost:${OPENEXTRACT_PORT:-8081}"

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
if ! docker info >/dev/null 2>&1; then
  echo "Docker is installed but not running. Open Docker Desktop and run this command again."
  exit 1
fi

host_cli_platform
if [ -w /usr/local/bin ]; then
  cli_path=/usr/local/bin/freegent
else
  mkdir -p "$HOME/.local/bin"
  cli_path="$HOME/.local/bin/freegent"
  if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
    case "${SHELL:-}" in
      */zsh) shell_profile="$HOME/.zprofile" ;;
      *) shell_profile="$HOME/.profile" ;;
    esac
    path_line='export PATH="$HOME/.local/bin:$PATH"'
    if ! grep -Fqx "$path_line" "$shell_profile" 2>/dev/null; then
      printf '\n%s\n' "$path_line" >> "$shell_profile"
    fi
  fi
fi

echo "Pulling Freegent install bundle"
docker pull "$api_image"
bundle_dir="$(mktemp -d)"
bundle_container="$(docker create "$api_image" true)"
cleanup_bundle() {
  docker rm "$bundle_container" >/dev/null 2>&1 || true
  rm -rf "$bundle_dir"
}
trap cleanup_bundle EXIT
docker cp "$bundle_container:/opt/freegent/install/." "$bundle_dir"
docker cp "$bundle_container:/opt/freegent/$cli_goos/freegent" "$bundle_dir/freegent"
mkdir -p "$install_dir"
install -m 0644 "$bundle_dir/compose.yaml" "$install_dir/compose.yaml"
install -m 0644 "$bundle_dir/.env.example" "$install_dir/.env.example"
install -m 0644 "$bundle_dir/SKILL.md" "$install_dir/SKILL.md"
install -m 0755 "$bundle_dir/freegent" "$cli_path"

cd "$install_dir"

if [ -f .env ] \
  && [ "${FREEGENT_REFRESH_KEYS:-0}" != "1" ] \
  && has_env_value OPENROUTER_API_KEY \
  && { has_env_value SERPER_API_KEY \
    || has_env_value EXA_API_KEY \
    || has_env_value TAVILY_API_KEY; }; then
  echo "Keeping existing .env"
else
  echo
  echo "Enter an OpenRouter key and one search key. Input is hidden."
  openrouter_key="${FREEGENT_OPENROUTER_API_KEY:-}"
  serper_key="${FREEGENT_SERPER_API_KEY:-}"
  exa_key="${FREEGENT_EXA_API_KEY:-}"
  tavily_key="${FREEGENT_TAVILY_API_KEY:-}"
  apify_key="${FREEGENT_APIFY_API_TOKEN:-}"
  if [ "${FREEGENT_NONINTERACTIVE:-0}" != "1" ] && [ -r /dev/tty ]; then
    if [ -z "$openrouter_key" ]; then
      read -r -s -p "OpenRouter API key: " openrouter_key </dev/tty
      printf '\n' >/dev/tty
    fi
    if [ -z "$serper_key$exa_key$tavily_key" ]; then
      read -r -s -p "Serper API key (press Enter to use Exa or Tavily): " serper_key </dev/tty
      printf '\n' >/dev/tty
    fi
    if [ -z "$serper_key$exa_key$tavily_key" ]; then
      read -r -s -p "Exa API key (press Enter to use Tavily): " exa_key </dev/tty
      printf '\n' >/dev/tty
    fi
    if [ -z "$serper_key$exa_key$tavily_key" ]; then
      read -r -s -p "Tavily API key: " tavily_key </dev/tty
      printf '\n' >/dev/tty
    fi
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
docker compose pull
docker compose up -d --force-recreate

mkdir -p "$HOME/.codex/skills/freegent" "$HOME/.claude/skills/freegent"
cp SKILL.md "$HOME/.codex/skills/freegent/SKILL.md"
cp SKILL.md "$HOME/.claude/skills/freegent/SKILL.md"

echo "Waiting for services"
for attempt in $(seq 1 30); do
  running_services="$(docker compose ps --status running --services)"
  if curl -fsS "$api_url/health" >/dev/null 2>&1 \
    && curl -fsS "$openextract_url/healthz" >/dev/null 2>&1 \
    && printf '%s\n' "$running_services" | grep -qx postgres \
    && printf '%s\n' "$running_services" | grep -qx api \
    && printf '%s\n' "$running_services" | grep -qx worker \
    && printf '%s\n' "$running_services" | grep -qx openextract \
    && docker compose exec -T worker sh -c 'test -n "$OPENROUTER_API_KEY"' \
    && "$cli_path" --help >/dev/null 2>&1; then
    echo
    echo "Freegent is ready: $api_url/dashboard"
    echo "CLI installed at $cli_path"
    if [[ ":$PATH:" != *":$(dirname "$cli_path"):"* ]]; then
      echo "CLI PATH saved for new shells. Run: export PATH=\"$(dirname "$cli_path"):\$PATH\""
    fi
    echo "Codex and Claude skill installed."
    echo "CLI usage: freegent --help"
    if [ "${FREEGENT_NO_OPEN:-0}" != "1" ]; then
      if command -v open >/dev/null 2>&1; then
        open "$api_url/dashboard" >/dev/null 2>&1 || true
      elif command -v xdg-open >/dev/null 2>&1; then
        xdg-open "$api_url/dashboard" >/dev/null 2>&1 || true
      fi
    fi
    exit 0
  fi
  sleep 2
done

echo "Services did not become healthy. Check:"
echo "  cd $install_dir && docker compose logs --tail=100"
exit 1
