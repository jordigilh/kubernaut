# Test Plan: Normalize LLM Configuration Between AF and KA

> **Template Version**: 2.0 -- Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1488-v1.0
**Feature**: Extract shared `LLMConfig` type, normalize API key handling to `apiKeyFile`
**Version**: 1.0
**Created**: 2026-06-24
**Author**: AI Agent
**Status**: Draft
**Branch**: `feat/1488-normalize-llm-config`

---

## 1. Introduction

### 1.1 Purpose

Validate that extracting a shared `LLMConfig` type into `pkg/shared/types/`
preserves all existing behavior for both the API Frontend (AF) and Kubernaut
Agent (KA) while eliminating schema divergence. Verify that KA's migration
from inline `apiKey` to file-based `apiKeyFile` maintains credential
resolution, and that hot-reload continues to function with the merged type.

### 1.2 Objectives

1. **Type correctness**: Shared `LLMConfig` struct can be deserialized from
   both AF and KA ConfigMap formats
2. **Validation portability**: `Validate()` method covers all provider rules
   currently split across AF and KA
3. **API key resolution**: `ResolveAPIKey()` reads credentials from file path
4. **AF backward compat**: AF config loading, factory dispatch, and transport
   chain work identically after migration
5. **KA backward compat**: KA config loading, LLM client construction, and
   hot-reload work identically after migration
