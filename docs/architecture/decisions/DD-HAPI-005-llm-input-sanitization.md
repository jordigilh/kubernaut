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
- ⚠️ Development effort (~7 hours including comprehensive testing)
- ⚠️ Potential for over-redaction (mitigated by pattern tuning)

**Confidence**: 95% - APPROVED

---

## Implementation

### Architecture

#### Component 1: LLM Sanitizer Module

**Location**: `holmesgpt-api/src/sanitization/llm_sanitizer.py`

**Responsibility**: Regex-based credential detection and redaction

**Patterns** (ported from `pkg/shared/sanitization/sanitizer.go`):

> **⚠️ CRITICAL: Pattern Ordering**
> Patterns MUST be applied in order: **broad container patterns first**, then specific patterns.
> This prevents sub-patterns from corrupting larger structures (e.g., password inside a URL).

| Priority | Pattern Category | Examples | Replacement |
|----------|-----------------|----------|-------------|
| **P0** | Database URLs | `postgres://user:pass@host`, `redis://...` | `postgres://user:[REDACTED]@host` |
| **P0** | Passwords (JSON) | `"password":"secret"`, `"pwd":"abc"` | `"password":"[REDACTED]"` |
| **P0** | Passwords (plain) | `password=secret`, `pwd: abc` | `password=[REDACTED]` |
| **P0** | Passwords (URL) | `://user:pass@host` | `://user:[REDACTED]@host` |
| **P0** | API Keys (OpenAI) | `sk-proj-abc123...` | `[REDACTED]` |
| **P0** | API Keys (generic) | `api_key=xxx`, `"apikey":"yyy"` | `api_key=[REDACTED]` |
| **P0** | Bearer Tokens | `Bearer eyJ...` | `Bearer [REDACTED]` |
| **P0** | JWT Tokens | `eyJhbG...eyJzdW...signature` | `[REDACTED_JWT]` |
| **P0** | GitHub Tokens | `ghp_xxxxxxxxxxxx` | `[REDACTED_GITHUB_TOKEN]` |
| **P1** | AWS Access Keys | `AKIAIOSFODNN7EXAMPLE` | `[REDACTED_AWS_ACCESS_KEY]` |
| **P1** | AWS Secret Keys | `aws_secret_access_key=xxx` | `[REDACTED]` |
| **P1** | Private Keys | `-----BEGIN PRIVATE KEY-----` | `[REDACTED_PRIVATE_KEY]` |
| **P1** | K8s Secrets | `data:\n  key: base64...` | `[REDACTED_K8S_SECRET_DATA]` |
| **P1** | Base64 Secrets | `secret: SGVsbG8gV29ybGQ=` | `secret: [REDACTED_BASE64]` |
| **P1** | Authorization | `authorization: Basic xxx` | `authorization: [REDACTED]` |
| **P2** | Secrets (JSON) | `"secret":"value"` | `"secret":"[REDACTED]"` |
| **P2** | Secrets (plain) | `client_secret=xxx` | `client_secret=[REDACTED]` |

**Total**: 17 pattern categories (aligned with Go shared library)

#### Component 2: Tool Invoke Wrapper

**Location**: `holmesgpt-api/src/extensions/llm_config.py`

**Responsibility**: Intercept `Tool.invoke()` to sanitize `StructuredToolResult.data`

**Hook Point**:
```python
# HolmesGPT SDK's Tool class
Tool.invoke(params) -> StructuredToolResult
    ├── status: SUCCESS/ERROR/...
    ├── data: Any           # ← SANITIZE THIS (str, dict, list, or None)
    ├── error: Optional[str] # ← AND THIS
    └── invocation: str
```

**Data Type Handling** (CRITICAL):

`StructuredToolResult.data` is typed as `Any` and can be:
- `str` - Direct regex sanitization
- `dict` - JSON serialize → sanitize → deserialize
- `list` - Recursive sanitization of each item
- `None` - Skip (return as-is)

```python
def sanitize_for_llm(content: Any) -> Any:
    """Sanitize any content type before sending to LLM."""
    if content is None:
        return None
    if isinstance(content, str):
        return _apply_patterns(content)
    if isinstance(content, dict):
        # Serialize to JSON, sanitize, deserialize
        return json.loads(_apply_patterns(json.dumps(content, default=str)))
    if isinstance(content, list):
        return [sanitize_for_llm(item) for item in content]
    # Fallback: convert to string and sanitize
    return _apply_patterns(str(content))
```

