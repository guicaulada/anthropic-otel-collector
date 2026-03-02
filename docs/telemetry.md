# Telemetry Reference

The Anthropic OTel Collector receiver emits traces, metrics, and logs for every API call it proxies.

## Traces

Every API call produces a trace with two spans:

| Span                | Kind   | Description                                   |
| ------------------- | ------ | --------------------------------------------- |
| `chat {model}`      | Client | Root span covering the full request lifecycle |
| `POST /v1/messages` | Client | Child span covering the upstream API call     |

### Span Attributes

**Standard attributes** on the root span:

| Attribute                                  | Description             |
| ------------------------------------------ | ----------------------- |
| `gen_ai.operation.name`                    | Always `"chat"`         |
| `gen_ai.provider.name`                     | Always `"anthropic"`    |
| `gen_ai.request.model`                     | Requested model name    |
| `gen_ai.response.model`                    | Actual model used       |
| `gen_ai.response.id`                       | Response ID             |
| `gen_ai.response.finish_reasons`           | Stop reason             |
| `gen_ai.response.stop_sequence`            | Stop sequence matched (if applicable) |
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
| `anthropic.request.api_version`            | API version from request header       |
| `anthropic.upstream.latency_ms`            | Upstream call latency in milliseconds |
| `anthropic.request.messages_count`         | Number of messages in the request     |
| `anthropic.request.{role}_messages_count`  | Messages per role (user, assistant)   |
| `anthropic.request.has_system_prompt`      | Whether a system prompt is present    |
| `anthropic.request.system_prompt.size`     | System prompt character count         |
| `anthropic.request.tools_count`            | Number of tools provided              |
| `anthropic.request.tool_choice`            | Tool choice type (if set)             |
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
| `anthropic.cost.credit_usage_usd`                         | Credit usage from API response header                         |

**Container attributes** (when using container-based execution):

| Attribute                       | Description                      |
| ------------------------------- | -------------------------------- |
| `anthropic.container.id`        | Container ID                     |
| `anthropic.container.expires_at`| Container expiration timestamp   |

**Claude Code attributes** (only for Claude Code requests):

| Attribute                    | Description                                   |
| ---------------------------- | --------------------------------------------- |
| `claude_code.is_claude_code` | Always `true` for Claude Code requests        |
| `claude_code.project.path`   | Working directory path                        |
| `claude_code.project.name`   | Project directory name (base of path)         |
| `claude_code.user_id`        | User identifier from request metadata (if set)|

**Streaming attributes** (only for streaming requests):

| Attribute                                    | Description                   |
| -------------------------------------------- | ----------------------------- |
| `anthropic.streaming.time_to_first_token_ms` | Time to first token in ms     |
| `anthropic.streaming.total_chunks`           | Number of text delta chunks   |
| `anthropic.streaming.total_events`           | Total SSE events received     |
| `anthropic.streaming.avg_time_per_token_ms`  | Average time per output token |

### Span Events

| Event                            | Description                                                                                                           |
| -------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `gen_ai.request`                 | Emitted at request time with model and message count                                                                  |
| `gen_ai.response`                | Emitted at response time with response ID and finish reason                                                           |
| `gen_ai.content_block`           | One per content block (text, tool_use, thinking, redacted_thinking, server_tool_use, web_search_tool_result, code_execution_tool_result) |
| `gen_ai.tool_call`               | One per tool_use block with tool name and call ID                                                                     |
| `gen_ai.thinking`                | One per thinking block with character length                                                                          |
| `gen_ai.redacted_thinking`       | One per redacted thinking block with data length                                                                      |
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

## Metrics

All metrics share these common attributes: `gen_ai.operation.name`, `gen_ai.provider.name`, `gen_ai.request.model`, `gen_ai.response.model`, `http.response.status_code`, `anthropic.request.streaming`, `anthropic.api_key_hash`, `server.address`, `server.port`. For Claude Code requests, `claude_code.project.name` is also included in common attributes.

### Latency & Duration

| Metric                                | Type      | Unit | Description                                    |
| ------------------------------------- | --------- | ---- | ---------------------------------------------- |
| `gen_ai.client.operation.duration`    | Histogram | s    | End-to-end request duration                    |
| `gen_ai.server.time_to_first_token`   | Histogram | s    | Time to first token (streaming only)           |
| `gen_ai.server.time_per_output_token` | Histogram | s    | Average time per output token (streaming only) |
| `anthropic.upstream.latency`          | Histogram | s    | Proxy-to-API upstream latency                  |
| `anthropic.streaming.duration`        | Histogram | s    | Total streaming duration                       |

