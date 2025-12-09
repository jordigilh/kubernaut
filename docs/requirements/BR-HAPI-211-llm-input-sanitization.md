# BR-HAPI-211: LLM Input Sanitization

**Business Requirement ID**: BR-HAPI-211
**Service**: HolmesGPT-API
**Category**: Security
**Priority**: P0 (CRITICAL)
**Status**: 📋 APPROVED (V1.0 Scope)
**Created**: December 9, 2025

---

## 📋 Summary

The HolmesGPT-API service MUST sanitize ALL data sent to external LLM providers to prevent credential leakage. This includes initial prompts, tool call results, and error messages.

---

## 🎯 Business Justification

### Problem

HAPI sends data to external LLM providers (OpenAI, Anthropic, Vertex AI) for AI-powered investigation. This data may contain:

- Database passwords in application logs (`kubectl logs`)
- API keys in environment variables (`kubectl describe pod`)
- Connection strings in error messages
- Credentials in workflow parameters

**Risk**: These credentials would be transmitted to external services, violating security policies and potentially exposing sensitive data.

### Business Impact

| Impact Area | Description | Severity |
|-------------|-------------|----------|
| **Security** | Credentials leaked to external providers | 🔴 HIGH |
| **Compliance** | Violation of data protection requirements | 🔴 HIGH |
| **Audit** | Cannot demonstrate credential protection | 🟡 MEDIUM |
| **Trust** | Customer data exposure risk | 🔴 HIGH |

### Value Proposition

- **Zero credential leakage** to external LLM providers
- **Compliance ready** for security audits
- **Consistent protection** with DD-005 patterns (Go services)
- **Preserved functionality** - logs toolset remains enabled

---

## 📐 Specification

### Functional Requirements

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| **FR-1** | Sanitize ALL prompts before LLM submission | P0 | 📋 Planned |
| **FR-2** | Sanitize ALL tool call results (Tool.invoke() wrapper) | P0 | 📋 Planned |
| **FR-3** | Sanitize error messages in tool results | P0 | 📋 Planned |
| **FR-4** | Use DD-005 compliant patterns | P1 | 📋 Planned |
| **FR-5** | Log sanitization events for audit | P2 | 📋 Planned |
| **FR-6** | Graceful degradation on sanitization errors | P1 | 📋 Planned |

### Sanitization Patterns (DD-005 Compliant)

> **⚠️ CRITICAL: Pattern Ordering**
> Patterns MUST be applied in order: **broad container patterns first**, then specific patterns.
> This prevents sub-patterns from corrupting larger structures.

| Priority | Category | Pattern | Replacement |
|----------|----------|---------|-------------|
| **P0** | Database URLs | `(postgres\|mysql\|mongodb\|redis)://[^:]+:[^@]+@` | `\1://user:[REDACTED]@` |
| **P0** | Passwords (JSON) | `"(password\|passwd\|pwd)"\s*:\s*"[^"]*"` | `"\1":"[REDACTED]"` |
| **P0** | Passwords (plain) | `(password\|passwd\|pwd)\s*[=:]\s*\S+` | `\1=[REDACTED]` |
| **P0** | Passwords (URL) | `://([^:/@]+):([^@]+)@` | `://\1:[REDACTED]@` |
| **P0** | API Keys (OpenAI) | `sk-[A-Za-z0-9_\-]{20,}` | `[REDACTED]` |
| **P0** | API Keys (generic) | `(api[_-]?key\|apikey)\s*[=:]\s*\S+` | `\1=[REDACTED]` |
| **P0** | Bearer Tokens | `Bearer\s+[A-Za-z0-9\-_\.]+` | `Bearer [REDACTED]` |
| **P0** | JWT Tokens | `eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+` | `[REDACTED_JWT]` |
| **P0** | GitHub Tokens | `ghp_[A-Za-z0-9]{36,}` | `[REDACTED_GITHUB_TOKEN]` |
| **P1** | AWS Access Keys | `AKIA[A-Z0-9]{16}` | `[REDACTED_AWS_ACCESS_KEY]` |
| **P1** | AWS Secret Keys | `(aws_secret[_-]?access[_-]?key)\s*[=:]\s*\S+` | `\1=[REDACTED]` |
| **P1** | Private Keys | `-----BEGIN.*PRIVATE KEY-----[\s\S]*?-----END.*PRIVATE KEY-----` | `[REDACTED_PRIVATE_KEY]` |
| **P1** | K8s Secret Data | Base64 values in k8s secret format | `[REDACTED_K8S_SECRET_DATA]` |
| **P1** | Base64 Secrets | `(secret\|key\|token)\s*[=:]\s*[A-Za-z0-9+/]{32,}={0,2}` | `\1=[REDACTED_BASE64]` |
| **P1** | Authorization | `(authorization)\s*:\s*\S+` | `\1: [REDACTED]` |
| **P2** | Secrets (JSON) | `"(secret\|client_secret)"\s*:\s*"[^"]*"` | `"\1":"[REDACTED]"` |
| **P2** | Secrets (plain) | `(secret\|client_secret)\s*[=:]\s*\S+` | `\1=[REDACTED]` |

