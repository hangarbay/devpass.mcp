# devpass-usage.skill

<p align="center">
  <img src="docs/demo.gif" alt="devpass-usage demo" width="720">
</p>

DevPass (by LLM Gateway) usage, spend, and credits from the terminal, packaged
as an agent skill for [Crush](https://github.com/charmbracelet/crush),
[Claude Code](https://claude.com/claude-code), and any other agent that reads
`SKILL.md` files.

Ask your agent things like *how much have I spent this week?* and it will run
the CLI and interpret the numbers. The CLI also works standalone.

## What you get

- `devpass-usage show --range 24h|7d|30d` — plan credits, spend, per-model
  breakdown, always fetched live from the API (no usage caching)

Data comes straight from the DevPass dashboard API (`internal.llmgateway.io`)
on every call. Only the auth session token is cached
(`~/.cache/devpass-usage/session.json`).

## Install

No toolchain needed — the installer downloads a prebuilt binary from GitHub
Releases (macOS and Linux, amd64/arm64), verifies its checksum, and installs
the skill for both agents:

```sh
curl -fsSL https://raw.githubusercontent.com/hangarbay/devpass-usage.skill/main/install.sh | bash
```

Or from a clone (also enables building from source with `./install.sh --build`):

```sh
git clone https://github.com/hangarbay/devpass-usage.skill
cd devpass-usage.skill
./install.sh          # pin a version: ./install.sh v0.1.0
```

This builds the CLI to `~/.local/bin/devpass-usage` and installs the skill to:

- `~/.config/crush/skills/devpass-usage` (Crush)
- `~/.claude/skills/devpass-usage` (Claude Code)

Binary only (requires Go, installs to `$GOPATH/bin`):

```sh
go install github.com/hangarbay/devpass-usage.skill@latest
```

## Credentials

Set one of the following in your shell profile:

```sh
export LLM_GATEWAY_EMAIL="you@example.com"
export LLM_GATEWAY_PASSWORD="..."
# or a 30-day session token from the dashboard
export LLM_GATEWAY_SESSION_TOKEN="..."
```

With email/password the CLI signs itself in and refreshes the session token
automatically (cached with `0600` permissions).

| Variable | Purpose |
| --- | --- |
| `LLM_GATEWAY_EMAIL` / `LLM_GATEWAY_PASSWORD` | sign-in credentials |
| `LLM_GATEWAY_SESSION_TOKEN` | existing session token (takes precedence) |
| `LLM_GATEWAY_BASE_URL` | override API base (default `https://internal.llmgateway.io`) |

## Using with Crush

Once installed, the skill appears as `user:devpass-usage`. Just ask:

> how much of my DevPass quota is left?

## License

[MIT](LICENSE.md)
