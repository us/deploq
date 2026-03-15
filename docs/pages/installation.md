# Installation

## Binary Download

Download the latest release for your platform:

```bash
# Linux amd64
curl -L https://github.com/us/deploq/releases/latest/download/deploq-linux-amd64 -o deploq
chmod +x deploq
sudo mv deploq /usr/local/bin/

# Other platforms:
# deploq-linux-arm64
# deploq-darwin-amd64
# deploq-darwin-arm64
```

## Go Install

```bash
go install github.com/us/deploq/cmd/deploq@latest
```

## Build from Source

```bash
git clone https://github.com/us/deploq.git
cd deploq
make build
```
