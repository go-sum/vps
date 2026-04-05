GO      := /usr/local/go/bin/go
GOOS    ?= linux
GOARCH  ?= amd64
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
# build for linux amd64 - uncomment to build for host os
# GOOS   ?= $(shell go env GOOS)
# GOARCH ?= $(shell go env GOARCH)

.PHONY: help build clean vet version release

help: ## Show this help message
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: vet ## Cross-compile server and app binaries
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build $(LDFLAGS) -o bin/server ./cmd/server
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build $(LDFLAGS) -o bin/app ./cmd/app
	@echo "To copy binaries, use: scp bin/server bin/app user@remote:/opt/vps/bin/"

clean: ## Remove compiled binaries
	rm -f bin/server bin/app

vet: ## Run go vet on all packages
	$(GO) vet ./...

version: ## Show current version tag
	@echo $(VERSION)

release: ## Create and push a new version tag (override: make release V=v1.0.0)
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo ""); \
	if [ -n "$$latest" ] && [ -z "$$(git log $$latest..HEAD --oneline)" ]; then \
		echo "No changes since $$latest — nothing to release"; \
		exit 1; \
	fi; \
	prev=$${latest:-v0.0.0}; \
	next=$${V:-$$(echo $$prev | awk -F. '{print $$1"."$$2"."$$3+1}')}; \
	echo "Tagging $$next (was $$prev)"; \
	git tag -a "$$next" -m "Release $$next" && \
	git push origin "$$next"
