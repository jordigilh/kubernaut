# TDD Execution Prompt Template

Use this prompt when asking the AI to execute a TDD plan for a PR.

---

## The Prompt

> Perform a rigorous due diligence and address all risks, gaps, and any other findings you encounter first, then create the test plan using @docs/testing/TEST_PLAN_TEMPLATE.md and store it in `docs/tests/{issue_number}/`. Tests must provide behavioral assurance against business acceptance criteria covering at least 80% of the testable code per tier and avoiding anti-patterns as defined in @docs/development/business-requirements/TESTING_GUIDELINES.md.
>
> Break down each TDD phase into an individual implementation phase: TDD Red (write failing tests), then TDD Green (implement to pass), then TDD Refactor (improve code quality), each captured in a single phase. During the TDD Refactor phase, validate that the business code is free of the Go mistakes listed in https://github.com/teivah/100-go-mistakes.
>
> Add checkpoints at your discretion. At each checkpoint, before advancing to the next phase, audit from **all 8 personas** below. For each category, cite the specific test(s) that satisfy it or write them if missing. Do not advance past a checkpoint until all personas are satisfied.
>
> ### 1. QE -- "Can we ship this with confidence?"
>
> 1.1. Every new public function/method has at least one test traceable to a requirement.
>
> 1.2. For every happy-path test there is a corresponding error-path/rejection test.
>
> 1.3. No test anti-patterns: fixed sleeps, tautological assertions, shared mutable state, implementation-coupled assertions.
>
> 1.4. Every bug fix has a regression test; every race fix has a `-race` stress test.
>
> ### 2. Prod Security -- "Can an attacker exploit this?"
>
> 2.1. Every external string input (IDs, names, messages) has a test with: empty, max-length+1, invalid charset (e.g. `"../../etc/passwd"`), Unicode edge cases. Invalid inputs must be rejected.
>
> 2.2. Secrets excluded from logs, metrics labels, and truncated tool responses.
>
> 2.3. RBAC verbs scoped to minimum; impersonation justified in ADR with compensating controls.
>
> 2.4. CI deps pinned to immutable refs (SHA); `govulncheck` clean.
>
> ### 3. Product -- "Does this deliver what was promised?"
>
> 3.1. Every acceptance criterion from the linked issue has a corresponding test or code path.
>
> 3.2. No silent breaking changes to API contracts, CRD schemas, or event formats.
>
> 3.3. No zombie code; deprecated features have notices before removal.
>
> 3.4. User-facing errors are actionable with no internal IDs or stack traces.
>
> ### 4. UX -- "Is this intuitive to consume?"
>
> 4.1. Endpoints, events, schemas documented and consistent with project conventions.
>
> 4.2. Naming follows conventions (snake_case JSON, camelCase Go, kebab-case K8s).
>
> 4.3. Error responses include machine-readable code, human message, and retry context.
>
> 4.4. SSE streams have coherent lifecycle (start, progress, done, heartbeat, retry).
>
> ### 5. Architecture -- "Does this fit the system?"
>
> 5.1. No circular deps; `internal/` never imports `cmd/`.
>
> 5.2. Interfaces at package boundaries; no concrete cross-package types where interfaces suffice.
>
> 5.3. CRD fields additive-only with defaults; implementation matches `docs/adr/`.
>
> 5.4. No test helpers or debug symbols exported from production packages.
>
> ### 6. SRE/Ops -- "Can we run this in production?"
>
> 6.1. For every metric defined, a test calls the production path and asserts the metric changed.
>
> 6.2. Error logs include structured context (resource name, namespace, phase) for SRE diagnosis.
>
> 6.3. Every map/slice/cache that grows with usage has a test running 50+ lifecycle cycles asserting bounded size.
>
> 6.4. Every method behind a mutex has a test with 10+ goroutines under `-race`.
>
> 6.5. External calls have timeouts, circuit breakers, fallbacks; readiness checks reflect real health.
>
> ### 7. Release / Change Mgmt -- "Can we roll this out and back safely?"
>
> 7.1. No irreversible migrations; CRD changes are additive-only.
>
> 7.2. High-risk behavior gated behind feature flags where appropriate.
>
> 7.3. User-visible changes documented; deployment ordering constraints noted.
>
> ### 8. FedRAMP -- "Does this satisfy NIST 800-53?"
>
> 8.1. AU-2/AU-12: security events (auth, session lifecycle, access denial) emit audit events.
>
> 8.2. AC-3/AC-6: RBAC least-privilege; impersonation documented in ADR.
>
> 8.3. IA-5: JWKS rotation with max cache age; fail-open justified in ADR.
>
> 8.4. SC-4/SC-28: PII classified per ADR-017; excluded from logs and metrics.
>
> 8.5. SI-10: external inputs sanitized per spec (RFC 1123, JWT claims, SSE fields).
>
> Escalate if you need input.

---

## Why Each Section Exists

| Section | What it prevents |
|---|---|
| Due diligence first | Executing a plan with incorrect API contracts or missing dependencies |
| Test plan template | Ensures IEEE 829 traceability between requirements and tests |
| 80% coverage + anti-patterns | Prevents hollow tests that pass but prove nothing |
| Red/Green/Refactor separation | Ensures tests fail first (proving they test the right thing) |
| 100-go-mistakes validation | Catches common Go pitfalls during refactor phase |
| 8-persona checkpoint audit | Catches the systematic blind spots TDD naturally misses (see below) |

## What the 8 Personas Catch That TDD Alone Misses

| Persona | Typical TDD blind spot | Real example from PR4 |
|---|---|---|
| QE | Tests exist but only cover happy paths | No error-path test for SessionID validation |
| Prod Security | Tests use well-formed inputs like `"sess-1"` | SessionID used as CRD name without RFC 1123 validation |
| Product | Code works but doesn't match acceptance criteria | Feature complete but no traceability to issue ACs |
| UX | API works but is inconsistent or undocumented | SSE `done` event with nil Content produced empty payload |
| Architecture | Code compiles but leaks abstractions | `CreateRequestWithDefaults` exported from production package |
| SRE/Ops | Metric defined in registry, never incremented in code | `af_sessions_active` gauge never called |
| Release/Change Mgmt | Code works but can't be rolled back | CRD schema change without additive-only check |
| FedRAMP | Functional tests pass, compliance gap | TTL controller auto-cancel without audit trail |

## Companion Documents

- **[PR_REVIEW_PROMPT.md](PR_REVIEW_PROMPT.md)** -- Use when reviewing a PR (same 8 personas, adapted for review instead of TDD)
- **[.cursor/rules/tdd-checkpoint-audit.mdc](../../.cursor/rules/tdd-checkpoint-audit.mdc)** -- Cursor rule that auto-activates during TDD plan execution
- **[.cursor/rules/pr-review-audit.mdc](../../.cursor/rules/pr-review-audit.mdc)** -- Cursor rule that auto-activates during PR review
