# Generic CI Integration

For non-GitHub CI systems, use token-based authentication.

## Request Format

```bash
curl -X POST https://deploy.example.com/webhook/my-app \
  -H "X-Deploq-Token: your-secret-token" \
  -H "Content-Type: application/json" \
  -d '{"ref":"refs/heads/main","sha":"abc1234..."}'
```

## GitHub Actions Example

```yaml
- name: Trigger deploy
  run: |
    curl -X POST https://deploy.example.com/webhook/my-app \
      -H "X-Deploq-Token: ${{ secrets.DEPLOQ_TOKEN }}" \
      -H "Content-Type: application/json" \
      -d '{"ref":"${{ github.ref }}","sha":"${{ github.sha }}"}'
```
