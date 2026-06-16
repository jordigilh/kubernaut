# Test Plan: TP-1436 — Multi-Provider JWT Authentication

**Test Plan Identifier**: TP-1436-MULTI-PROVIDER-JWT
**Issue**: [#1436](https://github.com/jordigilh/kubernaut/issues/1436)
**Service**: AF (API Frontend)
**Test ID Format**: `{TIER}-AF-1436-{SEQUENCE}`

## Business Requirements

| ID | Description |
|----|-------------|
| BR-AUTH-1436-001 | AF accepts JWT tokens from multiple configured OIDC issuers concurrently |
| BR-AUTH-1436-002 | Legacy single-provider config (`issuerURL`/`audience`) continues to work unchanged |
| BR-AUTH-1436-003 | Per-provider claim mappings extract correct user identity for RBAC |
| BR-AUTH-1436-004 | Unknown issuer tokens are rejected (no silent accept) |
| BR-AUTH-1436-005 | A2A agents on RHDH/ACM can authenticate via SPIRE JWT-SVIDs |

## FedRAMP Control Mapping

| Control | Objective | How This Issue Addresses It |
|---------|-----------|----------------------------|
| IA-2 | Identification and authentication | Multi-issuer: each deployment can validate tokens from multiple trusted IdPs |
| IA-5 | Authenticator management | Per-provider JWKS endpoints with independent key rotation |
| SC-8 | Transmission confidentiality | JWKS URLs must use HTTPS unless `allowInsecureIssuers` (dev only) |
| SC-23 | Session authenticity | Per-provider audience validation prevents cross-issuer token confusion |
| CM-6 | Configuration settings | Backward-compatible config: legacy flat fields for dev, structured array for production |
| AC-6 | Least privilege | Unknown issuers rejected; no fallback to weaker auth when multiple providers configured |

## Pyramid Invariant

```
UT  proves logic   --> config parsing, validation, buildAuthConfig precedence, claim mappings
IT  proves wiring  --> buildAuthMiddleware -> JWTValidator -> middleware -> two concurrent JWKS providers
E2E proves journey --> Existing DEX E2E suite as regression gate (multi-platform E2E deferred to operator)
```

## Tier 1: Unit Tests — prove logic

### Config parsing and validation (`pkg/apifrontend/config/config_test.go`)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|----|---------|---------|---------| 
| UT-AF-1436-001 | BR-AUTH-1436-001 | CM-6 | Config YAML with `jwtProviders[]` array loads correctly | All provider fields parsed: name, issuerURL, jwksURL, audiences, claimMappings |
| UT-AF-1436-002 | BR-AUTH-1436-002 | CM-6 | Config YAML with legacy `issuerURL` only (no `jwtProviders`) | Loads without error, backward compat preserved |
| UT-AF-1436-003 | BR-AUTH-1436-001 | CM-6 | Config YAML with both `issuerURL` and `jwtProviders` | Loads without error (precedence resolved in buildAuthConfig) |
| UT-AF-1436-004 | BR-AUTH-1436-004 | IA-2 | `jwtProviders` entry with empty `issuerURL` | Validation rejects with clear error |
| UT-AF-1436-005 | BR-AUTH-1436-004 | SC-23 | `jwtProviders` entry with empty `audiences` | Validation rejects: "would reject all tokens" |
| UT-AF-1436-006 | BR-AUTH-1436-001 | IA-2 | `jwtProviders` with duplicate provider names | Validation rejects duplicate |
| UT-AF-1436-007 | BR-AUTH-1436-001 | SC-8 | `jwtProviders` entry with HTTP `jwksURL` and `allowInsecureIssuers: false` | Validation rejects: HTTPS required |
| UT-AF-1436-008 | BR-AUTH-1436-001 | SC-8 | `jwtProviders` entry with HTTP `jwksURL` and `allowInsecureIssuers: true` | Validation accepts (dev/test) |

### buildAuthConfig precedence (`cmd/apifrontend/helpers_test.go`)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|----|---------|---------|---------| 
| UT-AF-1436-010 | BR-AUTH-1436-001 | IA-2 | `jwtProviders` with two providers | Returns `[]ProviderConfig` with both providers, correct issuer URLs and audiences |
| UT-AF-1436-011 | BR-AUTH-1436-002 | CM-6 | Legacy `issuerURL` set, no `jwtProviders` | Returns single-element slice (backward compat) |
| UT-AF-1436-012 | BR-AUTH-1436-001 | CM-6 | Both `issuerURL` and `jwtProviders` set | `jwtProviders` takes precedence, legacy fields ignored |
| UT-AF-1436-013 | BR-AUTH-1436-002 | IA-2 | Neither `issuerURL` nor `jwtProviders` set | Returns empty slice (TokenReview auto-detect, #1309) |
| UT-AF-1436-014 | BR-AUTH-1436-003 | IA-5 | `jwtProviders` with `claimMappings` | ClaimMappings propagated to `auth.ProviderConfig` |

### Auth mode auto-detect interaction (`cmd/apifrontend/helpers_test.go`)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|----|---------|---------|---------| 
| UT-AF-1436-020 | BR-AUTH-1436-001 | IA-2 | `jwtProviders` non-empty triggers OIDC mode | Same behavior as `issuerURL` set per #1309 |
| UT-AF-1436-021 | BR-AUTH-1436-004 | AC-6 | SPIRE issuer with HTTPS OIDC discovery URL | Validation accepts HTTPS OIDC discovery URL |

## Tier 2: Integration Tests — prove wiring

### Multi-issuer middleware (`test/integration/apifrontend/auth_middleware_test.go`)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|----|---------|---------|---------| 
| IT-AF-1436-001 | BR-AUTH-1436-001 | IA-2 | Two MockJWKSServer providers (keycloak + spire), JWT from provider A | 200 OK, UserIdentity extracted with correct username and groups |
| IT-AF-1436-002 | BR-AUTH-1436-005 | IA-2 | Same two providers, JWT from provider B (SPIRE-style issuer) | 200 OK, UserIdentity has SPIRE-issued identity |
| IT-AF-1436-003 | BR-AUTH-1436-004 | AC-6 | Same two providers, JWT from unknown third issuer | 401 Unauthorized, `ErrUnknownIssuer` |

## Tier 3: E2E Tests — prove the journey (regression gate)

No new E2E tests in this PR. Existing DEX E2E suite validates:

| Existing Test | Validates |
|---------------|-----------|
| `phase1_test.go` DEX JWT tests | OIDC mode works with non-Keycloak issuer |
| `phase1_test.go` unauthenticated rejection | Auth middleware active |
| E2E-1293-004 | SA token rejected in OIDC mode |
| All E2E tests | No regression from config refactor |

Multi-platform E2E (real SPIRE JWT-SVID validation on dev cluster) is deferred to operator integration testing after the operator emits `jwtProviders` in the AF ConfigMap.