**Total**: 17 pattern categories (aligned with Go shared library)

### Non-Functional Requirements

| ID | Requirement | Target |
|----|-------------|--------|
| **NFR-1** | Sanitization latency | <10ms per call |
| **NFR-2** | False positive rate | <5% |
| **NFR-3** | Pattern coverage | 100% DD-005 patterns |

---

## 🏗️ Architecture

### Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     SANITIZATION LAYER                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   INPUT DATA              SANITIZER                 OUTPUT       │
│                                                                  │
│   error_message ────┐                                            │
│   description   ────┼───▶ sanitize_for_llm() ───▶ Clean Prompt   │
│   parameters    ────┘                                            │
│                                                                  │
│   kubectl logs  ────┐                                            │
│   kubectl get   ────┼───▶ Tool.invoke() wrap ───▶ Clean Result   │
│   kubectl desc  ────┘                                            │
│                                                                  │
│                           │                                      │
│                           ▼                                      │
│                    External LLM                                  │
│                    (No credentials)                              │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Implementation Components

| Component | Location | Responsibility |
|-----------|----------|----------------|
| `LLMSanitizer` | `src/sanitization/llm_sanitizer.py` | Regex-based credential redaction |
| Tool Wrapper | `src/extensions/llm_config.py` | Wrap `Tool.invoke()` |
| Prompt Sanitization | `src/extensions/incident.py` | Sanitize constructed prompts |
| Prompt Sanitization | `src/extensions/recovery.py` | Sanitize recovery prompts |

---

## 🧪 Test Requirements

### Unit Tests (30+ required)

#### Pattern Tests (17 tests)
| Test | Description |
|------|-------------|
| `test_password_json_sanitized` | `{"password":"secret"}` → `{"password":"[REDACTED]"}` |
| `test_password_plain_sanitized` | `password=secret` → `password=[REDACTED]` |
| `test_password_url_sanitized` | `://user:pass@host` → `://user:[REDACTED]@host` |
| `test_database_url_postgres_sanitized` | PostgreSQL URLs |
| `test_database_url_redis_sanitized` | Redis URLs (new) |
| `test_api_key_openai_sanitized` | `sk-proj-xxx` → `[REDACTED]` |
| `test_api_key_generic_sanitized` | `api_key=xxx` patterns |
| `test_bearer_token_sanitized` | `Bearer xxx` tokens |
| `test_jwt_sanitized` | Full JWT tokens |
| `test_github_token_sanitized` | `ghp_xxx` patterns |
| `test_aws_access_key_sanitized` | `AKIA...` patterns |
| `test_aws_secret_key_sanitized` | AWS secret keys |
| `test_private_key_sanitized` | PEM private keys |
| `test_k8s_secret_data_sanitized` | Base64 K8s secrets |
| `test_base64_secret_sanitized` | Generic base64 secrets |
| `test_authorization_header_sanitized` | Auth headers |
| `test_client_secret_sanitized` | OAuth secrets |

#### Data Type Tests (5 tests)
| Test | Description |
|------|-------------|
| `test_sanitize_string` | Direct string sanitization |
| `test_sanitize_dict` | Dict with nested credentials |
| `test_sanitize_list` | List of mixed content |
| `test_sanitize_none_returns_none` | Null handling |
| `test_sanitize_nested_dict` | Deep nesting (3+ levels) |

#### Edge Case Tests (5 tests)
| Test | Description |
|------|-------------|
| `test_sanitize_empty_string` | Empty input handling |
| `test_sanitize_long_content` | >1MB content performance (<100ms) |
| `test_sanitize_multiple_credentials` | Multiple patterns in one string |
| `test_pattern_ordering` | Broad patterns applied before specific |
| `test_fallback_on_regex_error` | Graceful degradation |

#### Tool Wrapper Tests (3 tests)
| Test | Description |
|------|-------------|
| `test_tool_invoke_sanitizes_data` | Data field sanitization |
| `test_tool_invoke_sanitizes_error` | Error field sanitization |
| `test_tool_invoke_handles_none` | None data handling |

### Integration Tests (3+ required)

| Test | Description |
|------|-------------|
| `test_prompt_sanitization_e2e` | Full prompt construction with sanitization |
| `test_tool_result_sanitization_e2e` | Tool execution with sanitization |
| `test_incident_endpoint_sanitizes_prompts` | Full incident flow |

