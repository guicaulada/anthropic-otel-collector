OCB_VERSION  := 0.146.1
GOOS         ?= $(shell go env GOOS)
GOARCH       ?= $(shell go env GOARCH)
OCB          := ./ocb
DIST_DIR     := ./dist
BINARY       := $(DIST_DIR)/anthropic-otel-collector

.PHONY: install-ocb build run test lint clean docker-build docker-up docker-down dashboard

## install-ocb: Download the OpenTelemetry Collector Builder binary.
install-ocb:
	@if [ ! -f "$(OCB)" ]; then \
		echo "Downloading ocb $(OCB_VERSION) for $(GOOS)/$(GOARCH)..."; \
		curl -fsSL -o $(OCB) \
			"https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/cmd%2Fbuilder%2Fv$(OCB_VERSION)/ocb_$(OCB_VERSION)_$(GOOS)_$(GOARCH)"; \
		chmod +x $(OCB); \
	else \
		echo "ocb already exists, skipping download."; \
	fi

## build: Build the custom collector using the ocb builder.
build: install-ocb
	GOWORK=off $(OCB) --config builder-config.yaml

## run: Run the built collector with the example configuration.
run: build
	$(BINARY) --config collector-config.yaml

## test: Run all tests with race detection.
test:
	go test -race ./receiver/anthropicreceiver/...

## lint: Run go vet across all modules.
lint:
	go vet ./...

## clean: Remove build artifacts.
clean:
	rm -rf $(DIST_DIR)

## dashboard: Generate the Grafana dashboard JSON.
dashboard:
	@mkdir -p dashboard/dist
	cd dashboard && go run . > dist/anthropic-claude-code-usage.json

## docker-build: Build the Docker image for the collector.
docker-build:
	docker build -t anthropic-otel-collector:latest .

## docker-up: Start the full observability stack with Docker Compose.
docker-up: dashboard
	docker compose up -d

## docker-down: Stop the Docker Compose stack.
docker-down:
	docker compose down
