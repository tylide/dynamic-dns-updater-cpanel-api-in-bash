BINARY := dnsupdater
PKG := ./cmd/dnsupdater
GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || date +%Y%m%d-%H%M)
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST := dist

.PHONY: build build-all checksum install clean vet test fmt

build:
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY) $(PKG)

build-all:
	mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-amd64 $(PKG)
	GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-arm64 $(PKG)
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-armv7 $(PKG)
	GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-windows-amd64.exe $(PKG)
	GOOS=windows GOARCH=arm64 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-windows-arm64.exe $(PKG)
	GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-darwin-amd64 $(PKG)
	GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-darwin-arm64 $(PKG)
	$(MAKE) checksum

checksum:
	cd $(DIST) && sha256sum $(BINARY)-* > sha256sums.txt

install: build
	install -m 0755 $(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf $(DIST)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

fmt:
	gofmt -l -w cmd internal
