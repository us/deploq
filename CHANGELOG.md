# Changelog

All notable changes to deploq are documented here.

## v0.0.3

- **Event Type Filtering** — Configure which webhook events trigger deploys via `trigger: [push, release]`. Ping events return 200 pong. Unsupported event types are rejected.
- **Deploy Failure Handling** — New `/status/{project}` endpoint returns last deploy result with SHA, step, timestamp, and error. Optional `on_failure` shell hook runs on deploy failure with env vars (`DEPLOQ_PROJECT`, `DEPLOQ_SHA`, `DEPLOQ_STEP`, `DEPLOQ_ERROR`).
- **CI Status Check** — Poll GitHub commit status API before deploying when `require_status_checks: true`. Configurable `status_check_max_wait` with backoff strategy. Fails fast if token is missing.
- **Input Validation** — Tag names, ref names, owner/repo validated with regex. SHA format validation. Path traversal prevention on release events.
- **Safety Improvements** — TOCTOU race fix in repo info cache, UTF-8 safe env value sanitization with null byte filtering, graceful shutdown with 5-minute deploy wait timeout, `defaultBackoff` slice isolation.

## v0.0.2

- **Binary Releases** — Multi-platform release workflow (linux/darwin, amd64/arm64).
- **Install Instructions** — Updated docs with binary releases and `go install`.

## v0.0.1

- **Initial Release** — Webhook-based Docker Compose deploy tool with GitHub HMAC-SHA256 and generic token verification, branch filtering, deploy locking, and graceful shutdown.
