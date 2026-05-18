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
    sessionTTL: "30m"            # Max session duration before auto-release
    inactivityTimeout: "10m"     # Auto-release after last activity
    maxConcurrentSessions: 5     # Per-replica capacity
    rateLimitPerUser: 10         # MCP requests/second per user
    maxAnalyzingTimeout: "45m"   # Extended RO timeout while interactive session active
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Feature gate for the MCP interactive endpoint |
| `sessionTTL` | `30m` | Hard limit on session duration (max: 1h) |
| `inactivityTimeout` | `10m` | Auto-release after inactivity (max: 30m) |
| `maxConcurrentSessions` | `5` | Max concurrent sessions per agent replica (max: 100) |
| `rateLimitPerUser` | `10` | MCP requests per second per authenticated user (max: 100) |
| `maxAnalyzingTimeout` | `45m` | Extended analyzing timeout in Remediation Orchestrator while an interactive session is active, preventing RO from timing out the RR during operator investigation |

## Workflow Discovery and Selection

Interactive mode supports a structured workflow discovery and selection flow:

1. **Discover Workflows**: Call `kubernaut_investigate` with `action: "discover_workflows"` to trigger KA's Phase 3 LLM analysis. The response includes a recommended workflow and zero or more alternatives, each with:
   - `workflow_id` — unique identifier for catalog lookup
   - `confidence` — LLM confidence score (0-1)
   - `rationale` — why this workflow was recommended
   - `parameters` — LLM-populated execution parameters (e.g., `{"MEMORY_LIMIT_NEW": "512Mi"}`)

2. **Select Workflow**: Call `kubernaut_select_workflow` with the chosen `workflow_id`. The selected workflow's parameters are merged into the final investigation result and forwarded to the workflow execution engine.

3. **Complete Without Action**: Call `kubernaut_complete_no_action` if no workflow is appropriate.

Selecting a workflow auto-completes the interactive session: the final result is written to the HTTP session store, the MCP lease is released, and the AA controller transitions to completion.

For the detailed MCP tool contract including JSON schemas, parameter semantics, and gating rules, see [docs/mcp/discover-workflows-contract.md](../mcp/discover-workflows-contract.md).

## Disconnect Behavior

If your MCP connection drops (network issue, client crash):

1. The session is automatically released after disconnect detection.
2. A background reconstruction process captures investigation context.
3. You can reconnect and `takeover` to resume where you left off.
