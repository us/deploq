# Failure Hooks

Run shell commands when a deploy fails.

## Configuration

```yaml
projects:
  my-app:
    on_failure: "curl -s -X POST ${SLACK_WEBHOOK} -d '{\"text\":\"Deploy failed: $DEPLOQ_PROJECT at step $DEPLOQ_STEP\"}'"
```

## Environment Variables

The hook receives these env vars:

| Variable | Description |
|----------|-------------|
| `DEPLOQ_PROJECT` | Project name |
| `DEPLOQ_SHA` | Commit SHA |
| `DEPLOQ_STEP` | Failed step (git_fetch, git_reset, compose_build, compose_up, status_check) |
| `DEPLOQ_ERROR` | Error message (sanitized, max 512 chars) |

## Behavior

- Runs via `sh -c` with a 30-second timeout
- Hook failures are logged but don't affect deploy status
- Error messages are sanitized: newlines replaced with spaces, null bytes removed, truncated to 512 chars (UTF-8 safe)
