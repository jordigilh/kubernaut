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

All tests in `test/unit/shared/tls/profile_test.go`.

| ID | Description | BR |
|----|-------------|-----|
| UT-TLS-748-002 | Intermediate cipher suites contain exactly the 6 AEAD ECDHE suites | BR-SECURITY-748 |
| UT-TLS-748-003 | Old profile includes all Intermediate ciphers plus CBC/RSA fallbacks | BR-SECURITY-748 |
| UT-TLS-748-004 | Modern MinTLSVersion is 1.3 and CipherSuites is empty | BR-SECURITY-748 |
| UT-TLS-748-005 | Curve preferences: X25519 first, followed by P-256 and P-384 | BR-SECURITY-748 |
| UT-TLS-748-010 | `ApplyProfile` overlays MinVersion, CipherSuites, CurvePreferences | BR-SECURITY-748 |
| UT-TLS-748-011 | `ApplyProfile` overrides MinVersion when profile requires higher | BR-SECURITY-748 |
| UT-TLS-748-012 | `ApplyProfile` with nil profile leaves config unchanged | BR-SECURITY-748 |
| UT-TLS-748-013 | `ApplyProfile` with nil config does not panic | BR-SECURITY-748 |
| UT-TLS-748-014 | MaxTLSVersion is applied for Custom profiles | BR-SECURITY-748 |
| UT-TLS-748-020 | `ProfileForType` returns matching profile for each known type | BR-SECURITY-748 |
| UT-TLS-748-021 | `ProfileForType("Custom")` returns nil (requires explicit construction) | BR-SECURITY-748 |
| UT-TLS-748-022 | `ProfileForType` returns nil for unrecognized type | BR-SECURITY-748 |
| UT-TLS-748-050 | Modern profile upgrades server MinVersion to TLS 1.3 | BR-SECURITY-748 |
| UT-TLS-748-051 | Intermediate profile sets cipher suites on server | BR-SECURITY-748 |
| UT-TLS-748-052 | Old profile lowers server MinVersion to TLS 1.0 | BR-SECURITY-748 |
| UT-TLS-748-060 | Server uses TLS 1.2 with no cipher restriction when profile is absent | BR-SECURITY-748 |
| UT-TLS-748-061 | Empty config string is a no-op | BR-SECURITY-748 |
| UT-TLS-748-070 | `Intermediate` config value produces TLS 1.2 AEAD on server | BR-SECURITY-748 |
| UT-TLS-748-071 | `Modern` config value produces TLS 1.3 on server | BR-SECURITY-748 |
| UT-TLS-748-080 | Unknown profile name returns error and preserves TLS 1.2 default | BR-SECURITY-748 |
| UT-TLS-748-090 | Modern profile upgrades client transport to TLS 1.3 | BR-SECURITY-748 |
| UT-TLS-748-091 | No profile preserves TLS 1.2 on client transport | BR-SECURITY-748 |
| UT-TLS-748-100 | Mutating one Intermediate profile does not affect another (immutability) | BR-SECURITY-748 |
| UT-TLS-748-101 | Mutating Old profile curves does not affect Modern profile | BR-SECURITY-748 |

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
