VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BIN     := bin/learning-server

.PHONY: all web build run test clean

all: build

web:
	cd web && npm run build

# Full single-binary build: frontend first, then Go with dist embedded.
build: web
	go build -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/server

run: build
	./$(BIN)

test:
	go vet ./...
	go test ./...

clean:
	rm -rf bin web/dist/assets web/dist/index.html
