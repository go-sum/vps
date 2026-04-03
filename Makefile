GOOS   ?= linux
GOARCH ?= amd64
GO     := /usr/local/go/bin/go

.PHONY: build vet clean

build: vet ## Cross-compile admin and deploy binaries
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -o bin/admin  ./cmd/admin
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -o bin/deploy ./cmd/deploy

vet: ## Run go vet on all packages
	$(GO) vet ./...

clean: ## Remove compiled binaries
	rm -f bin/admin bin/deploy
