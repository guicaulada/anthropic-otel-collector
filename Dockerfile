# Stage 1: Build the custom collector binary.
FROM golang:1.26 AS builder

ARG OCB_VERSION=0.146.1
ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /build

# Download the OpenTelemetry Collector Builder.
RUN curl -fsSL -o /usr/local/bin/ocb \
    "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/cmd%2Fbuilder%2Fv${OCB_VERSION}/ocb_${OCB_VERSION}_${TARGETOS}_${TARGETARCH}" \
    && chmod +x /usr/local/bin/ocb

# Copy source files.
COPY go.work go.work.sum ./
COPY receiver/ receiver/
COPY builder-config.yaml .

# Build the collector.
RUN GOWORK=off ocb --config builder-config.yaml

# Stage 2: Minimal runtime image.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /build/dist/anthropic-otel-collector /usr/local/bin/anthropic-otel-collector

# 4319  - Anthropic receiver (HTTP proxy)
# 13133 - Health check extension
EXPOSE 4319 13133

ENTRYPOINT ["anthropic-otel-collector", "--config=/etc/otelcol/config.yaml"]