### Tokens

| Metric                                | Type      | Unit  | Description                                           |
| ------------------------------------- | --------- | ----- | ----------------------------------------------------- |
| `gen_ai.client.token.usage`           | Histogram | token | Token count (by `gen_ai.token.type`: input or output) |
| `anthropic.tokens.input`              | Sum       | token | Cumulative input tokens                               |
| `anthropic.tokens.output`             | Sum       | token | Cumulative output tokens                              |
| `anthropic.tokens.cache_read`         | Sum       | token | Cumulative cache read tokens                          |
| `anthropic.tokens.cache_creation`     | Sum       | token | Cumulative cache creation tokens                      |
| `anthropic.tokens.total_input`        | Sum       | token | Cumulative total input tokens (includes cache)        |
| `anthropic.tokens.output_utilization` | Gauge     | ratio | Output tokens / max tokens ratio (0-1)                |

### Cache

| Metric                      | Type  | Unit  | Description                                  |
| --------------------------- | ----- | ----- | -------------------------------------------- |
| `anthropic.cache.hit_ratio` | Gauge | ratio | Cache read tokens / total input tokens (0-1) |

### Requests

| Metric                     | Type  | Unit    | Description                                    |
| -------------------------- | ----- | ------- | ---------------------------------------------- |
| `anthropic.requests`       | Sum   | request | Total request count                            |
| `anthropic.requests.active`| Gauge | request | Currently active (in-flight) requests          |
| `anthropic.errors`         | Sum   | error   | Error count (HTTP status >= 400)               |
| `anthropic.errors.by_type` | Sum   | error   | Error count by `error.type` attribute          |
| `anthropic.stop_reason`    | Sum   | request | Request count by stop reason                   |

### Body Size

| Metric                         | Type      | Unit | Description                 |
| ------------------------------ | --------- | ---- | --------------------------- |
| `anthropic.request.body.size`  | Histogram | By   | Request body size in bytes  |
| `anthropic.response.body.size` | Histogram | By   | Response body size in bytes |

### Response Content

| Metric                             | Type      | Unit  | Description                                       |
| ---------------------------------- | --------- | ----- | ------------------------------------------------- |
| `anthropic.content_blocks`         | Sum       | block | Content blocks by type (text, tool_use, thinking) |
| `anthropic.response.text_length`   | Histogram | char  | Response text character count                     |
| `anthropic.thinking.output_length` | Histogram | char  | Thinking text character count                     |

### Request Parameters

| Metric                                 | Type      | Unit    | Description                              |
| -------------------------------------- | --------- | ------- | ---------------------------------------- |
| `anthropic.request.max_tokens`         | Histogram | token   | Requested max tokens                     |
| `anthropic.request.temperature`        | Histogram |         | Temperature value (when set)             |
| `anthropic.request.messages_count`     | Histogram | message | Messages in the request                  |
| `anthropic.request.system_prompt.size` | Histogram | char    | System prompt character count            |
| `anthropic.request.tools_count`        | Histogram | tool    | Number of tools provided                 |
| `anthropic.request.conversation_turns` | Histogram | turn    | Conversation depth (assistant turns > 0) |

### Extended Thinking

| Metric                             | Type      | Unit    | Description                    |
| ---------------------------------- | --------- | ------- | ------------------------------ |
| `anthropic.thinking.enabled`       | Sum       | request | Requests with thinking enabled            |
| `anthropic.thinking.budget_tokens` | Histogram | token   | Thinking budget token limit (only when > 0) |

### Rate Limits

