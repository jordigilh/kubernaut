# Test Plan: TLS Security Profiles

> **Template Version**: 2.0 — Hybrid IEEE 829 + Kubernaut

**Test Plan Identifier**: TP-748-v1.0
**Feature**: Configurable TLS security profiles (Old/Intermediate/Modern) for all Kubernaut services
**Version**: 1.0
**Created**: 2026-04-29
**Author**: AI Assistant
**Status**: Active
**Branch**: `main`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the TLS security profiles feature introduced by Issue #748. All 10 Kubernaut services support a `tlsProfile` configuration field that maps to predefined cipher suite and TLS version combinations, aligned with OpenShift's TLS security profile concept.

### 1.2 Objectives

1. Validate all three built-in profiles (Old, Intermediate, Modern) configure correct cipher suites and TLS versions
2. Validate `SetDefaultSecurityProfileFromConfig` applies the profile process-wide
3. Validate empty/omitted `tlsProfile` preserves default TLS 1.2 behavior (vanilla K8s)
4. Validate invalid profile name produces error
5. Validate profile immutability (no mutation of built-in profiles)

### 1.3 Success Metrics

| Metric | Target |
|--------|--------|
| Unit test pass rate | 100% |
| All 10 services wired | Verified via grep |

---

## 2. References

- [Issue #748](https://github.com/jordigilh/kubernaut/issues/748) — TLS security profiles
- [ADR-TLS-001](../../architecture/decisions/ADR-TLS-001-openshift-tls-security-profiles.md) — OpenShift TLS security profiles
- BR-SECURITY-748 — TLS security profiles
- `pkg/shared/tls/profile.go` — `SecurityProfile`, `ProfileForType`, `SetDefaultSecurityProfileFromConfig`

---

## 3. Scope

### 3.1 In Scope

- Built-in profiles: Old (TLS 1.0+), Intermediate (TLS 1.2+), Modern (TLS 1.3 only)
- Server-side TLS configuration (`ConfigureConditionalTLS`)
- Client-side TLS transports (`NewTLSTransport`, CA reloader)
- Profile application in all 10 service `main.go` files
- Error handling for invalid profiles

### 3.2 Out of Scope

- Custom profile (operator-side resolution — requires Kubernaut Operator)
- OCP APIServer CR reconciliation (operator responsibility)
- Certificate management (separate feature, #756)

### 3.3 Environment Considerations

On vanilla Kubernetes (Kind), `tlsProfile` is typically empty — the feature is a no-op with default TLS 1.2 behavior. Full profile testing (especially Old with legacy ciphers) requires explicit configuration. OCP-specific behavior requires the Kubernaut Operator and an OpenShift cluster.

---

## 4. Test Scenarios

### 4.1 Unit Tests

| ID | Description | BR |
|----|-------------|-----|
| UT-TLS-748-001 | `ProfileForType("Old")` returns correct cipher suites and min TLS version | BR-SECURITY-748 |
| UT-TLS-748-002 | `ProfileForType("Intermediate")` returns TLS 1.2+ ciphers | BR-SECURITY-748 |
| UT-TLS-748-003 | `ProfileForType("Modern")` returns TLS 1.3 only | BR-SECURITY-748 |
| UT-TLS-748-004 | `ProfileForType("")` returns nil (no-op) | BR-SECURITY-748 |
| UT-TLS-748-005 | `ProfileForType("Custom")` returns nil (operator-side) | BR-SECURITY-748 |
| UT-TLS-748-006 | `ProfileForType("invalid")` returns nil | BR-SECURITY-748 |
| UT-TLS-748-010 | `SetDefaultSecurityProfileFromConfig("")` is no-op | BR-SECURITY-748 |
| UT-TLS-748-011 | `SetDefaultSecurityProfileFromConfig("Intermediate")` sets process-wide default | BR-SECURITY-748 |
| UT-TLS-748-012 | `SetDefaultSecurityProfileFromConfig("invalid")` returns error | BR-SECURITY-748 |
| UT-TLS-748-020 | `ConfigureConditionalTLS` applies active profile to server | BR-SECURITY-748 |
| UT-TLS-748-021 | `NewTLSTransport` applies active profile to client | BR-SECURITY-748 |
| UT-TLS-748-030 | `ApplyProfile` does not mutate built-in profile (immutability) | BR-SECURITY-748 |

---

## 5. Existing Test Coverage

| File | Test IDs | Tier |
|------|----------|------|
| `test/unit/shared/tls/profile_test.go` | UT-TLS-748-* | Unit |

---

## 6. Execution

```bash
go test ./test/unit/shared/tls/... -v -run "748"
```

---

## 7. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-29 | Initial test plan — documents existing coverage for QE readiness |
