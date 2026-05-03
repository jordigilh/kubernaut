# ADR-009: Go Module Dependency Strategy

**Status:** Accepted
**Date:** 2026-05-02
**Deciders:** AF team
**Source:** IMP-3 (#61)

## Context

AF needs kubernaut's CRD types (RemediationRequest, AIAnalysis, SignalProcessing), shared types (EnrichmentResults, DetectedLabels), and utility packages (fingerprint, signal processing). These live in the kubernaut monorepo.

## Decision

Direct `go.mod` dependency on the kubernaut monorepo:

```
require github.com/jordigilh/kubernaut v1.5.0-alpha.X
```

Import specific packages:
- `api/remediation/v1alpha1`
- `api/aianalysis/v1alpha1`
- `api/signalprocessing/v1alpha1`
- `pkg/shared/types`
- `pkg/gateway/types` (SHA256 fingerprinting via `ResolveFingerprint`, owner resolution)
- `pkg/gateway/processing` (signal processing types)

## Consequences

- Compile-time type safety: AF uses exact same types as kubernaut controllers
- Contract tests (QE-4) catch breaking changes at compile time
- Version pinning strategy: dev (branch), CI (commit SHA), release (tag)
- Pulls in kubernaut's transitive dependencies (mitigated by Go's module pruning)
- Breaking kubernaut type changes require coordinated AF update
- Nightly CI job against kubernaut `main` provides early break detection

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| Extracted shared module (`kubernaut-types`) | Adds release coordination overhead; kubernaut team owns the types, not a shared repo |
| Copy types into AF repo | Types drift silently; no compile-time safety; maintenance burden |
| Protobuf/OpenAPI generated types | kubernaut uses kubebuilder (Go types are source of truth); generation adds indirection |
| Interface-only dependency (no types) | Loses compile-time safety; runtime schema validation is more fragile |
