# Quick Start

## 1. Generate Config

```bash
deploq init
```

This creates a `deploq.yaml` template.

## 2. Set Environment Variables

```bash
export DEPLOQ_SECRET_MY_APP="your-secret-here-min-16-chars"
```

## 3. Validate Config

```bash
deploq validate
```

## 4. Start Server

```bash
deploq serve
```

The server listens on the configured address (default `:9090`) and accepts webhooks at `/webhook/{project}`.

## 5. Configure Webhook

Add a webhook in your GitHub repository settings pointing to `https://your-server/webhook/your-project`.
