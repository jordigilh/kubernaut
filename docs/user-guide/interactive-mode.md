# Interactive Mode User Guide

## Overview

Interactive Mode allows SRE engineers to collaborate with the Kubernaut Agent in
real-time during incident investigations. Instead of relying solely on autonomous
analysis, you can guide the agent's investigation, ask follow-up questions, and
direct tool execution — all while Kubernaut enforces your Kubernetes RBAC.

## Prerequisites

- Kubernaut Agent v1.5+ deployed with `interactive.enabled: true`
- An MCP-compatible client (IDE plugin, CLI, or web UI)
- A valid Kubernetes bearer token with access to the `kubernaut-agent` service

## Connecting

Interactive mode is available at the MCP endpoint:

```
POST /api/v1/mcp
Authorization: Bearer <your-k8s-token>
```

The endpoint implements the [Model Context Protocol](https://spec.modelcontextprotocol.io/)
Streamable HTTP transport. Any MCP SDK client can connect.

## Session Lifecycle

### 1. Start or Takeover

Begin an interactive session for a specific remediation request:

```json
{
  "action": "start",
  "rr_id": "rr-abc-123"
}
```

If an autonomous investigation is already running, use `takeover` to cancel it
and begin driving interactively:

```json
{
  "action": "takeover",
  "rr_id": "rr-abc-123"
}
```

The response includes reconstructed conversation history from prior investigation turns.

### 2. Send Messages

Once you hold the session, send questions or instructions:

```json
{
  "action": "message",
  "rr_id": "rr-abc-123",
  "message": "What pods are crash-looping in the payment namespace?"
}
```

The agent executes Kubernetes API calls **as your identity** (via impersonation),
ensuring RBAC is enforced.

### 3. Check Status

Query the current state of any remediation:

```json
{
  "action": "status",
  "rr_id": "rr-abc-123"
}
```

Returns `autonomous`, `interactive` (with driver username), or `not_found`.

### 4. Complete or Cancel

Explicitly end your session:

```json
{
  "action": "complete",
  "rr_id": "rr-abc-123"
}
```

Or cancel without saving:

```json
{
  "action": "cancel",
  "rr_id": "rr-abc-123"
}
```

## Timeouts and Warnings

- **Inactivity timeout**: Sessions are automatically released after a period of
  inactivity (default: 10 minutes, configurable via `inactivityTimeout`).
- **Warnings**: You'll receive MCP log notifications (level: `warning`) at
  configurable intervals before the timeout fires (e.g., 2 minutes and 30 seconds
  before expiry).
- **Session TTL**: Hard limit on session duration (default: 30 minutes).

## Rate Limiting

To prevent abuse, messages are rate-limited per session (default: 10 messages/minute).
Exceeding the limit returns a `rate_limited` error. Wait and retry.

## Security Model

- **Authentication**: All requests require a valid Kubernetes bearer token.
- **Authorization**: Token must pass a SubjectAccessReview (SAR) check.
- **Impersonation**: All Kubernetes API calls made during your session use your
  identity. You can only see/modify resources your RBAC permits.
- **Driver exclusivity**: Only the active session driver can send messages,
  complete, or cancel a session. Other users see `session_active` errors.
- **Header stripping**: Client-supplied `Impersonate-*` headers are stripped
  before processing to prevent privilege escalation.

## Error Codes

| Code | Meaning |
|------|---------|
| `session_active` | Another user holds the session (includes driver username) |
| `not_driving` | Must start/takeover before sending messages |
| `not_found` | No active investigation for this remediation |
| `rate_limited` | Too many requests; slow down |
| `session_expired` | Session expired due to TTL or inactivity |
| `investigation_completed` | Autonomous investigation already finished |
| `max_sessions` | Server has reached maximum concurrent sessions |

## Helm Configuration

```yaml
kubernautAgent:
  interactive:
    enabled: true
    sessionTTL: "30m"
    inactivityTimeout: "10m"
    maxConcurrentSessions: 5
    rateLimitPerUser: 10
```

## Disconnect Behavior

If your MCP connection drops (network issue, client crash):

1. The session is automatically released after disconnect detection.
2. A background reconstruction process captures investigation context.
3. You can reconnect and `takeover` to resume where you left off.
