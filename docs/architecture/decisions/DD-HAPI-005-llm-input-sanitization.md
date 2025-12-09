# DD-HAPI-005: LLM Input Sanitization

**Status**: ✅ **APPROVED**
**Date**: December 9, 2025
**Decision Makers**: HAPI Team, Security Team
**Priority**: P0 (CRITICAL for V1.0)

---

## Context

### Problem Statement

HolmesGPT-API sends data to external LLM providers (OpenAI, Anthropic, etc.) for AI-powered Kubernetes investigation. This data flow includes:

1. **Initial prompts** containing error messages, descriptions, and signal context
2. **Tool call results** from Kubernetes toolsets (logs, pod descriptions, events)
3. **Recovery context** including workflow parameters and failure details

**Security Risk**: This data may contain credentials that would be leaked to external LLM providers:

| Data Source | Risk Level | Example Credential Exposure |
|-------------|------------|----------------------------|
| `kubectl logs` output | 🔴 HIGH | Database passwords in application logs |
| `error_message` field | 🔴 HIGH | Connection strings in error stack traces |
| `kubectl get pods -o yaml` | 🟡 MEDIUM | Environment variables with secrets |
| `kubectl get events` | 🟡 MEDIUM | Secrets in event messages |
| Workflow parameters | 🔴 HIGH | Credentials passed to remediation workflows |
| `naturalLanguageSummary` | 🟡 MEDIUM | WE-generated context may include secrets |

### Business Impact

- **Compliance Risk**: Credentials sent to external services violate security policies
- **Data Leakage**: LLM providers may log/train on sensitive data
- **Audit Failure**: Cannot demonstrate credential protection in security audits

### Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| **FR-1** | ALL data sent to LLM must be sanitized for credentials | P0 |
| **FR-2** | Sanitization must cover prompts AND tool call results | P0 |
| **FR-3** | Use DD-005 patterns for consistency with Go services | P1 |
| **FR-4** | Sanitization must not break LLM investigation capability | P0 |
| **FR-5** | Graceful degradation on sanitization errors | P1 |

---

## Decision

### APPROVED: Comprehensive LLM Input Sanitization Layer

Implement a sanitization layer that intercepts ALL data flowing to the LLM:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         HAPI Service                                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   Request Data                                                           │
│   (error_message,         ┌──────────────────┐                          │
│    description,    ────▶  │ LLM Sanitizer    │ ────▶  Sanitized Prompt   │
│    parameters)            │ (DD-HAPI-005)    │                          │
│                           └──────────────────┘                          │
│                                                                          │
│   Tool Execution                                                         │
│   ┌─────────────┐         ┌──────────────────┐                          │
│   │ kubernetes/ │         │ Wrapped          │                          │
│   │ logs        │ ────▶   │ Tool.invoke()    │ ────▶  Sanitized Result   │
│   │ core        │         │ (DD-HAPI-005)    │                          │
│   └─────────────┘         └──────────────────┘                          │
│                                                                          │
│                           ┌──────────────────┐                          │
│   Sanitized Data ────────▶│ External LLM     │                          │
│   (no credentials)        │ (OpenAI, etc.)   │                          │
│                           └──────────────────┘                          │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Alternatives Considered

### Alternative 1: Disable High-Risk Toolsets

**Approach**: Disable `kubernetes/logs` toolset to prevent log-based credential leakage.

**Pros**:
- ✅ Simple implementation (config change only)
- ✅ Zero development effort

**Cons**:
- ❌ **CRITICAL**: Logs are essential for root cause analysis
- ❌ Severely degrades investigation quality
- ❌ Does not address prompt-level leakage

**Confidence**: 10% - REJECTED (breaks core functionality)

---

### Alternative 2: RBAC Restriction Only

**Approach**: Rely on Kubernetes RBAC to prevent secret access.

**Pros**:
- ✅ Already implemented (HAPI SA cannot read arbitrary secrets)
- ✅ No code changes

**Cons**:
- ❌ Does not protect against secrets in logs
- ❌ Does not protect against secrets in error messages
- ❌ ConfigMaps may contain semi-sensitive data

**Confidence**: 30% - INSUFFICIENT (partial protection only)

---

### Alternative 3: Comprehensive Sanitization Layer (APPROVED)

**Approach**: Wrap ALL data paths to LLM with DD-005 compliant sanitization.

**Pros**:
- ✅ Complete coverage (prompts + tool results)
- ✅ Consistent with Go services (DD-005 patterns)
- ✅ Preserves full investigation capability
- ✅ Auditable protection

**Cons**:
- ⚠️ Development effort (~5.5 hours)
- ⚠️ Potential for over-redaction (mitigated by pattern tuning)

**Confidence**: 95% - APPROVED

---

## Implementation

### Architecture

#### Component 1: LLM Sanitizer Module

**Location**: `holmesgpt-api/src/sanitization/llm_sanitizer.py`

**Responsibility**: Regex-based credential detection and redaction

**Patterns** (ported from `pkg/shared/sanitization/sanitizer.go`):

