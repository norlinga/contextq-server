.PHONY: build release test vet check

GOCACHE ?= /tmp/contextq-server-go-cache
DIST ?= dist
TARGET_GOOS ?= linux
TARGET_GOARCH ?= $(shell go env GOARCH)
HOST_GOOS := $(shell go env GOOS)
HOST_GOARCH := $(shell go env GOARCH)
REMOTE_DIST := $(DIST)/$(TARGET_GOOS)-$(TARGET_GOARCH)
CONTEXTQ_SOURCE ?= ../contextq
RELEASE_GCFLAGS ?= all=-l
RELEASE_LDFLAGS ?= -s -w -buildid=

build:
	mkdir -p bin
	GOCACHE=$(GOCACHE) go build -buildvcs=false -o bin/contextq-server ./cmd/contextq-server

release:
	mkdir -p $(REMOTE_DIST)
	CGO_ENABLED=0 GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) GOCACHE=$(GOCACHE) go build -buildvcs=false -trimpath -gcflags='$(RELEASE_GCFLAGS)' -ldflags='$(RELEASE_LDFLAGS)' -o $(DIST)/contextq-server ./cmd/contextq-server
	CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) GOCACHE=$(GOCACHE) go build -buildvcs=false -trimpath -gcflags='$(RELEASE_GCFLAGS)' -ldflags='$(RELEASE_LDFLAGS)' -o $(REMOTE_DIST)/contextq-server ./cmd/contextq-server
	cd $(CONTEXTQ_SOURCE) && CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) GOCACHE=$(GOCACHE) go build -trimpath -gcflags='$(RELEASE_GCFLAGS)' -ldflags='$(RELEASE_LDFLAGS)' -o $(abspath $(REMOTE_DIST))/contextq ./cmd/contextq
	tar -C $(REMOTE_DIST) -czf $(DIST)/contextq-$(TARGET_GOOS)-$(TARGET_GOARCH).tar.gz contextq-server contextq
	cd $(DIST) && sha256sum contextq-server $(TARGET_GOOS)-$(TARGET_GOARCH)/contextq-server $(TARGET_GOOS)-$(TARGET_GOARCH)/contextq contextq-$(TARGET_GOOS)-$(TARGET_GOARCH).tar.gz > SHA256SUMS

test:
	GOCACHE=$(GOCACHE) go test ./...

vet:
	GOCACHE=$(GOCACHE) go vet ./...

check: test vet
