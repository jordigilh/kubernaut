# TP-2500-v1.0 — schema-valid MCP denial audit

**Status:** Approved 2026-10-10; local implementation verification complete;
batched PR CI pending.
**Issue:** [#2500](https://github.com/jordigilh/kubernaut/issues/2500)
**Approved full plan:** [IEEE 829 hybrid plan](https://github.com/jordigilh/kubernaut/issues/2500#issuecomment-6101625657)
**Delivery:** focused commit in PR #2493, batched with the other follow-ups.

## Purpose, requirements, scope and controls

BR-AUDIT-001/005, AU-2/AU-3/SI-10 and AC-6: real MCP authorization denial must
remain fail-closed while its audit event persists with truthful protocol metadata.
The centralized MCP denial emitter supplies `endpoint=mcp`. The shared typed
builder preserves explicit A2A/unknown protocols and known user-role metadata;
unknown policy roles serialize as `required_roles: []`, never fabricated names.
No schema weakening, auth bypass, correlation propagation or role redesign.

## Scenarios and pass criteria

| IDs | Contract |
|---|---|
| UT-AF-2500-001/002/004 | Complete generated payload validation and serialization; explicit MCP/A2A, correct tool/role/discriminator and non-null required array. |
| UT-AF-2500-003 | Empty/unsupported protocol stays invalid; no MCP fallback. |
| IT-AF-2500-001 | Actual authenticated MCP initialization + denied tool call → adapter/buffer/real DS → persisted actor/tool/UUID/action/outcome/protocol. |
| IT-AF-2500-002 | Console gate denies even before per-tool permission; no protected Kubernetes read. |
| IT-AF-2500-003 | Actual A2A agent/LLM dispatch retains A2A attribution and fail-closed behavior. |
| IT-AF-2500-004 | External SAR failure remains denied and audited. |
| IT-AF-2500-005 | External audit-backend failure is observable, exposes no JWT and does not execute the tool. |

Query the **actual persisted** correlation to recover the denial UUID. Do not use
a guessed MCP session/request ID. Mocks are external HTTP/LLM/Kubernetes failures
only; adapter, buffer, guard and persistence/query logic are real.

## Wiring manifest

| Existing production entry | Code | Proof |
|---|---|---|
| `cmd/apifrontend` MCP handler/bridge | `denyRBAC` in `mcp_bridge.go` | IT-001/002/004/005 |
| `cmd/apifrontend` audit wiring | `StoreAdapter` → `BufferedAuditStore` → DS | IT-001/003 plus actual correlation query |

## TDD, risks, environment and rollback

- RED: generated validator rejects nil array; real MCP path produces no persisted denial.
- GREEN: two bounded existing-code changes, no permission or data-contract expansion.
- REFACTOR: N/A for the production fix; no residual algorithm or duplicated logic.
- Regression: existing MCP/A2A, adapter and shared-audit suites plus real DS wiring.
- Environment: owned Helios08 account/netns, separate implementation source, real
  migrated DS, isolated envtest, four processes, `--race`, seed `1791470268`.
- Suspension: undefined infrastructure, weaker auth/schema, leaked token or guessed persistence.
- Rollback only this follow-up; retain unrelated working changes and failure artifacts.

## Evidence boundaries

Historical CI proves empty endpoint rejection (19 records), not a second logged
nil-array cause. Nil-array rejection is source/test evidence. Real wiring RED
reproduced three missing MCP denial rows while A2A and external-failure behavior
were unchanged. Final regression and batch readiness results are separate; this
single denied action does not prove an entire remediation lifecycle. CC8.1 maps
to approved/TDD/reviewed change management, not these audit payload contents.

## Execution record

- RED: generated validator rejection and absent rows through real MCP dispatch
  were reproduced before the two production changes.
- GREEN: complete AF integration **187/187**, four processes, `--race`, seed
  `1791470268`; this includes MCP/A2A/SAR/backend-failure denial wiring and actual
  persisted-correlation queries. The focused new wiring run passed 7/7.
- Unit coverage: both changed production functions, `denyRBAC` and
  `buildAuthAccessDeniedPayload`, have **100%** statement coverage. This is not a
  claim that the entire handler package has 100% coverage.
- Final `make test`: **114 successful suites / 11,281 passed specs**, zero
  failures/pending and two existing filtered specs. Build, all-suite compilation
  and global diff lint pass (**0 new issues**). An exact PR-head baseline reproduces
  the same 22 full-lint findings. External collector/Fleet verification is separate.
- Broader methodology checks remain failing: 37 non-BDD files and 15 heuristic
  orphan types are unchanged from the baseline. The static/null assertion checker
  reports 2,063 findings versus 2,047 baseline; added assertions were reviewed as
  schema/control contracts supplementing real dispatch/persistence/SAR checks,
  not null-only or mocked-business tests. This is not an automated-check pass.
- Both changed production functions (`denyRBAC` and
  `buildAuthAccessDeniedPayload`) have **100% unit statement coverage** in the
  final unit profile. Available AF unit/deployed profiles deduplicate to 87.5%
  (89.8% observed unit-testable coverage); no integration profile is included
  and no complete-service 100% coverage or blanket GA-readiness claim is made.
