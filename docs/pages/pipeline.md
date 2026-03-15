# Deploy Pipeline

## Steps

```
1. Webhook received
2. Verify signature (HMAC-SHA256 for GitHub, token for generic)
3. Check event type filter (push/release/ping)
4. Check branch filter (skipped for release events)
5. Check duplicate SHA
6. Acquire project lock (non-blocking, returns 409 if busy)
7. Wait for CI status checks (if enabled)
8. git fetch origin <branch>
9. git reset --hard origin/<branch>
10. docker compose build
11. docker compose up -d
12. On failure: run on_failure hook (if configured)
```

## Locking

Each project has an independent lock. Only one deploy per project can run at a time. Concurrent webhook requests for the same project get a 409 response.

## Duplicate Detection

If the same SHA is already being deployed (or was just deployed successfully), the webhook returns 200 with `"status":"skipped"`.
