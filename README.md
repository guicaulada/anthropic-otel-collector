# Anthropic OTel Collector

A custom [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/) that acts as a transparent reverse proxy for LLM APIs, capturing comprehensive telemetry — traces, metrics, and logs — for every API call.

Supports **multiple providers** (Anthropic, OpenAI, Google Gemini) through a pluggable adapter framework, with first-class [OpenClaw](https://github.com/openclaw) integration for multi-agent observability.

Point your LLM SDK at the collector instead of the provider API and get full observability with zero code changes.

## How It Works

```
┌──────────────┐       ┌─────────────────────────────────────┐       ┌───────────────────┐
│   LLM SDK    │──────▶│     OTel Collector (Multi-Provider) │──────▶│  Provider APIs    │
│   Client     │◀──────│  ┌─────────┐ ┌─────────┐ ┌────────┐│◀──────│  (Anthropic,      │
│              │       │  │Anthropic│ │ OpenAI  │ │ Gemini ││       │   OpenAI, etc.)   │
└──────────────┘       │  │ Adapter │ │ Adapter │ │Adapter ││       └───────────────────┘
                       │  └────┬────┘ └────┬────┘ └───┬────┘│
┌──────────────┐       │       └────────────┴──────────┘     │
│   OpenClaw   │──────▶│              │                      │
│   Gateway    │◀──────│       Unified Telemetry             │
└──────────────┘       └──────────────┬──────────────────────┘
                                      │
                        traces, metrics, logs
                                      │
                                      ▼
                       ┌─────────────────────────┐
                       │    OTLP Backend         │
                       │  (Grafana, Jaeger, etc) │
                       └─────────────────────────┘
```

The collector intercepts all traffic between your application and LLM provider APIs. It automatically detects the provider, parses requests and responses (including SSE streams), extracts telemetry, and forwards everything to your observability backend via OTLP. The proxy is fully transparent — your application receives the original API response unmodified.

## Supported Providers

| Provider | Detection | Streaming | Cost Tracking |
|----------|-----------|-----------|---------------|
| **Anthropic** | `x-api-key` or `anthropic-version` header | SSE (`message_start`, `content_block_delta`, etc.) | Per-model with cache pricing |
| **OpenAI** | `Authorization: Bearer sk-...` header | SSE (`data: {...}`, `data: [DONE]`) | Per-model with cached token pricing |
| **Google Gemini** | `key` query parameter or `contents` in body | SSE with Gemini response format | Per-model pricing |

Provider detection is automatic — the collector inspects request headers and body to route to the correct adapter.

## Quick Start

### Docker Compose

Starts the collector alongside a local [Grafana LGTM](https://github.com/grafana/docker-otel-lgtm) stack (Loki, Grafana, Tempo, Mimir) for development and testing:

```bash
docker compose up -d
```

This exposes:

| Port | Service                                  |
| ---- | ---------------------------------------- |
| 4319 | LLM receiver (point your SDK here)       |
| 3000 | Grafana UI                               |
| 4317 | OTLP gRPC                                |
| 4318 | OTLP HTTP                                |

Then configure your LLM SDK to use the proxy:

```python
# Anthropic
import anthropic
client = anthropic.Anthropic(base_url="http://localhost:4319")

# OpenAI
from openai import OpenAI
client = OpenAI(base_url="http://localhost:4319")
```

Open [http://localhost:3000](http://localhost:3000) to explore traces, metrics, and logs in Grafana. Two dashboards are pre-provisioned — see [Dashboards](#dashboards) for details.

### Local Build

Requires Go 1.24+ installed.

```bash
# Build the collector binary
make build

# Run with the example configuration
make run
```

To customise the configuration locally, copy the default and edit:

```bash
cp collector-config.yaml collector-config.local.yaml
```

`make run` automatically picks up `collector-config.local.yaml` when present. The local file is git-ignored.

### Running as a Background Service

For running the collector as a persistent daemon, you can use the [Docker image](#docker-build) with your container runtime, or register the binary with [launchd](https://support.apple.com/guide/terminal/apdc6c1077b-5d5d-4d35-9c19-60f2397b2369/mac) (macOS) or [systemd](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html) (Linux).

### Docker Build

```bash
make docker-build
docker run -p 4319:4319 -v ./collector-config.local.yaml:/etc/otelcol/config.yaml:ro anthropic-otel-collector:latest
```

## Using with Claude Code

Claude Code uses the `ANTHROPIC_BASE_URL` environment variable to determine which API endpoint to call. Point it at the collector to capture all telemetry automatically.

### Option 1: Environment variable

Set the environment variable before launching Claude Code:

```bash
export ANTHROPIC_BASE_URL=http://localhost:4319
claude
```

Or prefix a single invocation:

```bash
ANTHROPIC_BASE_URL=http://localhost:4319 claude
```

### Option 2: Claude Code settings

Add the base URL to your Claude Code settings file at `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:4319"
  }
}
```

This persists the setting across all Claude Code sessions without needing to export the variable each time.

### What you get

Once configured, the collector automatically:

- Detects Claude Code requests via the `anthropic-beta` header
- Extracts the **project name** from the system prompt's working directory
- Groups all metrics, traces, and logs by project
- Tracks tool usage (Edit, Write, Read, Bash, Glob, Grep) with file-level detail
- Calculates per-request and per-project costs including fast mode and long context multipliers

## OpenClaw Integration

The collector provides first-class support for [OpenClaw](https://github.com/openclaw) multi-agent systems. It extracts agent context from request headers, maps them to OTel attributes, and provides unified telemetry across all providers used by OpenClaw agents.

### Context Headers

OpenClaw sends custom headers with each LLM request that the collector extracts for telemetry:

| Header | OTel Attribute | Description |
|--------|---------------|-------------|
| `X-OpenClaw-Agent-ID` | `openclaw.agent.id` | Unique agent identifier |
| `X-OpenClaw-Session-Key` | `openclaw.session.key` | Session key for grouping related calls |
| `X-OpenClaw-Channel` | `openclaw.channel` | Communication channel (telegram, discord, slack, etc.) |
| `X-OpenClaw-Workspace` | `openclaw.workspace` | Workspace or project context |
| `X-OpenClaw-Provider` | `openclaw.provider` | Target LLM provider name |
| `X-OpenClaw-Target-URL` | — | Original provider API URL (used for routing) |

### W3C Trace Context Propagation

The collector supports W3C `traceparent` header propagation for end-to-end distributed tracing. When OpenClaw sends a `traceparent` header, the collector extracts the trace context and propagates it to the upstream provider, creating a connected trace from OpenClaw through the collector to the provider API.

### OpenClaw Client Middleware

Use the `createCollectorProxy` middleware in OpenClaw to route LLM traffic through the collector with full context:

```typescript
// openclaw/src/infra/otel-collector.ts
export interface CollectorConfig {
  enabled: boolean;
  baseUrl: string;  // e.g., "http://localhost:4319"
}

export function createCollectorProxy(
  providerConfig: ModelProviderConfig,
  collectorConfig: CollectorConfig,
  context: OpenClawContext,
  traceContext?: { traceId: string; spanId: string; sampled?: boolean }
): ModelProviderConfig {
  if (!collectorConfig.enabled) {
    return providerConfig;
  }

  // Build W3C traceparent header for distributed tracing
  const traceparent = traceContext
    ? `00-${traceContext.traceId}-${traceContext.spanId}-${traceContext.sampled ? '01' : '00'}`
    : undefined;

  return {
    ...providerConfig,
    baseUrl: collectorConfig.baseUrl,
    headers: {
      ...providerConfig.headers,
      ...(traceparent && { 'traceparent': traceparent }),
      'X-OpenClaw-Agent-ID': context.agentId,
      'X-OpenClaw-Session-Key': context.sessionKey,
      'X-OpenClaw-Channel': context.channel,
      'X-OpenClaw-Workspace': context.workspace,
      'X-OpenClaw-Provider': providerConfig.id,
      'X-OpenClaw-Target-URL': providerConfig.baseUrl,
    },
  };
}
```

### OpenClaw Configuration

```yaml
# ~/.openclaw/openclaw.yaml
gateway:
  # ... existing config

models:
  otel_collector:
    enabled: true
    base_url: "http://localhost:4319"
    # Can also be set per-provider

  providers:
    anthropic:
      enabled: true
      # If otel_collector.enabled, requests go through collector
      # which then proxies to the actual Anthropic API
```

### Unified Telemetry Schema

All metrics use a consistent `llm.*` namespace with standard labels across providers:

**Metrics:**

| Metric | Type | Description |
|--------|------|-------------|
| `llm.requests.total` | Counter | Total LLM API requests |
| `llm.request.duration` | Histogram | Request duration |
| `llm.request.errors` | Counter | Failed requests |
| `llm.tokens.input` | Counter | Input tokens consumed |
| `llm.tokens.output` | Counter | Output tokens generated |
| `llm.tokens.cache.read` | Counter | Tokens served from cache |
| `llm.tokens.cache.write` | Counter | Tokens written to cache |
| `llm.cost.total` | Counter | Total cost in USD |
| `llm.tool_calls.total` | Counter | Tool call invocations |

**Standard Labels:**

| Label | Description | Example Values |
|-------|-------------|----------------|
| `provider` | LLM provider | `anthropic`, `openai`, `gemini` |
| `model` | Model identifier | `claude-sonnet-4-6`, `gpt-4o`, `gemini-2.0-flash` |
| `agent_id` | OpenClaw agent ID | `agent-123` |
| `channel` | Communication channel | `telegram`, `discord`, `slack` |
| `workspace` | Normalized workspace | `openclaw-core`, `dev-workspace` |
| `status` | Request outcome | `success`, `error`, `timeout` |

### What You Get

- **Per-agent cost tracking** across all providers
- **Channel-level metrics** (latency, token usage, errors by channel)
- **Workspace activity** dashboards
- **Provider comparison** (cost, latency, quality side-by-side)
- **End-to-end traces** from OpenClaw through the collector to provider APIs

## Sending Data to Grafana Cloud

To export telemetry to [Grafana Cloud](https://grafana.com/products/cloud/) instead of a local stack, configure the OTLP exporter with your Grafana Cloud OTLP endpoint and authentication token.

### 1. Get your Grafana Cloud OTLP credentials

In your Grafana Cloud portal, navigate to **Connections > OpenTelemetry (OTLP)** and copy:

- **OTLP endpoint** (e.g., `https://otlp-gateway-prod-us-east-0.grafana.net/otlp`)
- **Instance ID** (your numeric instance ID)
- **API token** (generate one with `metrics:write`, `traces:write`, and `logs:write` scopes)

### 2. Configure the collector

Create a `collector-config.yaml` with the OTLP HTTP exporter pointing to Grafana Cloud:

```yaml
receivers:
  anthropic:
    endpoint: "127.0.0.1:4319"

exporters:
  otlphttp:
    endpoint: "https://otlp-gateway-prod-us-east-0.grafana.net/otlp"
    headers:
      Authorization: "Basic <base64-encoded instance_id:api_token>"

processors:
  batch: {}
  deltatocumulative: {}

service:
  pipelines:
    traces:
      receivers: [anthropic]
      processors: [batch]
      exporters: [otlphttp]
    metrics:
      receivers: [anthropic]
      processors: [deltatocumulative, batch]
      exporters: [otlphttp]
    logs:
      receivers: [anthropic]
      processors: [batch]
      exporters: [otlphttp]
```

> **Note:** The `deltatocumulative` processor is required for Grafana Cloud and other Prometheus-compatible backends that expect cumulative temporality for histogram and counter metrics.

Generate the `Authorization` header value:

```bash
echo -n "<instance_id>:<api_token>" | base64
```

### 3. Import dashboards

Import the pre-built Grafana dashboards from the `dashboard/dist/` directory into your Grafana Cloud instance:

1. Open your Grafana Cloud instance
2. Go to **Dashboards > New > Import**
3. Upload `dashboard/dist/anthropic-claude-code-usage.json`
4. Repeat for `dashboard/dist/claude-code-developer-activity.json`

## Configuration

### Multi-Provider Configuration (OpenClaw)

For multi-provider setups with OpenClaw, use the full provider configuration:

```yaml
receivers:
  openclaw:
    endpoint: "127.0.0.1:4319"

    # Provider configurations
    providers:
      anthropic:
        enabled: true
        base_url: "https://api.anthropic.com"
        default_model: "claude-sonnet-4-6"

      openai:
        enabled: true
        base_url: "https://api.openai.com"
        default_model: "gpt-4o"

      gemini:
        enabled: true
        base_url: "https://generativelanguage.googleapis.com"
        default_model: "gemini-2.0-flash"

    # OpenClaw-specific settings
    openclaw:
      extract_context: true
      workspace_aliases:
        "/Users/dev/openclaw": "openclaw-core"
        "/Users/dev/workspace": "dev-workspace"

    # Telemetry settings
    capture_request_body: false
    capture_response_body: false
    max_body_capture_size: 65536
    redact_api_key: true
    rate_limit_warning_threshold: 0.8
    parse_tool_calls: true
    include_file_path_label: false

    # Per-model pricing (USD per million tokens)
    pricing:
      "claude-opus-4-6":
        input_per_m_token: 5.0
        output_per_m_token: 25.0
        cache_read_per_m_token: 0.50
        cache_creation_per_m_token: 6.25
      "gpt-4o":
        input_per_m_token: 2.50
        output_per_m_token: 10.0
        cache_read_per_m_token: 1.25
        cache_creation_per_m_token: 2.50
      "gemini-2.0-flash":
        input_per_m_token: 0.10
        output_per_m_token: 0.40
```

### Single-Provider Configuration (Anthropic Only)

For Anthropic-only use, the existing configuration continues to work with no changes:

```yaml
receivers:
  anthropic:
    # HTTP server endpoint (default: 127.0.0.1:4319)
    endpoint: "127.0.0.1:4319"

    # Upstream Anthropic API URL
    anthropic_api: "https://api.anthropic.com"

    # Log full request/response bodies (disabled by default)
    capture_request_body: false
    capture_response_body: false
    max_body_capture_size: 65536

    # Max size in bytes for incoming request bodies (default: 10MB)
    max_request_body_size: 10485760

    # Redact API keys in log output (enabled by default)
    redact_api_key: true

    # Emit warnings when rate limit utilization exceeds this ratio (0-1)
    rate_limit_warning_threshold: 0.8

    # Parse tool_use content blocks for code-level metrics (enabled by default)
    parse_tool_calls: true

    # Include full file paths as metric labels (disabled by default, high cardinality)
    include_file_path_label: false

    # Cost per 1000 web search requests in USD (default: 10.0)
    web_search_price_per_1000: 10.0

    # Per-model pricing for cost calculation (USD per million tokens)
    # Defaults are included for current Claude models. Override or extend as needed:
    # pricing:
    #   claude-opus-4-6:
    #     input_per_m_token: 5.0
    #     output_per_m_token: 25.0
    #     cache_read_per_m_token: 0.50
    #     cache_creation_per_m_token: 6.25
```

### Pipelines

The receiver produces traces, metrics, and logs. Connect it to any combination of exporters:

```yaml
service:
  pipelines:
    traces:
      receivers: [anthropic]
      processors: [batch]
      exporters: [otlp_grpc]
    metrics:
      receivers: [anthropic]
      processors: [batch]
      exporters: [otlp_grpc]
    logs:
      receivers: [anthropic]
      processors: [batch]
      exporters: [otlp_grpc]
```

## Provider Adapter Architecture

The collector uses a pluggable adapter framework for multi-provider support:

```
receiver/anthropicreceiver/
├── adapter/
│   ├── provider.go              # ProviderAdapter interface
│   ├── registry.go              # Provider detection & routing
│   ├── anthropic/adapter.go     # Anthropic Messages API
│   ├── openai/adapter.go        # OpenAI Chat Completions API
│   └── gemini/adapter.go        # Google Gemini API
└── openclaw/
    ├── context.go               # X-OpenClaw-* header extraction
    └── attributes.go            # OTel attribute mapping
```

Each adapter implements the `ProviderAdapter` interface:

```go
type ProviderAdapter interface {
    Name() string
    Detect(req *http.Request, body []byte) bool
    ParseRequest(body []byte) (*LLMRequest, error)
    ParseResponse(body []byte, isStreaming bool) (*LLMResponse, error)
    ExtractUsage(resp *LLMResponse) Usage
    CalculateCost(usage Usage, model string, pricing map[string]ModelPricing) float64
    GetUpstreamURL() string
    ExtractContext(req *LLMRequest) map[string]string
}
```

To add a new provider, implement this interface and register it with the adapter registry.

## Dashboards

Two pre-built Grafana dashboards are included and auto-provisioned when using Docker Compose.

### Claude Code Usage

**File:** `dashboard/dist/anthropic-claude-code-usage.json`

Tracks API usage with a focus on Claude Code projects: request volume, error rates, token usage breakdown, cost tracking per project, cache hit ratio, rate limit utilization, streaming performance, and model/speed mode breakdown.

### Developer Activity

**File:** `dashboard/dist/claude-code-developer-activity.json`

Tracks developer workflows and tool usage patterns: tool call frequency by type, file operations with lines changed, file types affected, and session depth metrics.

See [docs/images/](docs/images/) for dashboard screenshots.

### Regenerating dashboards

Dashboards are generated from Go code in the `dashboard/` directory:

```bash
make dashboard
```

This regenerates the JSON files in `dashboard/dist/`.

## Telemetry Reference

The receiver emits traces, metrics, and logs for every API call.
See [docs/telemetry.md](docs/telemetry.md) for the full reference
including span attributes, all 70+ metrics, log types, and default pricing.

## Development

```bash
make build          # Build the collector binary
make run            # Build and run with collector-config.yaml
make test           # Run tests with race detection
make lint           # Run go vet
make clean          # Remove build artifacts
make dashboard      # Generate Grafana dashboards
```

## Components

This collector is built with the [OpenTelemetry Collector Builder](https://opentelemetry.io/docs/collector/custom-collector/) (v0.146.1) and includes:

| Component           | Type      | Description                                        |
| ------------------- | --------- | -------------------------------------------------- |
| `anthropicreceiver` | Receiver  | Multi-provider LLM API reverse proxy with telemetry capture |
| `debugexporter`     | Exporter  | Prints telemetry to stdout                         |
| `otlpexporter`      | Exporter  | Sends telemetry via gRPC                           |
| `otlphttpexporter`  | Exporter  | Sends telemetry via HTTP                           |
| `batchprocessor`    | Processor | Batches data before export                         |

## Migration Path

### For Existing Anthropic-Only Users

No breaking changes — the collector continues to work as before with the `anthropic` receiver key:

```yaml
receivers:
  anthropic:
    endpoint: "127.0.0.1:4319"
    anthropic_api: "https://api.anthropic.com"
```

### For OpenClaw Users

Enable multi-provider support and context extraction:

```yaml
receivers:
  openclaw:
    endpoint: "127.0.0.1:4319"
    providers:
      anthropic:
        enabled: true
        base_url: "https://api.anthropic.com"
      openai:
        enabled: true
        base_url: "https://api.openai.com"
    openclaw:
      extract_context: true
```

## Benefits

1. **Single observability pipeline** for all LLM providers
2. **Unified cost tracking** across Anthropic, OpenAI, and Gemini
3. **OpenClaw-specific insights** — agent performance, channel usage, workspace activity
4. **Provider comparison** — cost, latency, and quality side-by-side
5. **No code changes** to OpenClaw core — just configuration
6. **Production-ready telemetry** — fixes for histogram encoding, trace propagation, and security controls
7. **End-to-end trace visibility** — full request flow from OpenClaw to Collector to Provider APIs

## License

See [LICENSE](LICENSE) for details.