6. **apiKeyFile migration**: KA uses file-based credential resolution instead
   of inline API key

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit` |
| Integration test pass rate | 100% | `make test-integration-apifrontend`, `make test-integration-kubernautagent` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on shared types |
| Backward compatibility | 0 regressions | Existing tests pass unchanged |

---

## 2. References

### 2.1 Authority

- **BR-INTEGRATION-1488**: Normalize LLM configuration structure between AF
  and KA
- **Issue #1488**: Normalize LLM configuration structure between AF and KA

### 2.2 Cross-References

- [Testing Strategy](.cursor/rules/03-testing-strategy.mdc)
- [Wiring Verification](.cursor/rules/10-wiring-verification.mdc)
- [Test Plan TP-1254](../1254/TEST_PLAN.md) -- AF OpenAI adapter (related)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | KA hot-reload regression after type merge | LLM client not updated on ConfigMap change | Medium | IT-KA-1488-001 | IT test: change model in runtime ConfigMap, verify KA picks it up |
| R2 | YAML tag mismatch between AF and KA config | Deserialization silently drops fields | Low | UT-SH-1488-010, -011 | Round-trip parse tests for both formats |
| R3 | `apiKeyFile` backward compat breaks KA | KA fails to start without inline apiKey | Medium | UT-SH-1488-004 | Fallback: empty apiKeyFile accepted when credentials dir mounted |
| R4 | Validation rule differences between AF and KA | One service rejects valid config | Low | UT-SH-1488-005..009 | Port all provider-specific rules into shared Validate() |

---

## 4. Scope

### 4.1 Features to be Tested

- **Shared types** (`pkg/shared/types/llm.go`): `LLMConfig`, `LLMOAuth2Config`,
  `LLMCircuitBreaker`, `LLMHeaderDef`, `LLMOverride`, provider constants,
  `ResolveAPIKey()`, `Validate()`
- **AF migration**: `NewModelFromConfig` and `buildTransportChain` use shared type
- **KA migration**: `buildLLMClientFromConfig` uses shared type; hot-reload
  merges runtime fields into `types.LLMConfig`
- **apiKeyFile resolution**: File-based credential loading at startup and reload

### 4.2 Features Not to be Tested

- **Operator ConfigMap builders**: Tested in kubernaut-operator repo (IT-OP-1488-001)
- **Helm chart templates**: Dev-only, lower priority
- **Endpoint convention divergence**: AF vs KA path handling is adapter behavior

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (shared type validation, provider
  rules, credential resolution, YAML parsing)
- **Integration**: >=80% of integration-testable code (factory wiring, transport
  chain, hot-reload callback)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least UT + IT:
- **Unit tests**: Type validation, credential resolution, YAML round-trip
- **Integration tests**: Factory dispatch with shared type, hot-reload with
  merged config, transport chain with shared headers

### 5.3 Pass/Fail Criteria

**PASS**: All P0 tests pass, per-tier coverage >=80%, zero regressions.
**FAIL**: Any P0 test fails, coverage drops below 80%, existing tests regress.

---

## 6. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-INTEGRATION-1488 | Shared LLMConfig deserializes from YAML | P0 | Unit | UT-SH-1488-001 | Pending |
| BR-INTEGRATION-1488 | Provider constants match AF+KA union set | P0 | Unit | UT-SH-1488-002 | Pending |
| BR-INTEGRATION-1488 | ResolveAPIKey reads file into APIKey field | P0 | Unit | UT-SH-1488-003 | Pending |
| BR-INTEGRATION-1488 | ResolveAPIKey no-op when APIKeyFile empty | P0 | Unit | UT-SH-1488-004 | Pending |
| BR-INTEGRATION-1488 | Validate accepts vertex_ai with required fields | P0 | Unit | UT-SH-1488-005 | Pending |
| BR-INTEGRATION-1488 | Validate rejects unknown provider | P0 | Unit | UT-SH-1488-006 | Pending |
| BR-INTEGRATION-1488 | Validate rejects openai without endpoint | P0 | Unit | UT-SH-1488-007 | Pending |
| BR-INTEGRATION-1488 | Validate rejects openai without apiKeyFile or oauth2 | P0 | Unit | UT-SH-1488-008 | Pending |
| BR-INTEGRATION-1488 | Validate accepts openai_compatible without apiKeyFile | P0 | Unit | UT-SH-1488-009 | Pending |
| BR-INTEGRATION-1488 | YAML round-trip preserves AF config format | P0 | Unit | UT-SH-1488-010 | Pending |
| BR-INTEGRATION-1488 | YAML round-trip preserves KA runtime config format | P0 | Unit | UT-SH-1488-011 | Pending |
| BR-INTEGRATION-1488 | Validate enforces TLS cert pair rule | P1 | Unit | UT-SH-1488-012 | Pending |
| BR-INTEGRATION-1488 | Validate enforces OAuth2 constraints | P1 | Unit | UT-SH-1488-013 | Pending |
| BR-INTEGRATION-1488 | Validate enforces phaseModels key whitelist | P1 | Unit | UT-SH-1488-014 | Pending |
| BR-INTEGRATION-1488 | LLMHeaderDef validation (exactly one source) | P0 | Unit | UT-SH-1488-015 | Pending |
| BR-INTEGRATION-1488 | AF NewModelFromConfig works with shared type | P0 | Integration | IT-AF-1488-001 | Pending |
| BR-INTEGRATION-1488 | AF buildTransportChain works with shared headers | P0 | Integration | IT-AF-1488-002 | Pending |
| BR-INTEGRATION-1488 | KA buildLLMClientFromConfig works with shared type | P0 | Integration | IT-KA-1488-001 | Pending |
| BR-INTEGRATION-1488 | KA hot-reload merges runtime into shared type | P0 | Integration | IT-KA-1488-002 | Pending |
| BR-INTEGRATION-1488 | KA apiKeyFile resolution at reload time | P0 | Integration | IT-KA-1488-003 | Pending |

---

## 7. FedRAMP / SOC2 Control Verification

### IA-5 -- Authenticator Management

**Business behavior**: API keys for LLM authentication are resolved from mounted
Kubernetes Secret files (`apiKeyFile`), never stored inline in ConfigMaps.

**Tests**: UT-SH-1488-003, UT-SH-1488-004, IT-KA-1488-003

### SC-8 -- Transmission Confidentiality and Integrity

**Business behavior**: TLS configuration (CA, client certs) is carried through
the shared type and injected into transport chains for both services.

**Tests**: UT-SH-1488-012, IT-AF-1488-002

### CM-6 -- Configuration Settings

**Business behavior**: Configuration validation is centralized in the shared
type, enforcing consistent rules across services. Unknown providers are
rejected. Required fields are enforced per provider.

**Tests**: UT-SH-1488-005 through UT-SH-1488-014

### SI-10 -- Information Input Validation

**Business behavior**: Both services reject misconfigured LLM settings at
startup via the shared `Validate()` method, preventing malformed requests.

**Tests**: UT-SH-1488-006 through UT-SH-1488-009

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-1488-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `SH` (Shared), `AF` (API Frontend), `KA` (Kubernaut Agent)

### Tier 1: Unit Tests

#### Shared Type (`pkg/shared/types/llm_test.go`)

| ID | Business Outcome Under Test | FedRAMP |
|----|----------------------------|---------|
| UT-SH-1488-001 [CM-6] | LLMConfig struct deserializes from YAML with all fields populated | CM-6 |
| UT-SH-1488-002 [CM-6] | Provider constants cover the full AF+KA union set (vertex_ai, gemini, anthropic, openai, openai_compatible) | CM-6 |
| UT-SH-1488-003 [IA-5] | ResolveAPIKey reads file contents into APIKey field, trims whitespace | IA-5 |
| UT-SH-1488-004 [IA-5] | ResolveAPIKey is a no-op when APIKeyFile is empty (supports keyless providers) | IA-5 |
| UT-SH-1488-005 [SI-10] | Validate accepts vertex_ai with vertexProject + vertexLocation | SI-10 |
| UT-SH-1488-006 [SI-10] | Validate rejects unknown provider | SI-10 |
| UT-SH-1488-007 [SI-10] | Validate rejects openai/openai_compatible without endpoint | SI-10 |
| UT-SH-1488-008 [SI-10] | Validate rejects openai without apiKeyFile or oauth2 | SI-10 |
| UT-SH-1488-009 [SI-10] | Validate accepts openai_compatible without apiKeyFile (keyless) | SI-10 |
| UT-SH-1488-010 [CM-6] | YAML round-trip preserves AF config format (fields present in AF) | CM-6 |
| UT-SH-1488-011 [CM-6] | YAML round-trip preserves KA runtime format (temperature, maxRetries, phaseModels) | CM-6 |
| UT-SH-1488-012 [SC-8] | Validate enforces TLS cert/key pair rule (both or neither) | SC-8 |
| UT-SH-1488-013 [SI-10] | Validate enforces OAuth2 constraints (tokenURL required when enabled) | SI-10 |
| UT-SH-1488-014 [SI-10] | Validate rejects unknown phaseModels keys | SI-10 |
| UT-SH-1488-015 [SI-10] | LLMHeaderDef validation: exactly one of value/secretKeyRef/filePath | SI-10 |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | FedRAMP |
|----|----------------------------|---------|
| IT-AF-1488-001 [CM-6] | AF `NewModelFromConfig` dispatches correctly with `types.LLMConfig` | CM-6 |
| IT-AF-1488-002 [SC-8] | AF `buildTransportChain` injects TLS and custom headers from shared type | SC-8 |
| IT-KA-1488-001 [CM-6] | KA `buildLLMClientFromConfig` constructs client from single `types.LLMConfig` | CM-6 |
| IT-KA-1488-002 [CM-6] | KA hot-reload callback merges runtime fields into base `types.LLMConfig` | CM-6 |
| IT-KA-1488-003 [IA-5] | KA `apiKeyFile` resolution works at reload time | IA-5 |

---

## 9. Wiring Manifest (Pyramid Invariant)

> UT proves logic. IT proves wiring. E2E proves the journey.

| Component | Production Entry Point | Wiring Location | UT (logic) | IT (wiring) |
|-----------|----------------------|-----------------|------------|-------------|
| `types.LLMConfig` | `NewModelFromConfig()` (AF) | `pkg/apifrontend/launcher/model.go` | UT-SH-1488-001..015 | IT-AF-1488-001 |
| `types.LLMConfig` | `buildLLMClientFromConfig()` (KA) | `cmd/kubernautagent/llm_builder.go` | UT-SH-1488-001..015 | IT-KA-1488-001 |
| `types.LLMHeaderDef` | `buildTransportChain()` (AF, KA) | `model.go`, `llm_builder.go` | UT-SH-1488-015 | IT-AF-1488-002 |
| `ResolveAPIKey()` | `llmRuntimeReloadCallback()` | `cmd/kubernautagent/llm_builder.go` | UT-SH-1488-003..004 | IT-KA-1488-003 |

---

## 10. TDD Execution Order

1. **Phase 1 (RED -- Shared Type)**: Write failing tests UT-SH-1488-001 through -015
2. **Phase 2 (GREEN -- Shared Type)**: Implement `pkg/shared/types/llm.go`
3. **Phase 3 (GREEN -- CHECKPOINT W)**: Verify build, all shared type tests pass
4. **Phase 4 (REFACTOR -- Shared Type)**: Standard TDD refactoring + kubernaut-specific
5. **Phase 5 (RED -- AF Migration)**: Write/update failing IT-AF-1488-001, -002
6. **Phase 6 (GREEN -- AF Migration)**: Migrate AF to shared type
7. **Phase 7 (GREEN -- CHECKPOINT W)**: Verify AF tests pass, wiring verified
8. **Phase 8 (REFACTOR -- AF)**: Standard TDD refactoring + kubernaut-specific
9. **Phase 9 (RED -- KA Migration)**: Write/update failing IT-KA-1488-001..003
10. **Phase 10 (GREEN -- KA Migration)**: Migrate KA to shared type + apiKeyFile
11. **Phase 11 (GREEN -- CHECKPOINT W)**: Verify KA tests pass, wiring verified
12. **Phase 12 (REFACTOR -- KA)**: Standard TDD refactoring + kubernaut-specific

---

## 11. Environmental Needs

### 11.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Location**: `pkg/shared/types/llm_test.go`
- **Dependencies**: `os.WriteFile` for API key file fixtures

### 11.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: `httptest.NewServer` for LLM endpoint mock
- **Location**: `test/integration/apifrontend/`, `test/integration/kubernautagent/`

---

## 12. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/tests/1488/TEST_PLAN.md` |
| Shared type unit tests | `pkg/shared/types/llm_test.go` |
| AF integration tests | `test/integration/apifrontend/` (existing files updated) |
| KA integration tests | `test/integration/kubernautagent/` (existing files updated) |

---

## 13. Execution

```bash
# Unit tests (shared types)
make test-unit FOCUS="1488"

# Integration tests
make test-integration-apifrontend FOCUS="1488"
make test-integration-kubernautagent FOCUS="1488"

# Full regression
make test-unit
```

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-06-24 | Initial test plan |
