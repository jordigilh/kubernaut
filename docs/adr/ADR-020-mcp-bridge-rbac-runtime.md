# ADR-020: MCP Bridge Runtime RBAC Enforcement

**Status:** Accepted
**Date:** 2026-05-09
**Context:** Issues #19, #20 — MCP Tool Bridge

## Decision

RBAC is enforced solely at `tools/call` execution time. The `tools/list` response
always returns all 20 registered tools regardless of the caller's identity.

## Rationale

1. **TOCTOU elimination**: Filtering `tools/list` creates a time-of-check to
   time-of-use gap — permissions can change between listing and calling. Runtime
   enforcement at call time is the single source of truth.

2. **UX clarity**: Showing all available tools lets users understand the full
   system capability. Denied calls return a clear RBAC error with structured
   audit events, which is better than silently hiding tools.

3. **Simplicity**: A single enforcement point (the `wrapTool` middleware) is
   easier to audit, test, and reason about than dual filtering (list + call).

## Consequences

- Agents that call tools without permission receive a structured RBAC denial
  error and an `EventMCPToolDenied` audit event.
- The `af_mcp_rbac_denied_total` Prometheus counter tracks denial rates by tool.
- No `FilterToolsMiddleware` is applied; the function has been removed entirely.
  `tools/list` always returns all 20 tools regardless of caller identity.

## Alternatives Considered

- **Filter `tools/list` by role**: Rejected due to TOCTOU race and UX concerns.
- **Dual enforcement (filter + runtime check)**: Adds complexity with no
  security benefit since runtime check is mandatory regardless.
