# GitHub Webhooks

## Setup

1. Go to repo **Settings → Webhooks → Add webhook**
2. **Payload URL**: `https://deploy.example.com/webhook/my-app`
3. **Content type**: `application/json`
4. **Secret**: same as your `DEPLOQ_SECRET_MY_APP`
5. **Events**: Select `push` and/or `Releases` (matching your `trigger` config)

## Verification

deploq verifies GitHub webhooks using HMAC-SHA256 signature in the `X-Hub-Signature-256` header. Constant-time comparison prevents timing attacks.
