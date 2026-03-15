<p align="center">
  <img src="docs/images/hero.jpg" height="120" alt="deploq" />
</p>
<p align="center">Lightweight webhook deploy tool for Docker Compose. Single binary, zero dependencies.</p>
<p align="center">
  <a href="https://github.com/us/deploq/releases"><img src="https://img.shields.io/github/v/release/us/deploq" alt="Release"></a>
  <a href="https://github.com/us/deploq/actions"><img src="https://img.shields.io/github/actions/workflow/status/us/deploq/ci.yml?branch=main" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/us/deploq" alt="License"></a>
  <a href="https://github.com/us/deploq/stargazers"><img src="https://img.shields.io/github/stars/us/deploq" alt="Stars"></a>
</p>
<p align="center">
  <a href="#quick-start">Quick Start</a> •
  <a href="docs/">Documentation</a> •
  <a href="CHANGELOG.md">Changelog</a> •
  <a href="README.zh-CN.md">中文</a>
</p>

## What's New

### v0.0.3
- Event type filtering (`trigger: [push, release]`) with ping support
- Deploy failure handling: `/status/{project}` endpoint + `on_failure` shell hook
- CI status checks: wait for GitHub commit status before deploying
- Input validation and safety improvements

### v0.0.2
- Multi-platform binary releases (linux/darwin, amd64/arm64)

[Full changelog →](CHANGELOG.md)

## Why deploq?

Most deploy tools are either too complex (Kubernetes, Ansible) or too fragile (bare shell scripts). deploq sits in the sweet spot: a single binary that receives webhooks and runs `docker compose up`. No agents, no YAML templating engines, no cluster management.

- **One binary** — download, configure, run
- **Config-driven** — YAML with env var interpolation
- **Secure by default** — HMAC-SHA256 verification, secret validation, input sanitization
- **Production-ready** — graceful shutdown, deploy locking, failure hooks

## Features

- **GitHub & Generic webhooks** — HMAC-SHA256 or token-based verification
- **Event filtering** — trigger on `push`, `release`, or both
- **CI status checks** — wait for GitHub CI to pass before deploying
- **Failure hooks** — run shell commands on deploy failure (Slack, email, etc.)
- **Deploy status API** — `/status/{project}` returns last deploy result
- **Concurrent safety** — per-project locking, duplicate SHA detection
- **Graceful shutdown** — waits for active deploys with timeout

## Quick Start

```bash
# Install
curl -L https://github.com/us/deploq/releases/latest/download/deploq-linux-amd64 -o deploq
chmod +x deploq && sudo mv deploq /usr/local/bin/

# Or with Go
go install github.com/us/deploq/cmd/deploq@latest

# Generate config & start
deploq init
export DEPLOQ_SECRET_MY_APP="your-secret-here-min-16-chars"
deploq validate
deploq serve
```

## Configuration

```yaml
listen: ":9090"

projects:
  backend:
    path: /home/deploy/backend
    branch: main
    secret: "${DEPLOQ_SECRET_BACKEND}"
    compose_file: docker-compose.prod.yml  # default: docker-compose.yml
    deploy_timeout: 15m                     # default: 15m
    trigger: [push, release]               # default: [push]
    on_failure: "curl -s -X POST ${SLACK_WEBHOOK} -d '{\"text\":\"Deploy failed: $DEPLOQ_PROJECT\"}'"
    require_status_checks: true            # default: false
    status_check_max_wait: 10m             # default: 5m
```

Secrets use `${ENV_VAR}` interpolation — never stored in plaintext.

Set `DEPLOQ_GITHUB_TOKEN` when using `require_status_checks`.

## API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/webhook/{project}` | POST | Receive webhook, trigger deploy |
| `/status/{project}` | GET | Last deploy result (SHA, step, timestamp, error) |
| `/health` | GET | Health check (`{"status":"ok"}`) |

## Deploy Pipeline

```
webhook received
  → verify signature (HMAC-SHA256 for GitHub, token for generic)
  → check event type filter (push/release/ping)
  → check branch filter (skipped for release events)
  → check duplicate SHA
  → acquire project lock (non-blocking, returns 409 if busy)
  → wait for CI status checks (if enabled)
  → git fetch origin <branch>
  → git reset --hard origin/<branch>
  → docker compose build
  → docker compose up -d
  → on failure: run on_failure hook (if configured)
```

## Webhook Setup

### GitHub

1. Go to repo **Settings → Webhooks → Add webhook**
2. Payload URL: `https://deploy.example.com/webhook/my-app`
3. Content type: `application/json`
4. Secret: same as `DEPLOQ_SECRET_MY_APP`
5. Events: Select `push` and/or `Releases` (matching your `trigger` config)

### Generic CI (GitHub Actions, GitLab, etc.)

```yaml
- run: |
    curl -X POST https://deploy.example.com/webhook/my-app \
      -H "X-Deploq-Token: ${{ secrets.DEPLOQ_TOKEN }}" \
      -H "Content-Type: application/json" \
      -d '{"ref":"${{ github.ref }}","sha":"${{ github.sha }}"}'
```

## CLI Commands

```
deploq serve              # Start webhook server
deploq deploy <project>   # Manual deploy
deploq init               # Generate deploq.yaml
deploq validate           # Validate config
deploq version            # Print version
```

## Production Setup

<details>
<summary><strong>systemd</strong></summary>

```bash
sudo mkdir -p /etc/deploq

sudo tee /etc/deploq/deploq.yaml << 'EOF'
listen: ":9090"
projects:
  my-app:
    path: /home/deploy/my-app
    branch: main
    secret: "${DEPLOQ_SECRET_MY_APP}"
EOF

echo "DEPLOQ_SECRET_MY_APP=$(openssl rand -hex 20)" | sudo tee /etc/deploq/env
sudo chmod 600 /etc/deploq/env

sudo cp scripts/deploq.service /etc/systemd/system/
sudo systemctl enable --now deploq
```

</details>

<details>
<summary><strong>Caddy reverse proxy</strong></summary>

```
deploy.example.com {
    reverse_proxy localhost:9090
}
```

</details>

## Documentation

Full documentation available at [docs/](docs/).

## Contributing

PRs welcome. Please run `gofmt`, `go vet`, and `go test ./...` before submitting.

## License

MIT
