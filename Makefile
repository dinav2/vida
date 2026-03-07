PREFIX     ?= $(HOME)/.local
BINDIR     := $(PREFIX)/bin
CGO_ENABLED := 1

.PHONY: build install test vet clean

build:
	CGO_ENABLED=$(CGO_ENABLED) go build -o bin/vida         ./cmd/vida/
	CGO_ENABLED=0               go build -o bin/vida-daemon  ./cmd/vida-daemon/
	CGO_ENABLED=$(CGO_ENABLED) go build -o bin/vida-ui       ./cmd/vida-ui/

install: build
	install -Dm755 bin/vida        $(BINDIR)/vida
	install -Dm755 bin/vida-daemon $(BINDIR)/vida-daemon
	install -Dm755 bin/vida-ui     $(BINDIR)/vida-ui

test:
	go test ./internal/... ./cmd/vida-daemon/

vet:
	go vet ./...

clean:
	rm -rf bin/
