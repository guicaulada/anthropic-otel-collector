# Anthropic OTel Collector

A custom [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/) that acts as a transparent reverse proxy in front of the Anthropic API, capturing comprehensive telemetry — traces, metrics, and logs — for every API call.

Point your Anthropic SDK at the collector instead of `https://api.anthropic.com` and get full observability with zero code changes.

## How It Works

```
┌──────────────┐       ┌─────────────────────┐       ┌───────────────────┐
│  Anthropic   │──────▶│   OTel Collector    │──────▶│  Anthropic API    │
│  SDK Client  │◀──────│   (reverse proxy)   │◀──────│  api.anthropic.com│
└──────────────┘       └──────────┬──────────┘       └───────────────────┘
                                  │
                        traces, metrics, logs
                                  │
                                  ▼
                       ┌─────────────────────┐
                       │   OTLP Backend      │
                       │   (Grafana, etc.)   │
                       └─────────────────────┘
```

The collector intercepts all traffic between your application and the Anthropic API. It parses requests and responses (including SSE streams), extracts telemetry, and forwards everything to your observability backend via OTLP. The proxy is fully transparent — your application receives the original API response unmodified.

When used with Claude Code, the collector automatically detects requests and extracts **project** context — grouping related API calls by working directory. This gives you cost, token, and request metrics broken down by project with zero configuration.

## Quick Start

### Docker Compose (recommended)

Starts the collector alongside a [Grafana LGTM](https://github.com/grafana/docker-otel-lgtm) stack (Loki, Grafana, Tempo, Mimir) for instant dashboarding:

```bash
docker compose up -d
```

This exposes:

| Port | Service                                  |
| ---- | ---------------------------------------- |
| 4319 | Anthropic receiver (point your SDK here) |
| 3000 | Grafana UI                               |
| 4317 | OTLP gRPC                                |
| 4318 | OTLP HTTP                                |

Then configure your Anthropic SDK to use the proxy:

```python
import anthropic

client = anthropic.Anthropic(
    base_url="http://localhost:4319",
)
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

### Docker Build

```bash
make docker-build
docker run -p 4319:4319 -v ./collector-config.yaml:/etc/otelcol/config.yaml:ro anthropic-otel-collector:latest
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
    endpoint: "0.0.0.0:4319"

exporters:
  otlphttp:
    endpoint: "https://otlp-gateway-prod-us-east-0.grafana.net/otlp"
    headers:
      Authorization: "Basic <base64-encoded instance_id:api_token>"

processors:
  batch: {}

service:
  pipelines:
    traces:
      receivers: [anthropic]
      processors: [batch]
      exporters: [otlphttp]
    metrics:
      receivers: [anthropic]
      processors: [batch]
      exporters: [otlphttp]
    logs:
      receivers: [anthropic]
      processors: [batch]
      exporters: [otlphttp]
```

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

The receiver is configured in the collector YAML under the `anthropic` key:

```yaml
receivers:
  anthropic:
    # HTTP server endpoint
    endpoint: "0.0.0.0:4319"

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

## Dashboards

Two pre-built Grafana dashboards are included and auto-provisioned when using Docker Compose.

### Claude Code Usage

**File:** `dashboard/dist/anthropic-claude-code-usage.json`

Tracks API usage with a focus on Claude Code projects:

- Request volume, error rates, and active requests
- Token usage breakdown (input, output, cache read, cache creation)
- Cost tracking per request and per project
- Cache hit ratio and savings
- Rate limit utilization
- Streaming performance (time-to-first-token, throughput)
- Model and speed mode breakdown

![Claude Code Usage Dashboard](docs/images/dashboard-usage.png)

### Developer Activity

**File:** `dashboard/dist/claude-code-developer-activity.json`

Tracks developer workflows and tool usage patterns:

- Tool call frequency by type (Edit, Write, Read, Bash, Glob, Grep)
- File operations (edits, creates, reads) with lines changed
- File types affected by operations
- Session and conversation depth metrics

![Developer Activity Dashboard](docs/images/dashboard-activity.png)

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
make build      # Build the collector binary
make run        # Build and run with collector-config.yaml
make test       # Run tests with race detection
make lint       # Run go vet
make clean      # Remove build artifacts
make dashboard  # Generate Grafana dashboards
```

## Components

This collector is built with the [OpenTelemetry Collector Builder](https://opentelemetry.io/docs/collector/custom-collector/) (v0.146.1) and includes:

| Component           | Type      | Description                                        |
| ------------------- | --------- | -------------------------------------------------- |
| `anthropicreceiver` | Receiver  | Anthropic API reverse proxy with telemetry capture |
| `debugexporter`     | Exporter  | Prints telemetry to stdout                         |
| `otlpexporter`      | Exporter  | Sends telemetry via gRPC                           |
| `otlphttpexporter`  | Exporter  | Sends telemetry via HTTP                           |
| `batchprocessor`    | Processor | Batches data before export                         |

## License

See [LICENSE](LICENSE) for details.
