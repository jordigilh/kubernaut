# TP-2501-v1.0 — fresh audit observation and external collection

**Status:** Approved 2026-10-10; local implementation verification complete;
batched PR CI pending.
**Issue:** [#2501](https://github.com/jordigilh/kubernaut/issues/2501)
**Approved full plan:** [IEEE 829 hybrid plan](https://github.com/jordigilh/kubernaut/issues/2501#issuecomment-6102903135)

## Purpose, scope and requirements

BR-AUDIT-005/AU-3/CC7.2 and BR-PLATFORM-001.6a: prove the new successful MCP call,
not a stale/shared actor row or first-page substring. Preserve the original
60-second audit assertion. No production timestamp, UUID ordering, replay,
correlation/schema or networking change. Collector transport preserves SC-8 and
AC-6: verified CA/SAN, certificate service authority separate from owned-node
routing, 600-second get-only credentials in read-only files outside the archive.

## Scenarios

| IDs | Contract |
|---|---|
| UT-AF-2501-001/002 | Reject baseline, foreign actor, wrong type/tool/action and failure; accept only attributable fresh success. |
| UT-AF-2501-003 | Inclusive second-aligned bounds accommodate serializer precision; baseline IDs remain excluded. |
| UT-AF-2501-004 | Complete bounded pagination, deduplication and explicit exhaustion/HTTP error, never infinite traversal. |
| UT-AF-2501-005 | Reject insecure/credential-bearing authorities and credentials inside the archive tree before external tools run. |
| UT-AF-2501-006/007 | Unique test-only DEX OIDC identity retains issuer/SRE permissions; AF failure markers use owned temp storage and report write errors. |
| UT-AF-2501-008 | Existing AF provider attributes DEX's opaque subject, not its login email; claim decoding is never used for authorization. |
| IT-AF-2501-001 | Real repo/migrated PostgreSQL + DS API: 85 deterministic higher UUID-ranked same-second rows precede fresh target; all baseline pages covered. |
| IT-AF-2501-002 | Actual runner/subprocess/external image retrieves structured target with supplied TLS/route/files. |
| IT-AF-2501-003 | Correct trust/auth succeeds; wrong CA/hostname/unauthorized credentials fail; SAR proves get allowed and mutation denied. |
| IT-AF-2501-004 | DNS/TCP/401/403/quoted HTTP/invalid-duration errors retain bounded valid JSON and exit/status/stderr. |
| E2E-AF-2501-001 | Real MCP call by unique actor → fresh typed persisted success → query its actual correlation/UUID. |
| E2E-AF-2501-002 | Actual external production collector archives that deployed row without credentials. |

DS supports type/tool/time/correlation/pagination filters, not an actor filter;
compare actor client-side. Never assume MCP IDs are persisted correlations.

## Wiring manifest and approach

| Existing entry | Wiring | Proof |
|---|---|---|
| AF successful-tool E2E | Actual DS query + shared bounded observation helper | IT-001, E2E-001 |
| AF failure callback | `PrepareMustGatherDataStorage` → `RunMustGatherImage` options/mounts | IT-002/003, E2E-002 |
| Production collector entry | `gather.sh` → `datastorage.sh` → real curl | IT-004, E2E-002 |
| Local suite identity fixture | BeforeSuite → DEX ConfigMap/restart → synchronized actor → existing OIDC token helper | UT-006, E2E-001 |
| Parallel failure handoff | ReportAfterEach → AF-owned marker → SynchronizedAfterSuite | UT-007, deployed failure callback |

Ginkgo/Gomega TDD; real internal logic, external HTTP/Kubernetes fixtures only.
RED includes stale/later-page cases and actual TLS/file/time-window/error failures.
GREEN wires complete observation and explicit transport. REFACTOR deduplicates
query decoding, owns credential cleanup and preserves errors. No new production
business component. Code/build/lint/coverage and all wiring rows are final gates.

## Environment, execution, risks and rollback

Separate #2493 source/owned account/netns; real migrations, Podman/kind, four
processes, historical local AF seed `1791470268` and fresh seeds. Preserve bounded
failure artifacts. Suspend for permission expansion, missing infrastructure or
credential/correlation ambiguity. Revert only this follow-up on rollback.

Real integration RED stopped at 50/85 baseline rows; strict TLS/file/error
subprocess RED reproduced failed retrieval and silent success. Those are current
observation defects, not proof of the historical target's exact rank or cause.
Deployed external-image evidence and final full-lane regressions are recorded
below; the earlier diagnostic spike alone was not implementation GREEN or
historical closure.

The deployed RED run reached the spec but rejected Kubernetes service-account
login with HTTP 401; it also demonstrated an invisible parallel-worker failure
after the shared `/tmp` marker write failed silently. The user approved replacing
the login fixture with a unique DEX OIDC identity before specs, retaining existing
SRE group/issuer/token helper, and AF-only owned/error-reporting markers. No
production auth/RBAC or shared failure-marker helper is changed.

## Execution record

- Real OIDC call persisted success UUID `7b3c843c-d13d-48d2-a0e6-6cf10f073b52`;
  the initial strict matcher failed because the fixture guessed email rather
  than AF's actual opaque DEX subject. Subject-only test attribution now has
  RED/GREEN coverage; production authentication is unchanged.
- Collector DNS failure (exit 6), refused TCP (7), wrong CA/SAN (60), HTTP
  401/403/quoted bodies and invalid-duration subprocess contracts pass with
  bounded JSON/status/stderr and credential exclusion.
- A deterministic external-clock RED exposed the collector's same-second
  upper-bound exclusion. The collector now freezes a portable UTC ceiling;
  the original observation/gate budgets and DS query semantics are unchanged.
- Corrected deployed focused journey passed with race/four processes and seed
  `1791470268`: fresh UUID `16e2dcb9-6292-4a5c-8c1b-6597ee032190` appeared in
  both external-image output and its packaged archive. The spec checked every
  member for the actual short-lived collector token without logging it.
- The real parallel-worker failure callback archived earlier persisted UUID
  `7b3c843c-d13d-48d2-a0e6-6cf10f073b52`; this proves handoff/transport delivery,
  not a pass for that run's incorrect email attribution. It finished 210/211.
- Current external image `impl-pr2493-green-v4` includes the UTC ceiling and
  bounded newest-Fleet-snapshot collector, with source/image SHA-256 provenance.
- Full corrected local AF lane passed **216/216**, zero failed/pending, with
  race detection, four processes and fresh seed `1791681825` (886.6 seconds).
  Fifteen specs belong to excluded Fleet/helper labels, not deferred tests.
  Fresh UUID `3b2063ce-03c6-4e84-a826-bb5c8c6bbade` and its actual persisted
  correlation `4a2d635f-5294-4d94-89cb-ad19f64658b4` were independently verified
  in the preserved archive; SHA-256 is
  `e2096b557ed4c039fcf4e6dd7cf5445759b29ff756a5cb0c0348175380e2bae6`.
- Existing failure callback and get-only transport wiring pass; production
  authentication, audit ordering and correlation propagation are unchanged.
  Historical #2501 still lacks its original target UUID/query/persisted row,
  so these reproducible observation defects do not establish its exact RCA.
