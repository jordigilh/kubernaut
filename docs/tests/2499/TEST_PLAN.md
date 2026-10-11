# TP-2499-v1.0 — owned shutdown fixtures

**Status:** Approved 2026-10-10; local implementation verification complete;
batched PR CI pending. The
approval supersedes the draft banner in the published pre-implementation plan.
**Issue:** [#2499](https://github.com/jordigilh/kubernaut/issues/2499)
**Approved full plan:** [IEEE 829 hybrid plan](https://github.com/jordigilh/kubernaut/issues/2499#issuecomment-6101624395)
**Delivery:** isolated follow-up commit in PR #2493; batch remote updates.

## Purpose, requirements and scope

BR-STORAGE-028 and BR-AUDIT-005: independent shutdown tests must not share pending
notifications or persistence state. This is fixture isolation, not a PostgreSQL
outage repair or production consumer-ownership/replay redesign. Use unchanged
`ServerDeps`, `NewServer`, `DrainWithTimeout` and `Shutdown`.

## Approach and scenarios

Ginkgo/Gomega with real PostgreSQL, all Goose migrations, current partitions and
private Redis endpoints. No fabricated UT for fixture-only business behavior.

| Scenario | Contract |
|---|---|
| IT-DS-2499-001 | Before any drain, B cannot observe A's three pending messages. |
| IT-DS-2499-002 | Barrier-started real drains persist exactly A's three and B's five IDs, without cross-owner rows. |
| IT-DS-2499-003 | Empty B shutdown leaves A's pending message untouched. |
| IT-DS-2499-004 | Cleanup and partial provisioning failure, including a launcher error after Redis starts, retire only owned clients/database/container; other fixtures and suite resources survive. |

Also rerun the original timing, readiness, shutdown and DS unit regressions.
Independent verification clients outlive server shutdown; do not query a shared
database or pre-drain foreign leftovers to make an empty test pass.

## Wiring manifest

| Existing path | Fixture wiring | Proof |
|---|---|---|
| Shutdown + shared-port readiness specs | `createTestServerWithAccess` → `createTestServerWithBackends` → real `NewServer` | IT-001/003 and original specs |
| Real drain and persistence | Owned migrated DB/private Redis through existing constructors | IT-002 |
| Scoped cleanup | Ginkgo `DeferCleanup`, including failed provisioning | IT-004 |

## TDD, environment, risks and verification

- RED: IT-001/003 fail on the old shared fixture, deterministically before concurrency.
- GREEN: provision private migrated databases and random-port Redis; wire every helper caller.
- REFACTOR: remove duplicate shared verification connections and foreign cleanup drains;
  centralize owned cleanup and retain explicit errors. No production logic change.
- Environment: clean #2493 worktree, Podman, Go 1.26.6, four processes and `--race`.
- Verify: historical seed `1791469938`, fresh seeds, DS units, build/lint/compile checks.
- Fail/suspend: any foreign record/cleanup, missing migration, disabled spec or scope expansion.
- Rollback: revert only this follow-up; never discard unrelated #2496 work.

## Execution record

RED IT-001/003 reproduced shared queue observation and foreign consumption.
GREEN: 177/177 full DS integration specs pass with four processes and `--race`
for seeds `1791469938`, `1791674101`, `1791674107`; DS unit regression passes.
Final cleanup RED starts real Redis then injects launcher exit 17; it exposed an
orphaned owned container. GREEN records the unique name before launch and removes
only that attempted container (`--ignore` also handles a never-created name).
The final full suite passes **178/178**, zero failed/pending/skipped, with race
detection and four processes at historical seed `1791469938` and fresh seed
`1791683399`. Survivor fixture and suite PostgreSQL/Redis checks also pass.
Artifact logs are retained outside the source checkout in the PR #2493 RCA evidence
directory. Build/lint/final readiness are recorded with the batch verification,
not inferred from focused passes. No new audit event or compliance control mapping.
