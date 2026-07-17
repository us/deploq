# Changelog

All notable changes to deploq are documented here.

## [0.0.4](https://github.com/us/deploq/compare/v0.0.3...v0.0.4) (2026-07-17)


### Features

* add per-project deploy_command hook ([0496f62](https://github.com/us/deploq/commit/0496f62009a3f099bbb0e257fa20b056599169d6))
* **gate:** gate deploys on GitHub check-runs with a name allowlist ([3c8c1e6](https://github.com/us/deploq/commit/3c8c1e61e5c63f6cb1e7a3b67abc91d38e34acf9))
* inject DEPLOQ_PROJECT/DEPLOQ_SHA into deploy_command env ([136b783](https://github.com/us/deploq/commit/136b78341a5c387eb7a676fd738c80a8db931085))


### Bug Fixes

* **deploy:** coalesce webhooks to latest + kill deploy process group ([07acd5d](https://github.com/us/deploq/commit/07acd5d6258420cdb7ff475884935cd61e6b5a3b))
* pass error message to on_failure hook via DEPLOQ_ERROR env var ([2dacece](https://github.com/us/deploq/commit/2dacececb39c37440a4ce76c80b0fb49b943d911))

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
