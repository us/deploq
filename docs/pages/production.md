# Production Setup

## systemd Service

```bash
sudo mkdir -p /etc/deploq

# Create config
sudo tee /etc/deploq/deploq.yaml << 'EOF'
listen: ":9090"
projects:
  my-app:
    path: /home/deploy/my-app
    branch: main
    secret: "${DEPLOQ_SECRET_MY_APP}"
EOF

# Create secrets
echo "DEPLOQ_SECRET_MY_APP=$(openssl rand -hex 20)" | sudo tee /etc/deploq/env
sudo chmod 600 /etc/deploq/env

# Install service
sudo cp scripts/deploq.service /etc/systemd/system/
sudo systemctl enable --now deploq
```

## Reverse Proxy (Caddy)

```
# Dedicated subdomain
deploy.example.com {
    reverse_proxy localhost:9090
}

# Path-based routing
example.com {
    handle /webhook/* {
        reverse_proxy localhost:9090
    }
    handle /health {
        reverse_proxy localhost:9090
    }
}
```

## Reverse Proxy (Nginx)

```nginx
server {
    listen 443 ssl;
    server_name deploy.example.com;

    location / {
        proxy_pass http://127.0.0.1:9090;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_for_addr;
    }
}
```
