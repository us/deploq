# Changelog

## v0.0.3

- **Event Type Filtering** — Configure which webhook events trigger deploys via `trigger: [push, release]`. Ping events return 200 pong.
- **Deploy Failure Handling** — `/status/{project}` endpoint + `on_failure` shell hook with env vars.
- **CI Status Check** — Poll GitHub commit status API with backoff before deploying.
- **Input Validation** — Tag names, ref names, owner/repo validated. Path traversal prevention.
- **Safety** — TOCTOU race fix, UTF-8 safe sanitization, graceful shutdown timeout.

## v0.0.2

- Multi-platform binary releases (linux/darwin, amd64/arm64)

## v0.0.1

- Initial release — webhook-based Docker Compose deploy tool
