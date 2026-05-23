.PHONY: fmt test build doctor lint vet clean install frontend dashboard

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

fmt:
	gofmt -w cmd internal

vet:
	go vet ./...

lint: vet
	@echo "lint: go vet passed"

test:
	go test ./...

test-verbose:
	go test -v ./...

frontend:
	cd web/frontend && npm install && npm run build

build: frontend
	go build -ldflags "-X main.version=$(VERSION)" -o bin/cyberagent ./cmd/cyberagent

build-quick:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/cyberagent ./cmd/cyberagent

install: frontend
	go install ./cmd/cyberagent

dashboard: build
	./bin/cyberagent dashboard

dev-dashboard:
	@echo "Starting Go backend on :8080..."
	go run ./cmd/cyberagent dashboard --dev &
	@echo "Starting Vite dev server on :5173..."
	cd web/frontend && npm run dev

doctor:
	go run ./cmd/cyberagent doctor

clean:
	rm -rf bin/ runs/ internal/interfaces/web/frontend/dist
