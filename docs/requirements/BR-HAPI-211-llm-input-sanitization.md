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

| Pattern Category | Regex Pattern | Replacement |
|-----------------|---------------|-------------|
| **Passwords (JSON)** | `"(password\|passwd\|pwd)"\s*:\s*"[^"]*"` | `"\1":"[REDACTED]"` |
| **Passwords (plain)** | `(password\|passwd\|pwd)\s*[=:]\s*\S+` | `\1=[REDACTED]` |
| **API Keys** | `(api[_-]?key\|apikey)\s*[=:]\s*\S+` | `\1=[REDACTED]` |
| **Tokens** | `(token\|auth\|bearer)\s*[=:]\s*\S+` | `\1=[REDACTED]` |
| **JWT Tokens** | `eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+` | `[REDACTED_JWT]` |
| **Database URLs** | `(postgres\|mysql\|mongodb)://[^:]+:[^@]+@` | `\1://[USER]:[REDACTED]@` |
| **AWS Access Keys** | `AKIA[A-Z0-9]{16}` | `[REDACTED_AWS_ACCESS_KEY]` |
| **GitHub Tokens** | `ghp_[A-Za-z0-9]{36}` | `[REDACTED_GITHUB_TOKEN]` |
| **Private Keys** | `-----BEGIN.*PRIVATE KEY-----[\s\S]*?-----END.*PRIVATE KEY-----` | `[REDACTED_PRIVATE_KEY]` |
| **K8s Secret Data** | `data:\s*\n(\s+\w+:\s*[A-Za-z0-9+/]{32,}={0,2}\s*\n?)+` | `[REDACTED_K8S_SECRET_DATA]` |
| **Base64 Secrets** | `(secret\|key\|token)\s*[=:]\s*[A-Za-z0-9+/]{32,}={0,2}` | `\1=[REDACTED_BASE64]` |

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

### Unit Tests

| Test | Description | File |
|------|-------------|------|
| `test_password_json_sanitized` | JSON password patterns | `test_llm_sanitizer.py` |
| `test_password_plain_sanitized` | Plain text passwords | `test_llm_sanitizer.py` |
| `test_database_url_sanitized` | PostgreSQL/MySQL URLs | `test_llm_sanitizer.py` |
| `test_aws_key_sanitized` | AWS access keys | `test_llm_sanitizer.py` |
| `test_jwt_sanitized` | JWT tokens | `test_llm_sanitizer.py` |
| `test_github_token_sanitized` | GitHub PATs | `test_llm_sanitizer.py` |
| `test_private_key_sanitized` | PEM private keys | `test_llm_sanitizer.py` |
| `test_k8s_secret_data_sanitized` | K8s Secret base64 data | `test_llm_sanitizer.py` |
| `test_logs_with_credentials_sanitized` | Realistic log output | `test_llm_sanitizer.py` |
| `test_tool_invoke_sanitizes_data` | Tool wrapper integration | `test_llm_sanitizer.py` |

### Integration Tests

| Test | Description |
|------|-------------|
| `test_prompt_sanitization_e2e` | Full prompt construction with sanitization |
| `test_tool_result_sanitization_e2e` | Tool execution with sanitization |

---

## 📊 Acceptance Criteria

### Must Have (P0)

- [ ] All prompts sanitized before LLM submission
- [ ] All tool results sanitized (kubernetes/logs, kubernetes/core)
- [ ] All DD-005 patterns implemented
- [ ] Zero credential leakage in LLM communication
- [ ] 15+ unit tests passing

### Should Have (P1)

- [ ] Sanitization event logging for audit
- [ ] Graceful degradation on errors
- [ ] Metrics for sanitization operations

### Could Have (P2)

- [ ] Configurable pattern list
- [ ] Pattern tuning based on false positives

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

| Phase | Duration | Status |
|-------|----------|--------|
| Design & Spec | 1 hour | ✅ Complete |
| Implementation | 4 hours | 📋 Planned |
| Testing | 1.5 hours | 📋 Planned |
| Documentation | 0.5 hour | 📋 Planned |
| **Total** | **7 hours** | **V1.0** |

---

**Document Version**: 1.0
**Created**: December 9, 2025
**Author**: HAPI Team

