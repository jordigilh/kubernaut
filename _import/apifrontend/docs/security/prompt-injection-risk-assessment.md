# Prompt Injection Risk Assessment

**Service:** kubernaut-apifrontend
**Date:** 2026-05-07
**NIST Control:** SI-10 (Information Input Validation)
**Status:** Accepted

## Threat Model

### Attack Surface

The API Frontend receives untrusted user input via:

1. **A2A message/send** — Free-text user queries passed to the ADK agent
2. **MCP tool call parameters** — Structured JSON arguments for tool invocations
3. **Session metadata** — User-supplied context injected into session state

### Threat Vectors

| Vector | Severity | Likelihood | Description |
|--------|----------|------------|-------------|
| Direct prompt injection via A2A message | High | Medium | Attacker crafts query to override system prompt |
| Indirect injection via CRD data | Medium | Low | Malicious content in K8s resources processed by tools |
| Tool parameter manipulation | Medium | Medium | Crafted params to bypass validation or access unauthorized resources |
| Session context poisoning | Low | Low | Injecting adversarial state into session to influence future turns |

## Defenses in Place

### 1. Authentication & Authorization (OIDC + RBAC)

All A2A/MCP endpoints require JWT authentication. RBAC filtering ensures users only see tools their persona is authorized to invoke. An attacker cannot escalate privileges through prompt injection because tool access is enforced at the middleware layer, not at the LLM level.

### 2. RBAC-Scoped Tool Access

The `rbac_roles.yaml` defines per-persona tool whitelists. Even if an LLM is tricked into calling a tool, the execution layer validates the caller's groups before dispatching. Unauthorized tool calls are rejected with a 403 and audited.

### 3. KA Shadow Agent Architecture

The API Frontend does not directly execute remediation actions. All actions are delegated to the Kubernaut Agent (KA) through its MCP endpoint, which applies its own authorization layer (SubjectAccessReview). This creates a defense-in-depth boundary — AF cannot bypass KA's authorization even under full prompt injection.

### 4. Rate Limiting

Per-IP and per-user rate limiters prevent automated injection attempts at scale. The `ToolCallsPerMinute` limit specifically constrains how many tool invocations a compromised session can trigger.

### 5. Audit Trail (FedRAMP AU-2)

Every tool call, session creation, and state transition is emitted as a durable audit event to DataStorage. Security teams can detect injection patterns through anomaly detection on the audit stream.

### 6. Input Validation

- CRD names are validated against DNS label regex before K8s API calls
- CEL expressions in auth claims are evaluated in a sandboxed environment
- Tool parameters are type-checked by the MCP SDK before dispatch

## Residual Risk

| Risk | Impact | Mitigation | Decision |
|------|--------|------------|----------|
| LLM generates misleading triage advice | Medium | Human review required before remediation execution | Accept |
| Model exfiltrates context via tool descriptions | Low | Tool descriptions are static, non-sensitive | Accept |
| Slow injection via multi-turn conversation | Medium | Session TTL (30d) limits exposure window | Accept |

## Recommendations

1. **Monitor** — Add alert on unusual tool call patterns per session (>20 distinct tools in 5 minutes)
2. **Defer** — System prompt hardening via canary tokens (scheduled for v1.6)
3. **Defer** — Output filtering/content safety layer (depends on model provider capabilities)

## Compliance Mapping

- **NIST SI-10**: Input validation on all external interfaces (OIDC claims, CRD names, tool params)
- **NIST AC-6**: Least privilege via RBAC persona tool scoping
- **NIST AU-2**: Complete audit trail of all tool invocations and state changes
- **NIST IR-4**: Audit events enable incident response and forensic analysis
