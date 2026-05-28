# Test Plan — #1309: Auto-Detect Auth Mode from JWT Provider Presence

**IEEE 829 Compliant** | **Issue**: [#1309](https://github.com/jordigilh/kubernaut/issues/1309)

## 1. Test Plan Identifier

TP-1309-AUTH-AUTODETECT

## 2. Introduction

Remove the explicit `kubernetesAuthEnabled` toggle from the AF configuration and
auto-detect the authentication mode from JWT provider presence:

- `issuerURL` configured → OIDC/JWKS validation only (no TokenReview fallback)
- `issuerURL` empty → K8s TokenReview only

**Business Requirements**:
- BR-AUTH-1309-001: Auth mode auto-detected from JWT provider presence
- BR-AUTH-1309-002: No dual-auth — each deployment uses exactly one mechanism

**FedRAMP Control Mapping**:

| Control | Objective | Test Coverage |
|---------|-----------|---------------|
| IA-2    | Identification and authentication | Single deterministic auth per deployment |
| AC-6    | Least privilege | Auth mode determined by config, not runtime toggle |
| CM-6    | Configuration settings | Config surface simplified, toggle removed |

## 3. Test Items

| Item | File | Description |
|------|------|-------------|
| `JWTValidator.Validate` | `pkg/apifrontend/auth/jwt.go` | Fallback gate: `reviewer != nil` only |
| `JWTValidator` struct | `pkg/apifrontend/auth/jwt.go` | `k8sEnabled` field removed |
| `Config` (auth pkg) | `pkg/apifrontend/auth/config.go` | `KubernetesAuthConfig` struct removed |
| `AuthConfig` (config pkg) | `pkg/apifrontend/config/config.go` | `KubernetesAuthEnabled` field removed |
| `buildAuthMiddleware` | `cmd/apifrontend/main.go` | Auto-detect wiring via `len(ac.JWT)` |
| Deploy configs | `deploy/apifrontend/base/config.yaml`, `overlays/e2e/config.yaml` | Toggle removed |
| Helm chart | `charts/kubernaut/values.yaml`, `templates/apifrontend/apifrontend.yaml` | Toggle removed |
| Helm schema | `charts/kubernaut/values.schema.json` | Property removed |

## 4. Pyramid Invariant

```
UT  proves logic   --> jwt.go fallback gate, config parsing, validator construction
IT  proves wiring  --> main.go -> buildAuthMiddleware -> validator -> middleware -> envtest TokenReview
E2E proves journey --> Real AF pod with DEX: human JWT accepted, SA token rejected, no toggle
```

## 5. Features to Be Tested

### 5.1 Tier 1: Unit Tests — prove logic

#### Auth validator logic (`pkg/apifrontend/auth/jwt_test.go`)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1309-001 | BR-AUTH-1309-002 | IA-2 | OIDC mode: validator with JWT providers, NO reviewer | Opaque SA token gets `ErrMalformedToken` (no fallback) |
| UT-AF-1309-002 | BR-AUTH-1309-001 | IA-2 | TokenReview mode: validator with NO JWT providers, WITH reviewer | Opaque token authenticated via `TokenReviewer.Validate` |
| UT-AF-1309-003 | BR-AUTH-1309-001 | IA-2 | TokenReview mode: reviewer returns error | Malformed token rejected |
| UT-AF-1309-004 | BR-AUTH-1309-002 | IA-2 | OIDC mode: unknown issuer JWT | `ErrUnknownIssuer` returned, no fallback |
| UT-AF-1309-005 | BR-AUTH-1309-001 | IA-2 | Neither mode: no JWT providers, no reviewer | Clear error returned |

#### Config parsing (`pkg/apifrontend/config/config_test.go`)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1309-010 | BR-AUTH-1309-001 | CM-6 | Config YAML without `kubernetesAuthEnabled` | Loads without error |
| UT-AF-1309-011 | BR-AUTH-1309-001 | CM-6 | Config YAML with stale `kubernetesAuthEnabled: true` | Loads without error (backward compat) |

### 5.2 Tier 1b: Wiring Unit Tests — prove buildAuthMiddleware auto-detect

#### `cmd/apifrontend/main_wiring_test.go`

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1309-020 | BR-AUTH-1309-002 | AC-6 | `buildAuthMiddleware` with `issuerURL` set | Opaque token gets 401 (no TokenReview wired) |
| UT-AF-1309-021 | BR-AUTH-1309-001 | AC-6 | `buildAuthMiddleware` with empty `issuerURL` | Pass-through auth returned (no OIDC) |

#### `cmd/apifrontend/helpers_test.go`

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1309-022 | BR-AUTH-1309-001 | CM-6 | `buildAuthConfig` with `issuerURL` | Returns populated `JWT` slice |
| UT-AF-1309-023 | BR-AUTH-1309-001 | CM-6 | `buildAuthConfig` with empty `issuerURL` | Returns empty `JWT` slice |

### 5.3 Tier 2: Integration Tests — prove wiring with real K8s

#### `test/integration/apifrontend/auth_middleware_test.go`

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-AF-1309-001 | BR-AUTH-1309-002 | IA-2 | OIDC mode wiring: JWKS + NO reviewer | envtest SA token → 401 Unauthorized |
| IT-AF-1309-002 | BR-AUTH-1309-001 | IA-2 | TokenReview wiring: NO JWKS + WITH reviewer | envtest SA token → 200 OK, `IsServiceAccount: true` |
| IT-AF-1309-003 | BR-AUTH-1309-001 | AC-6 | TokenReview audit event | `auth.success` with `auth_method: token_review` |

### 5.4 Tier 3: E2E Tests — prove the journey (existing suite as regression gate)

No new E2E tests. Deploy config change removes `kubernetesAuthEnabled`. Existing tests validate:

| Existing Test | Validates |
|---------------|-----------|
| `phase1_test.go:114-130` | DEX JWT accepted at `/a2a/invoke` (OIDC mode works) |
| `phase1_test.go:89-98` | Unauthenticated request rejected (auth active) |
| E2E-1293-004 | SA token to AF rejected (no TokenReview in OIDC mode) |
| All E2E tests | DEX JWT auth (no regression) |

## 6. Acceptance Criteria

1. No references to `kubernetesAuthEnabled`, `KubernetesAuthEnabled`, `KubernetesAuthConfig`, or `k8sEnabled` remain in `.go` or `.yaml` files
2. All UT/IT pass: `go test ./pkg/apifrontend/auth/... ./pkg/apifrontend/config/... ./cmd/apifrontend/... ./test/integration/apifrontend/... -count=1`
3. E2E suite passes with config change: `make test-e2e-apifrontend`
4. `go build ./...` and `golangci-lint run --timeout=5m` clean

## 7. Test Environment

- **UT**: Standard Go test runner with httptest
- **IT**: envtest (real kube-apiserver, etcd) for TokenReview validation
- **E2E**: Kind cluster with DEX, mock-LLM, TLS certificates
