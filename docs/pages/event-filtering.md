# Event Filtering

Control which webhook events trigger deploys with the `trigger` config.

## Supported Events

- `push` — triggered on git push to the configured branch
- `release` — triggered when a GitHub release is published

## Configuration

```yaml
projects:
  my-app:
    trigger: [push]          # default: only push events
    # trigger: [release]     # only release events
    # trigger: [push, release]  # both
```

## Ping Events

GitHub sends a `ping` event when a webhook is first created. deploq responds with `200 {"status":"pong"}` automatically.

## Branch Filtering

Push events are filtered by the configured `branch`. Release events skip branch filtering since they are tag-based.
