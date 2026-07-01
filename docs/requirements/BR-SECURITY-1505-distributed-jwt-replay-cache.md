# BR-SECURITY-1505: Distributed JWT Replay Cache (Valkey-backed)

**Business Requirement ID**: BR-SECURITY-1505
**Category**: API Frontend — Authentication & Security
**Priority**: **P2 (MEDIUM)** — Closes a MEDIUM-severity GA readiness gap
**Target Version**: **V1.5**
**Status**: ✅ Implemented
**Date**: June 30, 2026
**GitHub Issues**: [#1505](https://github.com/jordigilh/kubernaut/issues/1505) (GAP-08)

---

## Business Need

### Problem Statement

APIFrontend detects replayed JWTs (a token intercepted and reused after legitimate use) by tracking each token's `jti` claim in a `ReplayCache`. Before this BR, that cache was **in-memory and per-process** (`pkg/apifrontend/auth/replay_cache.go`): each APIFrontend replica maintains its own independent map of seen `jti` values.

This was identified as **GAP-08 (MEDIUM severity)** in the GA Readiness Audit (issue #1505): in a multi-replica deployment (`apifrontend.replicas > 1`, or any HPA-scaled deployment), a token replayed against a *different* replica than the one that first observed it goes undetected — the replay protection silently degrades as replica count grows, without any visible failure.

Separately, `kubernaut-operator`'s `DataStorageConfigMap`-equivalent for APIFrontend already emitted an `auth.replayCache` config block (`backend: redis`, pointing at the cluster's shared Valkey instance) that kubernaut's Go code never consumed — the config contract existed on the operator side with no corresponding implementation.

### Impact Without This BR

- Token replay protection is only as strong as a single replica's uptime and request-routing luck; horizontally scaling APIFrontend for availability directly *weakens* a security control without any indication in logs, metrics, or config validation.
- `kubernaut-operator`-managed deployments configure a `redis` replay-cache backend that kubernaut's Go code silently ignores (unknown YAML field), giving operators false confidence that distributed replay protection is active.

---

## Decision: Valkey-backed Distributed Cache, Swap-Compatible with the Existing In-Memory Cache

A `ReplayCacheStore` interface (`pkg/apifrontend/auth/replay_cache.go`) now abstracts the replay-detection contract (`MissingJTI`, `Seen`, `Stop`), implemented by both:

1. **`ReplayCache`** (existing, unchanged behavior) — in-memory, single-process. Still the default when no distributed backend is configured, preserving backward compatibility for single-replica and dev/test deployments.
2. **`ValkeyReplayCache`** (new) — backed by the cluster's shared Valkey/Redis instance (the same instance and Secret already used by DataStorage), using atomic `SET ... NX EX <ttl>` semantics so a single round-trip both checks and marks a `jti`, avoiding a race between two replicas processing the same replayed token near-simultaneously.

Key design choices:

1. **Config-driven backend selection** (`auth.replayCache.backend`: `redis`/`valkey`/`memory`/unset), mirroring the `afReplayCacheYAML` contract already emitted by `kubernaut-operator`, so the same config shape works whether APIFrontend is Helm-managed or operator-managed.
2. **Fail-open with logged degradation, not fail-closed**: if Valkey is unreachable at startup, APIFrontend falls back to the in-memory cache rather than refusing to start — replay detection is defense-in-depth layered on top of signature/expiry/audience/issuer validation, not the sole authentication control, so a Valkey outage should degrade a secondary control, not take down authentication entirely. The same fail-open behavior applies per-request: if a `SETNX` call to Valkey errors after startup, that single check is treated as "not seen" (allowed) and logged, rather than blocking the request.
3. **Backward compatible**: the legacy `auth.enableReplayProtection: true` boolean continues to select the in-memory cache when no `auth.replayCache` block is present.
4. **Reuses existing infrastructure**: no new Secret, Valkey instance, or Helm value beyond an `enabled` flag — the distributed cache uses the same `valkey.existingSecret`/`valkey-secret` and shared Valkey address already deployed for DataStorage.

---

## Success Criteria

1. `ReplayCacheStore` interface allows `JWTValidator` to remain agnostic to the backing store; existing callers using the in-memory `*ReplayCache` are unaffected (interface satisfied without changes).
2. `ValkeyReplayCache.Seen()` correctly detects a replay across two independent `redis.Client` instances pointed at the same Valkey instance (simulating two APIFrontend replicas).
3. A Valkey outage at startup falls back to the in-memory cache (logged), rather than failing APIFrontend startup or silently disabling replay protection.
4. A Valkey outage at request time fails open (request allowed, logged) rather than blocking authenticated traffic.
5. `auth.replayCache` config validation rejects an unknown backend and a `redis`/`valkey` backend missing `redisAddr`.
6. Helm: `apifrontend.config.auth.replayCache.enabled=true` wires the ConfigMap block and mounts the existing shared Valkey secret; disabled by default (backward compatible, matches pre-GAP-08 behavior for single-replica/dev deployments).

---

## Related Documents

- [Kubernaut Helm Chart README — Optional: Distributed JWT Replay Cache](../../charts/kubernaut/README.md#optional-distributed-jwt-replay-cache-gap-08)

---

**Document Version**: 1.0
**Last Updated**: June 30, 2026
**Maintained By**: Kubernaut Architecture Team
