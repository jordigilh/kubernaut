# DD-FLEET-015: Owned registry sessions and enum-only scope

Status: user approved “Approve amended implementation (Recommended)”, without
conditions, 2026-11-16; PR #2637 / issue #2593.
Requirements: BR-SCOPE-001/002/010, BR-INTEGRATION-065, BR-SECURITY-016/017.

## Decisions and alternatives

1. **Owned LIST → WATCH supervisor (selected).** Both gateway registries use
   the existing dynamic client, extraction and publication helpers. A private,
   complete paginated LIST with a nonempty resourceVersion precedes a matching
   WATCH. Readiness requires installation of that snapshot and attachment of
   consumption to the successfully opened current-epoch stream. Quiet and empty
   registries need no event/bookmark. Fresh informers per recovery epoch were
   rejected because they introduce additional asynchronous publication stages;
   LIST-only recovery was rejected because it cannot prove watch establishment.
2. **Enum-only existing v1 (selected).** Reuse ScopeState/ScopeResult, replacing
   ScopeChecker with Check. Delete the boolean compatibility seam rather than
   inferring observation provenance from false or an empty response. Retaining a
   strict boolean adapter was rejected because it cannot express the required
   evidence. A v2 endpoint was rejected: this is an atomic pre-GA replacement.

## Session contract

The owner runs at most one WATCH. LIST and WATCH establishment each have a
30-second budget; established streams rotate after five minutes by local
cancellation. Failure retires the epoch and invalidates readiness under the
publication lock before any replacement can publish. Global mutation generation
fences LIST publication, while unchanged registrations retain DD-FLEET-013
per-registration revisions. Metadata/UID changes advance those revisions.
Rejected/incomplete/expired pagination cannot publish. Valid pending events retain
order; already-observed close/error cannot publish. Asynchronous decoding cannot
promise detection of every remote close before publication.

Failure retry is cancellable exponential backoff from one second to 30 seconds,
with ±10% jitter. Start owns the supervisor and waits for initial success or
cancellation. Stop retires/cancels and joins the owner before closing events;
Stop is idempotent. WithCurrent rejects unhealthy/stopped state under its lock.
Readiness probers observe Ready only; no LIST-based health restoration remains.
Production FMC signals cancel before synchronous dependency construction, and
shutdown joins startup workers. Preserve bootstrap health while startup waits.

## Scope contract

ScopeResult serializes exactly state and provenance. Reason and Retryable are
internal, bounded diagnostics. States are verified_managed, verified_unmanaged,
unavailable, unsupported, invalid and unauthorized. Provenance is resource-label,
namespace-label, none or unavailable; it identifies the determining observation,
not backend completeness or cluster identity. Unknown/empty/invalid combinations
are protocol failures, never default unmanaged. Execution/transport/cancellation/
protocol failures return a nonauthorizing result and error; completed domain
outcomes return a validated result and nil error.

GET /api/v1/scope/check remains application/json. Verified outcomes use 200;
unavailable uses 503; invalid request identity/GVK uses 400; observed invalid
labels/unsupported targets use 422; typed backend denial uses 403. Middleware
401/403/500 and non-GET 405 retain their existing boundaries. No managed field,
compatibility shim, v2 route/media, public reason/timestamp or target UID claim.
The HTTP client requires a bounded single JSON object, exactly the two required
members, exact enums, valid provenance and status/state pairing. Duplicate/unknown
members, null, wrong types, trailing data and malformed bodies fail closed.
Only transient transport/unavailable retries: at most three attempts in two
seconds. Redis no-replay ownership remains unchanged.

Local target presence precedes namespace inheritance. Missing targets, unknown
kinds and missing namespace evidence remain unavailable. Explicit resource true/
false determines resource-label provenance. Present unlabeled targets may inherit
an observed namespace label, including explicit namespace false. Only observed
absent opt-in yields none. Cluster-scoped targets resolve at level 1 from their own
label (BR-SCOPE-001 FR-SCOPE-004 Option A, #2670); only the non-labelable
`ClusterScope` pseudo-target remains unsupported. Internal Namespace observations
continue to support namespaced inheritance.

FMC retains UID/revision/generation/full-fence/completeness/TTL proof. Unknown
registrations perform no cache read. Observation-backed reads are mandatory.
Direct ACM uses exact, unambiguous GVK/identity and actual Search label projections,
not boolean conversion; empty/incomplete/mismatched/missing-label evidence is
unavailable. Explicitly present empty label projection can prove label absence.
Invalid/duplicate labels cannot inherit. DD-AF-018 Console inventory stays closed.

Enabled Fleet without an endpoint is a configuration error. Empty Fleet attribution
is invalid. Explicit registered hub IDs are remote; non-Fleet uses local scope.
Only verified_managed allows GW RR creation or AF action. RO preserves its existing
block/requeue envelope with unavailable diagnostics, not opt-in advice. Only
verified_unmanaged maps AF exact inventory to existing NotFound; other failures
remain unavailable with no row or fallback.

## Scope and verification

> **Reading note (2026-11-18):** the "no RBAC/dependency changes" sentence below
> applies to **this DD's** enum/WATCH work. The later #2661 amendment does add a
> Console inventory route consumer, a namespace-list SAR on the AF read path and
> an FMC outbound Search principal; those are owned by DD-FLEET-017 v2, not by
> this DD.

> **Amendment (2026-11-18, #2661):** the Console **inventory** lane is now
> FMC-owned (ADR-068 peer → facade amendment; DD-FLEET-017 v2). "DD-AF-018
> Console inventory stays closed" therefore means: closed until FMC's ACM source,
> the configured hub Search identity, completeness and a production journey are
> qualified. This DD's scope contract, enum migration and owned WATCH work are
> unchanged, and the scope lane is not rerouted here.

No CRD/operator, HA, RBAC, dependency, Console list/policy-scope, audit-event or
producer-parser changes. Preserve DD-FLEET-011/012/013/014 and Redis ambiguous-write
regressions. AC-4/AC-6/SI-10 and existing SC-8 TLS boundaries apply; CC8.1 records
approved change management, not audit completeness. See TEST_PLAN for 26 critical
contracts and eleven required wiring rows. Final artifact E2E is CI-owned.
Rollback is an atomic reviewed revert, not a partial compatibility migration.
