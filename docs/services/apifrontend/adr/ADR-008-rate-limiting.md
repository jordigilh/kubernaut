# ADR-008: Three-Tier Rate Limiting Architecture

**Status:** Accepted
**Date:** 2026-05-02
**Deciders:** AF team
**Source:** SEC-5 (#59)

## Context

AF exposes LLM-backed endpoints that are expensive (cost per token) and slow (seconds per call). Without rate limiting, a single user could exhaust LLM capacity, a burst of requests could overload the LLM provider, or token costs could spiral.

## Decision

Implement three independent rate limiting tiers:

| Tier | Scope | Mechanism | Purpose |
|------|-------|-----------|---------|
| 1 | Per-user request rate | Token bucket (in-memory) | Prevent single-user abuse |
| 2 | Global LLM concurrency | Semaphore (in-memory) | Prevent LLM provider overload |
| 3 | Per-user token budget | Counter (in-memory, when available) | Cost control |

All tiers evaluated per-request. HTTP 429 returned when any tier is exceeded.

## Consequences

- Defense-in-depth: each tier protects against a different abuse vector
- In-memory state: simple, fast, no external dependency (Redis)
- HA limitation: per-replica state means a user can get 2x budget across 2 replicas
- Tier 3 disabled when LLM provider doesn't report token usage (graceful degradation)
- Tiers 1 and 2 ALWAYS active regardless of token reporting
- Configuration via operator ConfigMap (adjustable without redeploy)

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| Single global rate limit | Doesn't protect against single-user monopolizing capacity |
| Redis-backed distributed rate limit | External dependency; operational complexity; <100 users doesn't justify |
| No rate limiting (rely on LLM provider) | Provider limits are account-wide; one AF user could exhaust quota for all |
| API gateway rate limiting only | Doesn't know about token budgets or LLM concurrency; too coarse |
