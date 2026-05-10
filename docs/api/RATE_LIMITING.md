# Rate Limiting

The API Frontend enforces rate limits at two levels: per-IP and per-user. Limits are configurable via the `rateLimit` section of `config.yaml`.

## Default Limits

| Scope | Limit | Burst | Description |
|-------|-------|-------|-------------|
| Per IP | 10 req/s | 20 | Protects against DDoS; applies before authentication |
| Per User — requests | 30 req/min | 30 | Total HTTP requests per authenticated user |
| Per User — tool calls | 60 calls/min | 60 | MCP `tools/call` invocations per user |
| Per User — sessions | 3 concurrent | — | Maximum concurrent MCP sessions per user |
| Per Session — tools | 10 concurrent | — | Maximum concurrent tool calls within a single session (semaphore) |

## Response Headers

When a request is rate-limited, the response includes:

| Header | Value | Description |
|--------|-------|-------------|
| `Retry-After` | seconds (integer) | Minimum time to wait before retrying |

## HTTP Status Codes

| Code | Meaning |
|------|---------|
| `429 Too Many Requests` | IP or user rate limit exceeded |
| MCP `isError: true` with `"server busy — too many concurrent tool calls, please retry"` | Per-session semaphore full |

## Retry Guidance

1. **On 429**: Wait for the `Retry-After` duration, then retry with exponential backoff
2. **On semaphore throttle**: Wait 1-2 seconds and retry; reduce concurrent tool calls
3. **Do not** retry immediately — the rate limiter uses a token bucket algorithm; immediate retries will continue to be rejected

## Configuration

```yaml
rateLimit:
  ipRequestsPerSec: 10
  ipBurst: 20
  userRequestsPerMin: 30
  userMaxConcurrentSessions: 3
  userToolCallsPerMin: 60
```

These values can be updated at runtime via config hot-reload (the service watches `config.yaml` for changes).

## MCP-Specific Behavior

- The per-session semaphore (`MaxConcurrentTools: 10`) limits how many tool calls can execute simultaneously within a single MCP session
- `tools/list` is not rate-limited (lightweight metadata operation)
- RBAC denials (HTTP 200 with `isError: true`) do not count against rate limits
