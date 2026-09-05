# devpass-usage

<p align="center">
  <img src="docs/demo.gif" alt="devpass-usage demo" width="720">
</p>

DevPass (by LLM Gateway) usage, spend, and credits for AI agents — packaged as
a Dockerized [MCP](https://modelcontextprotocol.io) server for
[Crush](https://github.com/charmbracelet/crush), Claude Code, Cursor, and any
other MCP client. Also works as a standalone CLI.

Ask your agent *how much of my DevPass quota is left?* and it calls the
`show_usage` tool: live data straight from the DevPass dashboard API, so
answers are never stale.

## The tool

`show_usage` takes one optional argument and returns plan credits (monthly and
premium weekly), spend and request counts, and a per-model cost breakdown.

| argument | values | default |
| --- | --- | --- |
| `range` | `24h`, `7d`, `30d` | `7d` |

Data is fetched live on every call. Only the auth session token is cached.

## Quick start (Crush)

```sh
docker pull hangarbay/devpass.mcp:latest
```

Add to `~/.config/crush/crushrc`:

```bash
CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/devpass-usage"
[ "$(uname)" = Darwin ] && CACHE="$HOME/Library/Caches/devpass-usage"
dargs=(run --rm -i --user "$(id -u):$(id -g)"
  --env LLM_GATEWAY_SESSION_TOKEN "${LLM_GATEWAY_SESSION_TOKEN:-}"
  --env LLM_GATEWAY_EMAIL "${LLM_GATEWAY_EMAIL:-}"
  --env LLM_GATEWAY_PASSWORD "${LLM_GATEWAY_PASSWORD:-}"
  --env XDG_CACHE_HOME /cache
  --volume "$CACHE:/cache/devpass-usage"
  hangarbay/devpass.mcp:latest)
args=()
for a in "${dargs[@]}"; do args+=(--args "$a"); done
mcp add devpass-usage --command docker "${args[@]}"
```

Then just ask:

> how much of my DevPass quota is left?

## Other MCP clients

For `claude_desktop_config.json`, `.mcp.json`, or `crush.json`:

```json
{
  "mcpServers": {
    "devpass-usage": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-e", "LLM_GATEWAY_SESSION_TOKEN",
        "-e", "LLM_GATEWAY_EMAIL",
        "-e", "LLM_GATEWAY_PASSWORD",
        "hangarbay/devpass.mcp:latest"
      ]
    }
  }
}
```

## Credentials

Set one of the following in your shell profile (the container inherits them via
the `-e` flags):

| Variable | Purpose |
| --- | --- |
| `LLM_GATEWAY_EMAIL` / `LLM_GATEWAY_PASSWORD` | sign-in credentials |
| `LLM_GATEWAY_SESSION_TOKEN` | 30-day session token (takes precedence) |
| `LLM_GATEWAY_BASE_URL` | override API base (default `https://internal.llmgateway.io`) |

With email/password the server signs itself in and refreshes the session token
automatically. The cache mount in the Crush snippet shares the host's session
cache (`~/Library/Caches/devpass-usage` on macOS, `~/.cache/devpass-usage` on
Linux), so sign-in happens at most once per ~30 days; without it the container
re-signs-in on every start.

## Standalone CLI (no Docker)

```sh
go install github.com/hangarbay/devpass.mcp@latest
devpass-usage show --range 7d   # same data, pretty-printed
```

`devpass-usage serve` runs the same MCP server over stdio without Docker.

Or use the installer, which downloads a prebuilt binary from GitHub Releases
(macOS and Linux, amd64/arm64) and verifies its checksum:

```sh
curl -fsSL https://raw.githubusercontent.com/hangarbay/devpass.mcp/main/install.sh | bash
```

## Building the image

```sh
make docker
```

The release workflow publishes `hangarbay/devpass.mcp` to Docker Hub
(multi-arch: linux/amd64, linux/arm64) on every `v*` tag.

## License

[MIT](LICENSE.md)
