# API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/webhook/{project}` | POST | Receive webhook, trigger deploy |
| `/status/{project}` | GET | Last deploy result |
| `/health` | GET | Health check |

## POST /webhook/{project}

Trigger a deploy for the named project.

### Response Codes

| Status | Body | Meaning |
|--------|------|---------|
| 202 | `{"status":"accepted","sha":"..."}` | Deploy started |
| 200 | `{"status":"skipped","reason":"..."}` | Skipped (branch mismatch, event type, duplicate) |
| 200 | `{"status":"pong"}` | Ping event |
| 409 | `{"status":"rejected","reason":"deploy already in progress"}` | Locked |
| 401 | `unauthorized` | Signature/token invalid |
| 404 | `project not found` | Unknown project |
| 400 | `invalid project name` | Bad project name |

## GET /status/{project}

Returns the last deploy result.

```json
{
  "sha": "abc1234...",
  "step": "done",
  "timestamp": "2026-03-15T14:30:00Z",
  "error": ""
}
```

## GET /health

```json
{"status": "ok"}
```