| Pattern Category | Examples | Replacement |
|-----------------|----------|-------------|
| Passwords | `password=secret`, `"pwd":"abc"` | `password=[REDACTED]` |
| API Keys | `api_key=sk-xxx`, `OPENAI_API_KEY` | `api_key=[REDACTED]` |
| Tokens | `Bearer eyJ...`, `token=ghp_xxx` | `[REDACTED_JWT]`, `[REDACTED_GITHUB_TOKEN]` |
| Database URLs | `postgres://user:pass@host` | `postgres://[USER]:[REDACTED]@host` |
| AWS Credentials | `AKIAIOSFODNN7EXAMPLE` | `[REDACTED_AWS_ACCESS_KEY]` |
| Private Keys | `-----BEGIN PRIVATE KEY-----` | `[REDACTED_PRIVATE_KEY]` |
| K8s Secrets | `data:\n  key: base64...` | `[REDACTED_K8S_SECRET_DATA]` |

#### Component 2: Tool Invoke Wrapper

**Location**: `holmesgpt-api/src/extensions/llm_config.py`

**Responsibility**: Intercept `Tool.invoke()` to sanitize `StructuredToolResult.data`

**Hook Point**:
```python
# HolmesGPT SDK's Tool class
Tool.invoke(params) -> StructuredToolResult
    ├── status: SUCCESS/ERROR/...
    ├── data: Any           # ← SANITIZE THIS
    ├── error: Optional[str] # ← AND THIS
    └── invocation: str
```

**Wrapping Strategy** (extends existing monkey-patch pattern):
```python
def wrap_tool_results_with_sanitization(tool_executor):
    """BR-HAPI-211: Wrap Tool.invoke() for credential sanitization."""
    for toolset in tool_executor.toolsets:
        for tool in toolset.tools:
            original_invoke = tool.invoke
            
            def sanitized_invoke(params, ...):
                result = original_invoke(params, ...)
                result.data = sanitize_for_llm(result.data)
                result.error = sanitize_for_llm(result.error) if result.error else None
                return result
            
            tool.invoke = sanitized_invoke
```

#### Component 3: Prompt Sanitization

**Location**: `holmesgpt-api/src/extensions/incident.py`, `recovery.py`

**Responsibility**: Sanitize constructed prompts before LLM submission

**Integration Point**:
```python
def _create_incident_investigation_prompt(request_data):
    # ... construct prompt ...
    return sanitize_for_llm(prompt)  # ← ADD THIS
```

### Data Flow (After Implementation)

```
1. Request arrives with error_message, description, etc.
   ↓
2. Prompt constructed from request data
   ↓
3. ✅ SANITIZE: sanitize_for_llm(prompt)
   ↓
4. HolmesGPT SDK processes prompt
   ↓
5. LLM requests tool call (e.g., kubectl logs)
   ↓
6. Tool.invoke() executes kubectl command
   ↓
7. ✅ SANITIZE: Wrapped invoke() sanitizes result.data
   ↓
8. Sanitized result returned to LLM
   ↓
9. LLM generates analysis (no credentials in context)
```

---

## Consequences

### Positive

- ✅ **Security**: Credentials cannot leak to external LLM providers
- ✅ **Compliance**: Demonstrates security controls for audits
- ✅ **Consistency**: Uses DD-005 patterns (same as Go services)
- ✅ **Preserved Capability**: Logs toolset remains enabled

### Negative

- ⚠️ **Over-Redaction Risk**: Legitimate data may be redacted
  - **Mitigation**: Pattern tuning, logging of redaction events
- ⚠️ **Performance**: Regex processing adds latency
  - **Mitigation**: Minimal (~1-5ms per sanitization call)

### Neutral

- 🔄 Patterns must be maintained in sync with Go shared library
- 🔄 New credential patterns require updates

---

## Validation

### Test Coverage Requirements

| Test Type | Count | Coverage |
|-----------|-------|----------|
| Unit Tests (sanitizer patterns) | 15+ | All DD-005 patterns |
| Unit Tests (tool wrapper) | 5+ | Invoke wrapping |
| Integration Tests | 3+ | End-to-end sanitization |

### Security Verification

```bash
# Verify no credentials in LLM audit events
grep -r "password\|secret\|token\|api_key" audit_events.json
# Should return: Only "[REDACTED]" placeholders
```

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [DD-005-OBSERVABILITY-STANDARDS.md](./DD-005-OBSERVABILITY-STANDARDS.md) | DD-005 patterns source |
| [BR-HAPI-211](../../requirements/BR-HAPI-211-llm-input-sanitization.md) | Business requirement |
| [pkg/shared/sanitization/](../../../pkg/shared/sanitization/) | Go reference implementation |
| [security-configuration.md](../../services/stateless/holmesgpt-api/security-configuration.md) | HAPI security overview |

---

## Review & Evolution

### When to Revisit

- If new credential patterns are identified
- If LLM providers offer built-in PII/credential filtering
- If performance impact becomes significant

### Success Metrics

| Metric | Target |
|--------|--------|
| Credential leakage incidents | 0 |
| False positive redaction rate | <5% |
| Sanitization latency | <10ms |

---

**Document Version**: 1.0
**Created**: December 9, 2025
**Author**: HAPI Team

