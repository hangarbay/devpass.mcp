---
name: devpass-usage
description: Show LLM Gateway DevPass usage, spend, and credits. Use when the user asks about their usage, cost, spend, credits, quota, or DevPass plan.
user-invocable: true
---

Run `~/.local/bin/devpass-usage show --range <R>` where R is `24h`, `7d`, or `30d` (default `7d`; pick based on what the user asked about). Present the output as-is in a code block, then answer the user's question using the numbers (credits remaining, 24h/30d spend, per-model breakdown).
