# deploq

Lightweight git push deploy tool for Docker Compose projects. Single binary, zero dependencies.

Receives GitHub webhooks (or generic HTTP calls from CI), pulls the latest code, and runs `docker compose build && docker compose up -d`.

## Install

```bash
# Build from source
git clone https://github.com/us/deploq.git
cd deploq
make build

# Cross-compile for Linux
make release
scp deploq-linux-amd64 your-server:~/deploq
```

## Quick Start

```bash
# Generate config
deploq init

# Edit deploq.yaml, set environment variables
export DEPLOQ_SECRET_MY_PROJECT="your-secret-here-min-16-chars"

# Validate config
deploq validate

# Start server
deploq serve
```

## Configuration

```yaml
listen: ":9090"

projects:
  my-app:
    path: /home/deploy/my-app
    branch: main
    secret: "${DEPLOQ_SECRET_MY_APP}"
    compose_file: docker-compose.prod.yml  # optional, default: docker-compose.yml
    deploy_timeout: 15m                     # optional, default: 15m
```

Secrets use `${ENV_VAR}` interpolation — never stored in plaintext.

## Webhook Setup

### GitHub

1. Go to repo Settings → Webhooks → Add webhook
2. Payload URL: `https://deploy.example.com/webhook/my-app`
3. Content type: `application/json`
4. Secret: same as `DEPLOQ_SECRET_MY_APP`
5. Events: Just the `push` event

### GitHub Actions / Generic CI

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

## Deploy Pipeline

```
webhook received
  → verify signature (HMAC-SHA256 for GitHub, token for generic)
  → check branch filter
  → check duplicate SHA
  → acquire project lock (non-blocking, returns 409 if busy)
  → git fetch origin <branch>
  → git reset --hard origin/<branch>
  → docker compose build
  → docker compose up -d
```

## Production Setup

### systemd

```bash
sudo cp deploq /usr/local/bin/
sudo cp scripts/deploq.service /etc/systemd/system/
sudo mkdir -p /etc/deploq
sudo cp deploq.yaml /etc/deploq/
echo "DEPLOQ_SECRET_MY_APP=your-secret" | sudo tee /etc/deploq/env
sudo systemctl enable --now deploq
```

### Caddy reverse proxy

```
deploy.example.com {
    reverse_proxy localhost:9090
}
```

## API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/webhook/{project}` | POST | Receive webhook, trigger deploy |
| `/health` | GET | Health check (`{"status":"ok"}`) |

## License

MIT
