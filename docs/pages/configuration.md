# Configuration

deploq uses a YAML configuration file with environment variable interpolation.

## Full Example

```yaml
listen: ":9090"

projects:
  backend:
    path: /home/deploy/backend
    branch: main
    secret: "${DEPLOQ_SECRET_BACKEND}"
    compose_file: docker-compose.prod.yml
    deploy_timeout: 15m
    trigger: [push, release]
    on_failure: "curl -s -X POST ${SLACK_WEBHOOK} -d '{\"text\":\"Deploy failed: $DEPLOQ_PROJECT\"}'"
    require_status_checks: true
    status_check_max_wait: 10m

  frontend:
    path: /home/deploy/frontend
    branch: main
    secret: "${DEPLOQ_SECRET_FRONTEND}"
    trigger: [push]
```

## Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `path` | string | required | Absolute path to the project directory |
| `branch` | string | required | Branch to deploy |
| `secret` | string | required | Webhook secret (min 16 chars) |
| `compose_file` | string | `docker-compose.yml` | Docker Compose file name |
| `deploy_timeout` | duration | `15m` | Maximum deploy duration |
| `trigger` | string[] | `[push]` | Event types to trigger deploy |
| `on_failure` | string | empty | Shell command to run on failure |
| `require_status_checks` | bool | `false` | Wait for CI before deploying |
| `status_check_max_wait` | duration | `5m` | Max wait for CI status |

## Environment Variables

Secrets use `${ENV_VAR}` interpolation — never stored in plaintext. All referenced variables must be set or the config will fail to load.
