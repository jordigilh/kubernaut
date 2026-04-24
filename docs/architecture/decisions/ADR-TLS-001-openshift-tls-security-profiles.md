# ADR-TLS-001: OpenShift TLS Security Profile Support

**Status**: ACCEPTED
**Date**: 2026-03-04
**Issue**: [#748](https://github.com/jordigilh/kubernaut/issues/748)
**Operator Issue**: [kubernaut-operator#3](https://github.com/jordigilh/kubernaut-operator/issues/3)

## Context

OpenShift clusters enforce cluster-wide TLS security profiles via the `APIServer` custom resource (`config.openshift.io/v1`). These profiles — Old, Intermediate (default), Modern, and Custom — define minimum TLS versions, cipher suites, and curve preferences that all components should respect.

Kubernaut services hardcode `MinVersion: tls.VersionTLS12` for both server-side (`ConfigureConditionalTLS`) and client-side (`buildCATransport`, `NewTLSTransport`) TLS. This is equivalent to the Intermediate profile but does not constrain cipher suites or respond to cluster-wide policy changes.

For OCP certification and production deployments, Kubernaut must honor the operator-injected TLS security profile.

## Decision

### Scope: OCP-only

TLS security profiles apply exclusively to OpenShift deployments. Vanilla Kubernetes and Kind environments remain unaffected — when the environment variable is not set, the existing hardcoded `MinVersion: tls.VersionTLS12` behavior is preserved.

### Architecture

A process-wide security profile is set once at startup via a package-level setter in `pkg/shared/tls`. All existing TLS configuration points read this profile internally — no function signature changes were required.

```
Operator                         ConfigMap YAML           Service Binary
   |                                  |                        |
   | writes tlsProfile: ...           |                        |
   +--------------------------------->|                        |
                                      |   config.LoadFromFile  |
                                      +----------------------->|
                                                               |  SetDefaultSecurityProfileFromConfig(cfg.TLSProfile)
                                                               |      |
                                                               |      v
                                                               |  [package-level SecurityProfile]
                                                               |      |
                                                               |      +---> ConfigureConditionalTLS (server)
                                                               |      +---> buildCATransport (client CA reloader)
                                                               |      +---> NewTLSTransport (client one-shot)
```

### Configuration Contract

Each service's YAML configuration includes a top-level `tlsProfile` field:

```yaml
# Values: Old | Intermediate | Modern
# Omit for vanilla K8s / Kind (no profile applied, uses TLS 1.2 default)
tlsProfile: Intermediate
```

The `kubernaut-operator` reads the cluster `APIServer` CR, resolves the effective profile type, and writes it into each service's ConfigMap. This follows the standard Kubernetes pattern of configuration via mounted ConfigMaps rather than environment variables.

### Built-in Profile Definitions

| Profile | MinTLS | MaxTLS | Cipher Suites | Curves |
|---------|--------|--------|---------------|--------|
| Old | TLS 1.0 | — | AEAD ECDHE + CBC/RSA fallbacks (15 suites) | X25519, P-256, P-384 |
| Intermediate | TLS 1.2 | — | AEAD ECDHE only (6 suites) | X25519, P-256, P-384 |
| Modern | TLS 1.3 | — | Auto (Go TLS 1.3 cipher selection) | X25519, P-256, P-384 |
| Custom | Per JSON | Per JSON | Per JSON | Per JSON |

DHE cipher suites from the OpenShift profile specification are omitted because Go's `crypto/tls` does not support finite-field Diffie-Hellman key exchange.

### Integration Points

**Modified files** (body changes only, no signature changes):
- `pkg/shared/tls/tls.go` — `ConfigureConditionalTLS`, `NewTLSTransport`
- `pkg/shared/tls/ca_reloader.go` — `buildCATransport`

**New file**:
- `pkg/shared/tls/profile.go` — `SecurityProfile` type, built-in profiles, `ApplyProfile`, `SetDefaultSecurityProfile`, `SetDefaultSecurityProfileFromConfig`

**Config structs** (all 10 services):
- `TLSProfile string` field added with YAML tag `tlsProfile,omitempty`

**Service wiring** (all 10 `cmd/*/main.go`):
- `SetDefaultSecurityProfileFromConfig(cfg.TLSProfile)` called before `StartCAFileWatcher()`

### What is NOT in scope

- Reading the `APIServer` CR directly from Kubernaut services (this is the operator's responsibility)
- Modifying vanilla K8s / Kind behavior
- Deprecated `pkg/effectivenessmonitor/client/ca_reloader.go` (production uses shared TLS)
- External service clients (e.g., AWX `InsecureSkipVerify`)
- Test infrastructure (`test/infrastructure/interservice_tls.go`)

## Consequences

### Positive

- Kubernaut respects cluster-wide TLS policy on OpenShift without importing the OpenShift API
- Zero signature changes — existing call sites and tests are unaffected
- Vanilla K8s deployments are completely unaffected (env var is simply absent)
- The package-level setter pattern is consistent with the existing `TLS_CA_FILE` / `DefaultBaseTransport` singleton pattern

### Negative

- DHE cipher suites cannot be supported due to Go's `crypto/tls` limitations
- Custom profiles require operator-side resolution (the operator maps OCP custom ciphers to a built-in profile or extends the YAML schema)
- The profile is set once at startup; mid-flight profile changes require a pod restart

### Risks

- If the operator sets an invalid profile name, `SetDefaultSecurityProfileFromConfig` returns an error. Each service logs the error and falls back to the hardcoded TLS 1.2 default. The service does **not** crash on invalid profile names.

## References

- [OpenShift TLS Security Profiles](https://docs.openshift.com/container-platform/4.17/security/tls-security-profiles.html)
- [Mozilla Server-Side TLS](https://wiki.mozilla.org/Security/Server_Side_TLS)
- [kubernaut-operator#3](https://github.com/jordigilh/kubernaut-operator/issues/3) — Operator-side profile injection