---

## 📊 Acceptance Criteria

### Must Have (P0)

- [ ] All prompts sanitized before LLM submission (`incident.py`, `recovery.py`)
- [ ] All tool results sanitized (kubernetes/logs, kubernetes/core)
- [ ] All 17 DD-005 pattern categories implemented
- [ ] Data type handling: str, dict, list, None
- [ ] Pattern ordering: broad patterns first
- [ ] Zero credential leakage in LLM communication
- [ ] 30+ unit tests passing

### Should Have (P1)

- [ ] Graceful degradation with fallback mechanism
- [ ] Sanitization event logging for audit
- [ ] Performance <10ms per sanitization call
- [ ] Metrics for sanitization operations

### Could Have (P2)

- [ ] Configurable pattern list (extend defaults)
- [ ] Pattern tuning based on false positives
- [ ] Streaming sanitization for very large content

---

## 🔧 Data Type Handling Specification

**CRITICAL**: `StructuredToolResult.data` is typed as `Any` and MUST handle all types:

```python
def sanitize_for_llm(content: Any) -> Any:
    """
    Sanitize any content type before sending to LLM.

    Handles:
    - str: Direct regex sanitization
    - dict: JSON serialize → sanitize → deserialize
    - list: Recursive sanitization of each item
    - None: Return as-is (no sanitization needed)
    - Other: Convert to string, sanitize, return string
    """
    if content is None:
        return None
    if isinstance(content, str):
        return _apply_patterns(content)
    if isinstance(content, dict):
        return json.loads(_apply_patterns(json.dumps(content, default=str)))
    if isinstance(content, list):
        return [sanitize_for_llm(item) for item in content]
    return _apply_patterns(str(content))
```

---

## 🛡️ Graceful Degradation Specification

**FR-6**: If regex processing fails, fall back to simple string matching:

```python
def sanitize_with_fallback(content: str) -> tuple[str, Optional[Exception]]:
    """Sanitize with automatic fallback on regex errors."""
    try:
        return _apply_patterns(content), None
    except Exception as e:
        logger.warning(f"BR-HAPI-211: Regex failed, using fallback: {e}")
        return _safe_fallback(content), e

def _safe_fallback(content: str) -> str:
    """Simple string matching when regex fails."""
    output = content
    keywords = ["password:", "secret:", "token:", "api_key:", "bearer:"]
    for kw in keywords:
        # Simple find-and-redact without regex
        idx = output.lower().find(kw)
        while idx != -1:
            value_start = idx + len(kw)
            value_end = _find_value_end(output, value_start)
            output = output[:value_start] + "[REDACTED]" + output[value_end:]
            idx = output.lower().find(kw, value_start + 10)
    return output
```

---

## 🔗 Related Documents

| Document | Relationship |
|----------|--------------|
| [DD-HAPI-005](../../architecture/decisions/DD-HAPI-005-llm-input-sanitization.md) | Design Decision |
| [DD-005](../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) | Pattern Source |
| [pkg/shared/sanitization/](../../../pkg/shared/sanitization/) | Go Reference Implementation |
| [security-configuration.md](../../services/stateless/holmesgpt-api/security-configuration.md) | HAPI Security Overview |

---

## 📅 Timeline

| Phase | Duration | Tasks | Status |
|-------|----------|-------|--------|
| Design & Spec | 1 hr | DD-HAPI-005, BR-HAPI-211 | ✅ Complete |
| Triage & Update | 0.5 hr | Gaps, inconsistencies, spec updates | ✅ Complete |
| Sanitizer Core | 1.5 hr | `llm_sanitizer.py` with all patterns, types, fallback | 📋 Planned |
| Tool Wrapper | 1.5 hr | Extend `patched_create_tool_executor()` | 📋 Planned |
| Prompt Sanitization | 0.5 hr | `incident.py`, `recovery.py` | 📋 Planned |
| Unit Tests | 2 hr | 30+ tests (patterns, types, edge cases) | 📋 Planned |
| Integration Tests | 1 hr | E2E prompt + tool sanitization | 📋 Planned |
| **Total** | **8 hours** | | |

---

## ⚠️ Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Over-redaction** | LLM loses context | Pattern tuning, <5% false positive target |
| **Under-redaction** | Credential leakage | Comprehensive patterns from Go library |
| **Performance** | Latency spikes | Lazy eval for >1MB, <10ms target |
| **SDK changes** | Signature change | Pin version, version check in tests |

---

**Document Version**: 1.1
**Created**: December 9, 2025
**Updated**: December 9, 2025 (Triage findings incorporated)
**Author**: HAPI Team

