VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test clean release lint vet

build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o pushup ./cmd/pushup

test:
	go test ./... -v -count=1

vet:
	go vet ./...

lint: vet
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipping"

clean:
	rm -f pushup pushup-linux-amd64

release:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o pushup-linux-amd64 ./cmd/pushup
	@echo "Built pushup-linux-amd64 ($(VERSION))"
