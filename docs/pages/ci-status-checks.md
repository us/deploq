# CI Status Checks

Wait for GitHub CI to pass before deploying.

## Setup

1. Set the GitHub token:
```bash
export DEPLOQ_GITHUB_TOKEN="ghp_your_token_here"
```

2. Enable in config:
```yaml
projects:
  my-app:
    require_status_checks: true
    status_check_max_wait: 10m  # default: 5m
```

## How It Works

When a webhook arrives, deploq polls the GitHub Combined Status API:

```
GET /repos/{owner}/{repo}/commits/{sha}/status
```

- **success** — proceed with deploy
- **pending** — retry with backoff (0s, 10s, 20s, 30s, 30s...)
- **failure/error** — abort deploy
- **timeout** — abort deploy

## Requirements

- `DEPLOQ_GITHUB_TOKEN` must be set when `require_status_checks: true`
- `status_check_max_wait` must be less than `deploy_timeout`
- Release events have no SHA — CI check is skipped with a warning