| Metric                                          | Type  | Unit    | Description                    |
| ----------------------------------------------- | ----- | ------- | ------------------------------ |
| `anthropic.ratelimit.requests.limit`            | Gauge | request | Request rate limit             |
| `anthropic.ratelimit.requests.remaining`        | Gauge | request | Remaining requests             |
| `anthropic.ratelimit.requests.utilization`      | Gauge | ratio   | Request utilization (0-1)      |
| `anthropic.ratelimit.input_tokens.limit`        | Gauge | token   | Input token rate limit         |
| `anthropic.ratelimit.input_tokens.remaining`    | Gauge | token   | Remaining input tokens         |
| `anthropic.ratelimit.input_tokens.utilization`  | Gauge | ratio   | Input token utilization (0-1)  |
| `anthropic.ratelimit.output_tokens.limit`       | Gauge | token   | Output token rate limit (only when > 0)        |
| `anthropic.ratelimit.output_tokens.remaining`   | Gauge | token   | Remaining output tokens (only when limit > 0)  |
| `anthropic.ratelimit.output_tokens.utilization` | Gauge | ratio   | Output token utilization (only when limit > 0) |

### Streaming

| Metric                                       | Type      | Unit  | Description                   |
| -------------------------------------------- | --------- | ----- | ----------------------------- |
| `anthropic.streaming.events`                 | Sum       | event | SSE events by `event_type`    |
| `anthropic.streaming.chunks`                 | Histogram | chunk | Text delta chunks per request |
| `anthropic.streaming.content_block.duration` | Histogram | s     | Duration per content block    |

### Cost

| Metric                                      | Type      | Unit    | Description                                                        |
| ------------------------------------------- | --------- | ------- | ------------------------------------------------------------------ |
| `anthropic.cost.request`                    | Histogram | USD     | Total cost per request                                             |
| `anthropic.cost.input_tokens`               | Sum       | USD     | Cumulative input token cost                                        |
| `anthropic.cost.output_tokens`              | Sum       | USD     | Cumulative output token cost                                       |
| `anthropic.cost.cache_read`                 | Sum       | USD     | Cumulative cache read cost                                         |
| `anthropic.cost.cache_creation`             | Sum       | USD     | Cumulative cache creation cost                                     |
| `anthropic.cost.total`                      | Sum       | USD     | Cumulative total cost                                              |
| `anthropic.cost.credit_usage`               | Gauge     | USD     | Credit usage from API response header                              |
| `anthropic.cost.credit_usage.total`         | Sum       | USD     | Cumulative credit usage from API                                   |
| `anthropic.cost.cache_savings`              | Sum       | USD     | Cumulative cost savings from cache hits                            |
| `anthropic.cost.server_tool_use.web_search` | Sum       | USD     | Cumulative web search cost (configurable per 1000 searches)        |
| `anthropic.cost.multiplied_requests`        | Sum       | request | Requests with non-standard cost multiplier (by `multiplier` label) |

### Server Tool Use

| Metric                                              | Type | Unit    | Description                         |
| --------------------------------------------------- | ---- | ------- | ----------------------------------- |
| `anthropic.server_tool_use.web_search_requests`     | Sum  | request | Server-side web search requests     |
| `anthropic.server_tool_use.web_fetch_requests`      | Sum  | request | Server-side web fetch requests      |
| `anthropic.server_tool_use.code_execution_requests` | Sum  | request | Server-side code execution requests |

### Speed & Throughput

| Metric                             | Type  | Unit    | Description                               |
| ---------------------------------- | ----- | ------- | ----------------------------------------- |
| `anthropic.requests.by_speed`      | Sum   | request | Requests by speed mode (by `speed` label) |
| `anthropic.throughput.output_tokens`| Gauge | token/s | Output token generation throughput        |

### Tool Use

These metrics are emitted when `parse_tool_calls` is enabled (default) and the model uses tools like Edit, Write, Read, Bash, Glob, or Grep.

| Metric                             | Type      | Unit      | Description                                       |
| ---------------------------------- | --------- | --------- | ------------------------------------------------- |
| `anthropic.tool_use.calls`         | Sum       | call      | Tool calls by `tool.name`                         |
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

### Claude Code Projects

These metrics are emitted only for Claude Code requests with a detected project. They include `claude_code.project.name` as a label.

| Metric                              | Type | Unit    | Description                   |
| ----------------------------------- | ---- | ------- | ----------------------------- |
| `claude_code.project.requests`      | Sum  | request | Request count per project     |
| `claude_code.project.cost`          | Sum  | USD     | Cumulative cost per project   |
| `claude_code.project.tokens.input`  | Sum  | token   | Cumulative input per project  |
| `claude_code.project.tokens.output` | Sum  | token   | Cumulative output per project |
| `claude_code.project.errors`        | Sum  | error   | Error count per project       |

## Logs

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
