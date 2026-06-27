.PHONY: build release test vet check

GOCACHE ?= /tmp/contextq-server-go-cache
GOMODCACHE ?= $(shell go env GOMODCACHE)
DIST ?= dist
TARGET_GOOS ?= linux
TARGET_GOARCH ?= $(shell go env GOARCH)
HOST_GOOS := $(shell go env GOOS)
HOST_GOARCH := $(shell go env GOARCH)
REMOTE_DIST := $(DIST)/$(TARGET_GOOS)-$(TARGET_GOARCH)
CONTEXTQ_VERSION ?= $(shell cat CONTEXTQ_VERSION)
CONTEXTQ_MODULE := github.com/norlinga/contextq
CONTEXTQ_SOURCE ?=
RELEASE_GCFLAGS ?= all=-l
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell git show -s --format=%cI HEAD 2>/dev/null || echo unknown)
SOURCE_DATE_EPOCH ?= $(shell git show -s --format=%ct HEAD 2>/dev/null || date +%s)
BUILDINFO_LDFLAGS := -X github.com/norlinga/contextq-server/internal/buildinfo.Version=$(VERSION) -X github.com/norlinga/contextq-server/internal/buildinfo.Commit=$(COMMIT) -X github.com/norlinga/contextq-server/internal/buildinfo.BuildDate=$(BUILD_DATE) -X github.com/norlinga/contextq-server/internal/buildinfo.ContextqVersion=$(CONTEXTQ_VERSION)
RELEASE_LDFLAGS ?= -s -w -buildid= $(BUILDINFO_LDFLAGS)

build:
	mkdir -p bin
	GOCACHE=$(GOCACHE) go build -buildvcs=false -o bin/contextq-server ./cmd/contextq-server

release:
	mkdir -p $(REMOTE_DIST)
	CGO_ENABLED=0 GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) GOCACHE=$(GOCACHE) go build -buildvcs=false -trimpath -gcflags='$(RELEASE_GCFLAGS)' -ldflags='$(RELEASE_LDFLAGS)' -o $(DIST)/contextq-server ./cmd/contextq-server
	CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) GOCACHE=$(GOCACHE) go build -buildvcs=false -trimpath -gcflags='$(RELEASE_GCFLAGS)' -ldflags='$(RELEASE_LDFLAGS)' -o $(REMOTE_DIST)/contextq-server ./cmd/contextq-server
	@set -eu; if [ -n "$(CONTEXTQ_SOURCE)" ]; then \
		cd "$(CONTEXTQ_SOURCE)" && CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) GOCACHE=$(GOCACHE) go build -buildvcs=false -trimpath -gcflags='$(RELEASE_GCFLAGS)' -ldflags='-s -w -buildid=' -o $(abspath $(REMOTE_DIST))/contextq ./cmd/contextq; \
	else \
		GOMODCACHE=$(GOMODCACHE) go mod download $(CONTEXTQ_MODULE)@$(CONTEXTQ_VERSION); \
		cd "$(GOMODCACHE)/$(CONTEXTQ_MODULE)@$(CONTEXTQ_VERSION)" && CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go build -buildvcs=false -trimpath -gcflags='$(RELEASE_GCFLAGS)' -ldflags='-s -w -buildid=' -o $(abspath $(REMOTE_DIST))/contextq ./cmd/contextq; \
	fi
	cp LICENSE $(REMOTE_DIST)/CONTEXTQ_SERVER_LICENSE
	cp licenses/contextq-LICENSE $(REMOTE_DIST)/CONTEXTQ_LICENSE
	cp THIRD_PARTY_NOTICES.md $(REMOTE_DIST)/THIRD_PARTY_NOTICES.md
	mkdir -p $(REMOTE_DIST)/licenses
	cp licenses/APACHE-2.0.txt licenses/gofrs-flock-LICENSE licenses/google-uuid-LICENSE licenses/spf13-pflag-LICENSE licenses/golang-x-sys-LICENSE $(REMOTE_DIST)/licenses/
	tar --sort=name --mtime=@$(SOURCE_DATE_EPOCH) --owner=0 --group=0 --numeric-owner -C $(REMOTE_DIST) -cf $(DIST)/contextq-$(TARGET_GOOS)-$(TARGET_GOARCH).tar contextq-server contextq CONTEXTQ_SERVER_LICENSE CONTEXTQ_LICENSE THIRD_PARTY_NOTICES.md licenses
	gzip -n -9 -f $(DIST)/contextq-$(TARGET_GOOS)-$(TARGET_GOARCH).tar
	cd $(DIST) && sha256sum contextq-server $(TARGET_GOOS)-$(TARGET_GOARCH)/contextq-server $(TARGET_GOOS)-$(TARGET_GOARCH)/contextq contextq-$(TARGET_GOOS)-$(TARGET_GOARCH).tar.gz > SHA256SUMS

test:
	GOCACHE=$(GOCACHE) go test ./...

vet:
	GOCACHE=$(GOCACHE) go vet ./...

check: test vet
