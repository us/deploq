# deploq

Lightweight git push deploy tool for Docker Compose projects. Single binary, zero dependencies.

Receives GitHub webhooks (or generic HTTP calls from CI), pulls the latest code, and runs `docker compose build && docker compose up -d`.

## Install

```bash
# Download binary (Linux amd64)
curl -L https://github.com/us/deploq/releases/latest/download/deploq-linux-amd64 -o deploq
chmod +x deploq
sudo mv deploq /usr/local/bin/

# Other platforms:
# deploq-linux-arm64, deploq-darwin-amd64, deploq-darwin-arm64
```

Or install with Go:

```bash
go install github.com/us/deploq/cmd/deploq@latest
```

Or build from source:

```bash
git clone https://github.com/us/deploq.git && cd deploq && make build
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

### One-liner install (Linux amd64)

```bash
curl -L https://github.com/us/deploq/releases/latest/download/deploq-linux-amd64 \
  | sudo tee /usr/local/bin/deploq > /dev/null && sudo chmod +x /usr/local/bin/deploq
```

### systemd

```bash
sudo mkdir -p /etc/deploq

# Create config
sudo tee /etc/deploq/deploq.yaml << 'EOF'
listen: ":9090"
projects:
  my-app:
    path: /home/deploy/my-app
    branch: main
    secret: "${DEPLOQ_SECRET_MY_APP}"
EOF

# Create secrets
echo "DEPLOQ_SECRET_MY_APP=$(openssl rand -hex 20)" | sudo tee /etc/deploq/env
sudo chmod 600 /etc/deploq/env

# Install service
sudo cp scripts/deploq.service /etc/systemd/system/
sudo systemctl enable --now deploq
```

### Caddy reverse proxy

```
# Option A: dedicated subdomain
deploy.example.com {
    reverse_proxy localhost:9090
}

# Option B: path-based routing on existing domain
example.com {
    handle /webhook/* {
        reverse_proxy localhost:9090
    }
    handle /health {
        reverse_proxy localhost:9090
    }
    handle {
        reverse_proxy localhost:3000
    }
}
```

## API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/webhook/{project}` | POST | Receive webhook, trigger deploy |
| `/health` | GET | Health check (`{"status":"ok"}`) |

## License

MIT
