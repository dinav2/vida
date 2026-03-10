PREFIX     ?= $(HOME)/.local
BINDIR     := $(PREFIX)/bin
CGO_ENABLED := 1

.PHONY: build install test vet clean restart

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

# Rebuild and restart running processes (useful during development).
restart: build
	pkill vida-daemon 2>/dev/null || true
	pkill vida-ui     2>/dev/null || true
	./bin/vida-daemon &
	sleep 0.3
	./bin/vida-ui &
