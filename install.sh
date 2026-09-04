#!/usr/bin/env bash
# Install devpass-usage: builds the CLI and installs the skill for
# SKILL.md-compatible agents (Crush, Claude Code).
set -euo pipefail
cd "$(dirname "$0")"

command -v go >/dev/null 2>&1 || {
  echo "error: Go is required to build the CLI: https://go.dev/dl" >&2
  exit 1
}

echo "==> Building devpass-usage"
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/devpass-usage" .

echo "==> Installing skills"
for dest in "$HOME/.config/crush/skills" "$HOME/.claude/skills"; do
  mkdir -p "$dest"
  rm -rf "$dest/devpass-usage"
  cp -R skills/devpass-usage "$dest/devpass-usage"
  echo "    installed -> $dest/devpass-usage"
done

case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) echo "note: $HOME/.local/bin is not on your PATH" ;;
esac

echo "==> Checking credentials"
if [[ -n "${LLM_GATEWAY_SESSION_TOKEN:-}" ]]; then
  echo "    using LLM_GATEWAY_SESSION_TOKEN"
elif [[ -n "${LLM_GATEWAY_EMAIL:-}" && -n "${LLM_GATEWAY_PASSWORD:-}" ]]; then
  echo "    using LLM_GATEWAY_EMAIL + LLM_GATEWAY_PASSWORD"
else
  cat <<'EOF'
    No DevPass credentials found. Add ONE of the following to your shell
    profile (~/.zshrc, ~/.bashrc, or ~/.crush_env):

      export LLM_GATEWAY_EMAIL="you@example.com"
      export LLM_GATEWAY_PASSWORD="..."

      # or a 30-day session token from the dashboard
      export LLM_GATEWAY_SESSION_TOKEN="..."
EOF
fi

echo "==> Done. Try: devpass-usage show --range 7d"
