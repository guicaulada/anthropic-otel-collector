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

Open [http://localhost:3000](http://localhost:3000) to explore traces, metrics, and logs in Grafana.

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

    # Redact API keys in log output (enabled by default)
    redact_api_key: true

    # Emit warnings when rate limit utilization exceeds this ratio (0-1)
    rate_limit_warning_threshold: 0.8

    # Parse tool_use content blocks for code-level metrics (enabled by default)
    parse_tool_calls: true

    # Include full file paths as metric labels (disabled by default, high cardinality)
    include_file_path_label: false

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

## Telemetry Reference

### Traces

Every API call produces a trace with two spans:

| Span                | Kind   | Description                                   |
| ------------------- | ------ | --------------------------------------------- |
| `chat {model}`      | Client | Root span covering the full request lifecycle |
| `POST /v1/messages` | Client | Child span covering the upstream API call     |

#### Span Attributes

**Standard attributes** on the root span:

| Attribute                                  | Description             |
| ------------------------------------------ | ----------------------- |
| `gen_ai.operation.name`                    | Always `"chat"`         |
| `gen_ai.provider.name`                     | Always `"anthropic"`    |
| `gen_ai.request.model`                     | Requested model name    |
| `gen_ai.response.model`                    | Actual model used       |
| `gen_ai.response.id`                       | Response ID             |
| `gen_ai.response.finish_reasons`           | Stop reason             |
| `gen_ai.usage.input_tokens`                | Input token count       |
| `gen_ai.usage.output_tokens`               | Output token count      |
| `gen_ai.usage.cache_read.input_tokens`     | Cache read tokens       |
| `gen_ai.usage.cache_creation.input_tokens` | Cache creation tokens   |
| `gen_ai.request.max_tokens`                | Max tokens requested    |
| `gen_ai.request.temperature`               | Temperature (if set)    |
| `gen_ai.request.top_p`                     | Top-p (if set)          |
| `gen_ai.request.top_k`                     | Top-k (if set)          |
| `http.request.method`                      | Always `"POST"`         |
| `http.response.status_code`                | HTTP status code        |
| `http.request.body.size`                   | Request body bytes      |
| `http.response.body.size`                  | Response body bytes     |
| `server.address`                           | API host                |
| `server.port`                              | API port                |
| `url.path`                                 | Always `"/v1/messages"` |

**Anthropic-specific attributes:**

| Attribute                                  | Description                           |
| ------------------------------------------ | ------------------------------------- |
| `anthropic.request_id`                     | Request ID from response headers      |
| `anthropic.api_key_hash`                   | Truncated SHA-256 hash of the API key |
| `anthropic.request.streaming`              | Whether the request uses streaming    |
| `anthropic.upstream.latency_ms`            | Upstream call latency in milliseconds |
| `anthropic.request.messages_count`         | Number of messages in the request     |
| `anthropic.request.{role}_messages_count`  | Messages per role (user, assistant)   |
| `anthropic.request.has_system_prompt`      | Whether a system prompt is present    |
| `anthropic.request.system_prompt.size`     | System prompt character count         |
| `anthropic.request.tools_count`            | Number of tools provided              |
| `anthropic.request.stop_sequences_count`   | Number of stop sequences              |
| `anthropic.request.thinking.enabled`       | Whether extended thinking is enabled  |
| `anthropic.request.thinking.budget_tokens` | Thinking budget token limit           |
| `anthropic.usage.total_input_tokens`       | All input tokens including cache      |
| `anthropic.cache.hit_ratio`                | Cache read / total input ratio        |
| `anthropic.response.content_blocks_count`  | Number of content blocks              |
| `anthropic.response.text_length`           | Response text character count         |
| `anthropic.response.tool_calls_count`      | Number of tool calls                  |
| `anthropic.response.thinking_length`       | Thinking text character count         |

**Rate limit attributes** (from response headers):

| Attribute                                     | Description                              |
| --------------------------------------------- | ---------------------------------------- |
| `anthropic.ratelimit.requests.limit`          | Request rate limit                       |
| `anthropic.ratelimit.requests.remaining`      | Remaining requests                       |
| `anthropic.ratelimit.input_tokens.limit`      | Input token rate limit                   |
| `anthropic.ratelimit.input_tokens.remaining`  | Remaining input tokens                   |
| `anthropic.ratelimit.output_tokens.limit`     | Output token rate limit                  |
| `anthropic.ratelimit.output_tokens.remaining` | Remaining output tokens                  |
| `anthropic.ratelimit.requests.reset`          | RFC 3339 reset time for requests         |
| `anthropic.ratelimit.input_tokens.reset`      | RFC 3339 reset time for input tokens     |
| `anthropic.ratelimit.output_tokens.reset`     | RFC 3339 reset time for output tokens    |
| `anthropic.ratelimit.tokens.limit`            | Unified token limit                      |
| `anthropic.ratelimit.tokens.remaining`        | Unified remaining tokens                 |
| `anthropic.ratelimit.retry_after`             | Retry-after seconds (429 responses)      |
| `anthropic.ratelimit.unified_status`          | Rate limit status (allowed/rate_limited) |

**Speed and cost attributes:**

| Attribute                                                 | Description                                                   |
| --------------------------------------------------------- | ------------------------------------------------------------- |
| `anthropic.usage.speed`                                   | Speed mode (fast/standard)                                    |
| `anthropic.usage.server_tool_use.web_search_requests`     | Server web search request count                               |
| `anthropic.usage.server_tool_use.web_fetch_requests`      | Server web fetch request count                                |
| `anthropic.usage.server_tool_use.code_execution_requests` | Server code execution count                                   |
| `anthropic.request.beta_features`                         | Beta features header value                                    |
| `anthropic.organization_id`                               | Organization ID from response headers                         |
| `anthropic.cost.multiplier`                               | Cost multiplier applied (fast/long_context/fast+long_context) |

**Streaming attributes** (only for streaming requests):

| Attribute                                    | Description                   |
| -------------------------------------------- | ----------------------------- |
| `anthropic.streaming.time_to_first_token_ms` | Time to first token in ms     |
| `anthropic.streaming.total_chunks`           | Number of text delta chunks   |
| `anthropic.streaming.total_events`           | Total SSE events received     |
| `anthropic.streaming.avg_time_per_token_ms`  | Average time per output token |

#### Span Events

| Event                            | Description                                                                                                           |
| -------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `gen_ai.request`                 | Emitted at request time with model and message count                                                                  |
| `gen_ai.response`                | Emitted at response time with response ID and finish reason                                                           |
| `gen_ai.content_block`           | One per content block (text, tool_use, thinking, server_tool_use, web_search_tool_result, code_execution_tool_result) |
| `gen_ai.tool_call`               | One per tool_use block with tool name and call ID                                                                     |
| `gen_ai.thinking`                | One per thinking block with character length                                                                          |
| `gen_ai.error`                   | On HTTP errors (status >= 400) with error type and message                                                            |
| `anthropic.tool_use.file_edit`   | Edit tool calls with file path, lines added/removed                                                                   |
| `anthropic.tool_use.file_create` | Write tool calls with file path and content size                                                                      |
| `anthropic.tool_use.file_read`   | Read tool calls with file path                                                                                        |
| `anthropic.tool_use.bash`        | Bash tool calls with command preview                                                                                  |
| `anthropic.tool_use.glob`        | Glob tool calls with pattern                                                                                          |
| `anthropic.tool_use.grep`        | Grep tool calls with pattern                                                                                          |
| `anthropic.rate_limit_warning`   | When utilization exceeds the configured threshold                                                                     |
| `anthropic.stream.first_token`   | When the first token is received (streaming)                                                                          |
| `anthropic.stream.complete`      | When the stream ends with event/chunk counts                                                                          |
| `anthropic.cost`                 | Cost breakdown in USD (input, output, cache, total)                                                                   |

### Metrics

All metrics share these common attributes: `gen_ai.operation.name`, `gen_ai.provider.name`, `gen_ai.request.model`, `gen_ai.response.model`, `http.response.status_code`, `anthropic.request.streaming`, `anthropic.api_key_hash`, `server.address`, `server.port`.

#### Latency & Duration

| Metric                                | Type      | Unit | Description                                    |
| ------------------------------------- | --------- | ---- | ---------------------------------------------- |
| `gen_ai.client.operation.duration`    | Histogram | s    | End-to-end request duration                    |
| `gen_ai.server.time_to_first_token`   | Histogram | s    | Time to first token (streaming only)           |
| `gen_ai.server.time_per_output_token` | Histogram | s    | Average time per output token (streaming only) |
| `anthropic.upstream.latency`          | Histogram | s    | Proxy-to-API upstream latency                  |
| `anthropic.streaming.duration`        | Histogram | s    | Total streaming duration                       |

#### Tokens

| Metric                            | Type      | Unit  | Description                                           |
| --------------------------------- | --------- | ----- | ----------------------------------------------------- |
| `gen_ai.client.token.usage`       | Histogram | token | Token count (by `gen_ai.token.type`: input or output) |
| `anthropic.tokens.input`          | Sum       | token | Cumulative input tokens                               |
| `anthropic.tokens.output`         | Sum       | token | Cumulative output tokens                              |
| `anthropic.tokens.cache_read`     | Sum       | token | Cumulative cache read tokens                          |
| `anthropic.tokens.cache_creation` | Sum       | token | Cumulative cache creation tokens                      |
| `anthropic.tokens.total_input`    | Sum       | token | Cumulative total input tokens (includes cache)        |

#### Cache

| Metric                      | Type  | Unit  | Description                                  |
| --------------------------- | ----- | ----- | -------------------------------------------- |
| `anthropic.cache.hit_ratio` | Gauge | ratio | Cache read tokens / total input tokens (0-1) |

#### Requests

| Metric                  | Type | Unit    | Description                      |
| ----------------------- | ---- | ------- | -------------------------------- |
| `anthropic.requests`    | Sum  | request | Total request count              |
| `anthropic.errors`      | Sum  | error   | Error count (HTTP status >= 400) |
| `anthropic.stop_reason` | Sum  | request | Request count by stop reason     |

#### Body Size

| Metric                         | Type      | Unit | Description                 |
| ------------------------------ | --------- | ---- | --------------------------- |
| `anthropic.request.body.size`  | Histogram | By   | Request body size in bytes  |
| `anthropic.response.body.size` | Histogram | By   | Response body size in bytes |

#### Response Content

| Metric                             | Type      | Unit  | Description                                       |
| ---------------------------------- | --------- | ----- | ------------------------------------------------- |
| `anthropic.content_blocks`         | Sum       | block | Content blocks by type (text, tool_use, thinking) |
| `anthropic.response.text_length`   | Histogram | char  | Response text character count                     |
| `anthropic.thinking.output_length` | Histogram | char  | Thinking text character count                     |

#### Request Parameters

| Metric                                 | Type      | Unit    | Description                   |
| -------------------------------------- | --------- | ------- | ----------------------------- |
| `anthropic.request.max_tokens`         | Histogram | token   | Requested max tokens          |
| `anthropic.request.temperature`        | Histogram |         | Temperature value (when set)  |
| `anthropic.request.messages_count`     | Histogram | message | Messages in the request       |
| `anthropic.request.system_prompt.size` | Histogram | char    | System prompt character count |
| `anthropic.request.tools_count`        | Histogram | tool    | Number of tools provided      |

#### Extended Thinking

| Metric                             | Type      | Unit    | Description                    |
| ---------------------------------- | --------- | ------- | ------------------------------ |
| `anthropic.thinking.enabled`       | Sum       | request | Requests with thinking enabled |
| `anthropic.thinking.budget_tokens` | Histogram | token   | Thinking budget token limit    |

#### Rate Limits

| Metric                                          | Type  | Unit    | Description                    |
| ----------------------------------------------- | ----- | ------- | ------------------------------ |
| `anthropic.ratelimit.requests.limit`            | Gauge | request | Request rate limit             |
| `anthropic.ratelimit.requests.remaining`        | Gauge | request | Remaining requests             |
| `anthropic.ratelimit.requests.utilization`      | Gauge | ratio   | Request utilization (0-1)      |
| `anthropic.ratelimit.input_tokens.limit`        | Gauge | token   | Input token rate limit         |
| `anthropic.ratelimit.input_tokens.remaining`    | Gauge | token   | Remaining input tokens         |
| `anthropic.ratelimit.input_tokens.utilization`  | Gauge | ratio   | Input token utilization (0-1)  |
| `anthropic.ratelimit.output_tokens.limit`       | Gauge | token   | Output token rate limit        |
| `anthropic.ratelimit.output_tokens.remaining`   | Gauge | token   | Remaining output tokens        |
| `anthropic.ratelimit.output_tokens.utilization` | Gauge | ratio   | Output token utilization (0-1) |

#### Streaming

| Metric                                       | Type      | Unit  | Description                   |
| -------------------------------------------- | --------- | ----- | ----------------------------- |
| `anthropic.streaming.events`                 | Sum       | event | SSE events by `event_type`    |
| `anthropic.streaming.chunks`                 | Histogram | chunk | Text delta chunks per request |
| `anthropic.streaming.content_block.duration` | Histogram | s     | Duration per content block    |

#### Cost

| Metric                                      | Type      | Unit    | Description                                                        |
| ------------------------------------------- | --------- | ------- | ------------------------------------------------------------------ |
| `anthropic.cost.request`                    | Histogram | USD     | Total cost per request                                             |
| `anthropic.cost.input_tokens`               | Sum       | USD     | Cumulative input token cost                                        |
| `anthropic.cost.output_tokens`              | Sum       | USD     | Cumulative output token cost                                       |
| `anthropic.cost.cache_read`                 | Sum       | USD     | Cumulative cache read cost                                         |
| `anthropic.cost.cache_creation`             | Sum       | USD     | Cumulative cache creation cost                                     |
| `anthropic.cost.total`                      | Sum       | USD     | Cumulative total cost                                              |
| `anthropic.cost.server_tool_use.web_search` | Sum       | USD     | Cumulative web search cost ($10/1000 searches)                     |
| `anthropic.cost.multiplied_requests`        | Sum       | request | Requests with non-standard cost multiplier (by `multiplier` label) |

#### Server Tool Use

| Metric                                              | Type | Unit    | Description                         |
| --------------------------------------------------- | ---- | ------- | ----------------------------------- |
| `anthropic.server_tool_use.web_search_requests`     | Sum  | request | Server-side web search requests     |
| `anthropic.server_tool_use.web_fetch_requests`      | Sum  | request | Server-side web fetch requests      |
| `anthropic.server_tool_use.code_execution_requests` | Sum  | request | Server-side code execution requests |

#### Speed & Throughput

| Metric                                          | Type  | Unit    | Description                               |
| ----------------------------------------------- | ----- | ------- | ----------------------------------------- |
| `anthropic.requests.by_speed`                   | Sum   | request | Requests by speed mode (by `speed` label) |
| `anthropic.throughput.output_tokens_per_second` | Gauge | token/s | Output token generation throughput        |

#### Tool Use

These metrics are emitted when `parse_tool_calls` is enabled (default) and the model uses tools like Edit, Write, Read, Bash, Glob, or Grep.

| Metric                             | Type      | Unit      | Description                                       |
| ---------------------------------- | --------- | --------- | ------------------------------------------------- |
| `anthropic.tool_calls`             | Sum       | call      | Tool calls by `tool.name`                         |
| `anthropic.tool_use.calls`         | Sum       | call      | Parsed tool calls by `tool.name`                  |
| `anthropic.tool_use.file_edits`    | Sum       | edit      | File edit operations                              |
| `anthropic.tool_use.lines_added`   | Sum       | line      | Lines added across edits                          |
| `anthropic.tool_use.lines_removed` | Sum       | line      | Lines removed across edits                        |
| `anthropic.tool_use.lines_changed` | Sum       | line      | Total lines changed (added + removed)             |
| `anthropic.tool_use.edit_size`     | Histogram | char      | Edit operation size (old + new string bytes)      |
| `anthropic.tool_use.file_creates`  | Sum       | file      | File create operations                            |
| `anthropic.tool_use.write_size`    | Histogram | char      | Written content size                              |
| `anthropic.tool_use.file_reads`    | Sum       | read      | File read operations                              |
| `anthropic.tool_use.bash_commands` | Sum       | command   | Bash commands executed                            |
| `anthropic.tool_use.glob_searches` | Sum       | search    | Glob searches                                     |
| `anthropic.tool_use.grep_searches` | Sum       | search    | Grep searches                                     |
| `anthropic.tool_use.files_touched` | Sum       | file      | Unique files touched (edit/write/read)            |
| `anthropic.tool_use.file_type`     | Sum       | operation | Operations by `file.extension` and operation type |

### Logs

| Log                 | Severity   | Description                                                |
| ------------------- | ---------- | ---------------------------------------------------------- |
| Operation log       | INFO/ERROR | Emitted for every request with full metadata               |
| Request body        | DEBUG      | Raw request JSON (requires `capture_request_body: true`)   |
| Response body       | DEBUG      | Raw response JSON (requires `capture_response_body: true`) |
| Error               | ERROR      | Error type and message for failed requests                 |
| Rate limit warning  | WARN       | When utilization exceeds the configured threshold          |
| Tool call           | INFO       | One per tool_use block with tool name and call ID          |
| Detailed tool call  | INFO       | Parsed tool call details (file paths, patterns, sizes)     |
| File change         | INFO       | File modification summary with lines added/removed         |
| Cost summary        | INFO       | Cost breakdown in USD                                      |
| Streaming summary   | INFO       | Event count, chunk count, time-to-first-token              |
| Notable stop reason | WARN       | Safety refusal, turn paused, or context window exceeded    |

## Default Pricing

Built-in pricing table (USD per million tokens):

| Model                        | Input | Output | Cache Read | Cache Creation |
| ---------------------------- | ----- | ------ | ---------- | -------------- |
| `claude-opus-4-6`            | $5.00 | $25.00 | $0.50      | $6.25          |
| `claude-sonnet-4-6`          | $3.00 | $15.00 | $0.30      | $3.75          |
| `claude-haiku-4-5-20251001`  | $1.00 | $5.00  | $0.10      | $1.25          |
| `claude-3-5-sonnet-20241022` | $3.00 | $15.00 | $0.30      | $3.75          |
| `claude-3-5-haiku-20241022`  | $0.80 | $4.00  | $0.08      | $1.00          |

**Cost multipliers** are applied automatically:

- **Fast mode** (6x): All costs multiplied by 6 when `speed` is `"fast"`
- **Long context** (2x input, 1.5x output): Applied when total input tokens exceed 200,000
- Multipliers stack: fast + long context applies both

Override or extend pricing via the `pricing` configuration key.

## Development

```bash
make build    # Build the collector binary
make run      # Build and run with collector-config.yaml
make test     # Run tests with race detection
make lint     # Run go vet
make clean    # Remove build artifacts
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
