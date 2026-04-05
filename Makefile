GO     := /usr/local/go/bin/go
GOOS   ?= linux
GOARCH ?= amd64
# build for linux amd64 - uncomment to build for host os
# GOOS   ?= $(shell go env GOOS)
# GOARCH ?= $(shell go env GOARCH)

.PHONY: help build clean vet

help: ## Show this help message
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: vet ## Cross-compile admin and deploy binaries
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -o bin/admin  ./cmd/admin
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -o bin/deploy ./cmd/deploy
	echo "scp bin/admin bin/deploy user@remote:/opt/vps/bin/"

clean: ## Remove compiled binaries
	rm -f bin/admin bin/deploy

vet: ## Run go vet on all packages
	$(GO) vet ./...
