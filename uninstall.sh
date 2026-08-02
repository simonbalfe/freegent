#!/usr/bin/env bash
set -euo pipefail

install_dir="${FREEGENT_DIR:-$HOME/freegent}"
api_image="ghcr.io/simonbalfe/freegent-api:${FREEGENT_IMAGE_TAG:-latest}"

if command -v docker >/dev/null 2>&1; then
  if ! docker info >/dev/null 2>&1; then
    echo "Docker is not running. Open Docker Desktop and run this command again."
    exit 1
  fi
  if [ -f "$install_dir/compose.yaml" ]; then
    echo "Removing Freegent containers, volumes, and networks"
    docker compose -f "$install_dir/compose.yaml" down --volumes --remove-orphans
    docker image rm "$api_image" >/dev/null 2>&1 || true
  fi
fi

rm -f /usr/local/bin/freegent "$HOME/.local/bin/freegent"
rm -rf "$HOME/.codex/skills/freegent" "$HOME/.claude/skills/freegent"
if [ "$install_dir" = "$HOME/freegent" ] && [ -d "$install_dir" ]; then
  find "$install_dir" -mindepth 1 -maxdepth 1 ! -name .env -exec rm -rf -- {} +
else
  rm -f \
    "$install_dir/compose.yaml" \
    "$install_dir/.env.example" \
    "$install_dir/SKILL.md" \
    "$install_dir/uninstall.sh"
fi

echo "Freegent was removed. Credentials remain in $install_dir/.env"
