# Introduction

deploq is a lightweight, single-binary webhook deploy tool for Docker Compose projects.

It receives GitHub webhooks (or generic HTTP calls from CI), pulls the latest code, and runs `docker compose build && docker compose up -d`.

## Features

- **Zero dependencies** — single Go binary, no runtime requirements
- **GitHub & Generic webhooks** — HMAC-SHA256 or token-based verification
- **Event filtering** — trigger on push, release, or both
- **CI status checks** — wait for GitHub CI to pass before deploying
- **Failure hooks** — run shell commands on deploy failure (Slack, email, etc.)
- **Deploy status** — REST API to check last deploy result
- **Concurrent safety** — per-project locking, duplicate SHA detection
- **Graceful shutdown** — waits for active deploys to complete

## How It Works

```
webhook received
  → verify signature
  → check event type filter
  → check branch filter
  → acquire project lock
  → wait for CI (if enabled)
  → git fetch + reset
  → docker compose build
  → docker compose up -d
```