**Wrapping Strategy** (extends existing monkey-patch pattern):
```python
def wrap_tool_results_with_sanitization(tool_executor):
    """BR-HAPI-211: Wrap Tool.invoke() for credential sanitization."""
    for toolset in tool_executor.toolsets:
        for tool in toolset.tools:
            original_invoke = tool.invoke

            def sanitized_invoke(params, tool_number=None, user_approved=False,
                                 _orig=original_invoke, _tool_name=tool.name):
                result = _orig(params, tool_number, user_approved)
                # Sanitize data (handles str, dict, list, None)
                result.data = sanitize_for_llm(result.data)
                # Sanitize error message if present
                if result.error:
                    result.error = sanitize_for_llm(result.error)
                logger.debug(f"BR-HAPI-211: Sanitized tool result for {_tool_name}")
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
  - **Mitigation**: Pattern tuning, logging of redaction events, <5% target
- ⚠️ **Performance**: Regex processing adds latency
  - **Mitigation**: Minimal (~1-5ms per sanitization call), lazy eval for >1MB

### Graceful Degradation (FR-5)

If regex processing fails (e.g., malformed input, regex catastrophic backtracking):

```python
def sanitize_with_fallback(content: str) -> tuple[str, Optional[Exception]]:
    """
    Sanitize with automatic fallback on regex errors.

    Returns: (sanitized_content, error_if_fallback_used)
    """
    try:
        return _apply_patterns(content), None
    except Exception as e:
        logger.warning(f"BR-HAPI-211: Regex failed, using fallback: {e}")
        # Fallback: simple string replacement (no regex)
        return _safe_fallback(content), e

def _safe_fallback(content: str) -> str:
    """Simple string matching fallback when regex fails."""
    output = content
    for keyword in ["password:", "secret:", "token:", "api_key:", "bearer:"]:
        # Find and redact values after keywords
        idx = output.lower().find(keyword)
        while idx != -1:
            value_start = idx + len(keyword)
            value_end = _find_value_end(output, value_start)
            output = output[:value_start] + "[REDACTED]" + output[value_end:]
            idx = output.lower().find(keyword, value_start + len("[REDACTED]"))
    return output
```

### Neutral

- 🔄 Patterns must be maintained in sync with Go shared library
- 🔄 New credential patterns require updates

---

## Validation

### Test Coverage Requirements

| Test Type | Count | Coverage |
|-----------|-------|----------|
| Unit Tests (patterns) | 17+ | All 17 pattern categories |
| Unit Tests (data types) | 5+ | str, dict, list, None, mixed |
| Unit Tests (edge cases) | 5+ | Empty, long content, nested, fallback |
| Unit Tests (tool wrapper) | 3+ | Invoke wrapping, error handling |
| Integration Tests | 3+ | End-to-end prompt + tool sanitization |
| **Total Unit Tests** | **30+** | |

**Required Test Scenarios**:
```python
# Pattern tests
test_password_json_sanitized()
test_password_plain_sanitized()
test_password_url_sanitized()
test_database_url_postgres_sanitized()
test_database_url_redis_sanitized()
test_api_key_openai_sanitized()
test_api_key_generic_sanitized()
test_bearer_token_sanitized()
test_jwt_token_sanitized()
test_github_token_sanitized()
test_aws_access_key_sanitized()
test_aws_secret_key_sanitized()
test_private_key_sanitized()
test_k8s_secret_data_sanitized()
test_base64_secret_sanitized()
test_authorization_header_sanitized()
test_client_secret_sanitized()

# Data type tests
test_sanitize_string()
test_sanitize_dict()
test_sanitize_list()
test_sanitize_none_returns_none()
test_sanitize_nested_dict()

# Edge case tests
test_sanitize_empty_string()
test_sanitize_long_content_performance()
test_sanitize_multiple_credentials()
test_pattern_ordering_prevents_corruption()
test_fallback_on_regex_error()

# Tool wrapper tests
test_tool_invoke_sanitizes_data()
test_tool_invoke_sanitizes_error()
test_tool_invoke_handles_none_data()
```

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

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| **Over-redaction** | LLM loses investigation context | MEDIUM | Pattern tuning, log redaction events, <5% target |
| **Under-redaction** | Credential leakage | LOW | Comprehensive pattern list from Go library |
| **Performance on large logs** | Latency spikes | LOW | Lazy evaluation for >1MB content |
| **SDK version changes** | Tool.invoke() signature change | LOW | Pin SDK version, add version check in tests |
| **False negatives (new formats)** | New credential formats leak | MEDIUM | Extensible pattern list, quarterly review |

---

## Implementation Timeline

| Phase | Duration | Tasks |
|-------|----------|-------|
| **1. Sanitizer Core** | 1.5 hr | Create `llm_sanitizer.py` with all patterns, type handling, fallback |
| **2. Tool Wrapper** | 1.5 hr | Extend `patched_create_tool_executor()` with sanitization |
| **3. Prompt Sanitization** | 0.5 hr | Add to `incident.py`, `recovery.py` prompt construction |
| **4. Unit Tests** | 2 hr | 30+ tests covering all patterns + edge cases |
| **5. Integration Tests** | 1 hr | E2E tool result + prompt sanitization |
| **6. Documentation** | 0.5 hr | Update specs, security docs |
| **Total** | **7 hours** | |

---

**Document Version**: 1.1
**Created**: December 9, 2025
**Updated**: December 9, 2025 (Triage findings incorporated)
**Author**: HAPI Team

