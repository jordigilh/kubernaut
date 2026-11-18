# DD-FLEET-016: Staged FMC repair with atomic commit swap

Status: user approved direction A2, 2026-11-17, PR #2637 / issue #2593.
Amends: DD-FLEET-013 (A1/B1/C1 proof lifecycle). DD-FLEET-015 watch/enum scope
and DD-FLEET-012 singleton coordination are unchanged.
Requirements: BR-SCOPE-001, BR-INTEGRATION-065, BR-SAFETY-001.

## Problem (live-evidenced)

The current repair mutates partition state in place. `begin` marks the status
`repairing` and bumps the generation, `repair_reconcile` overwrites live
records, and `commit` flips the state back to `clean`. Because fenced reads
require proven-clean state, every read during a repair returns unavailable.

Live measurement on `af-fleet-2593-rca` (2026-11-17, diagnostic
`rotation-diagnostic/`): with the deployed `sync.interval: 10s`, every repair
cycle blocks reads for 2.1-4.2s — roughly 30% total read unavailability,
continuously. Rotation windows are worst (12.8s observed). One compound event
(token expiry during a repair window) blocked reads for 2min09s. AF's approved
scope-client retry budget (3 attempts, 2s total) cannot absorb any of these
windows, so approximately one in three journey admissions fails terminally.

This is not an implementation defect: `IT-AF-FMC-2593-001` asserts the
repair-window refusal as a contract. The design itself specifies the outage.

## Decision (A2)

Repairs write to a **staged generation**, and a single atomic Lua
`commit_swap` installs the staged state over the live keyspace:

1. `begin` (staged): writes a staged status key (`:status:staged`) carrying
   the new generation, repair token, and full fence, with a TTL. It does NOT
   touch the committed status. In-flight detection uses only the staged key's
   existence — TTL-bounded, so an abandoned repair self-clears and can never
   strand a "repair in flight" flag on the committed view.
2. `repair_reconcile` / member cleanup / empty finalization: operate on staged
   records (`:record:staged:<id>`) and a staged index (`:index:staged`). The
   committed view is untouched and remains readable throughout.
3. `commit_swap` (one Lua invocation): validates the staged status's full
   fence, token and generation, validates the staged index cardinality
   (ZCARD) against the expected observation count, then atomically renames
   every staged record into its live key, renames the staged index over the
   live index, deletes records removed from the new observation set, and
   overwrites the committed status with the new clean generation. Redis
   executes the whole script atomically; readers observe either the old or
   the new committed proof, never a partial state.
4. Fenced reads are unchanged in predicate: they still require a committed
   clean status, full fence, registration revision and TTL proofs. An
   in-flight repair is invisible to readers. When no committed proof exists
   (fresh partition, first repair, fence change), reads still fail closed —
   the `IT-AF-FMC-2593-001` refusal contract is preserved for that case.
5. Readiness publication changes meaning only in availability: a partition
   with a committed clean proof stays `ServiceReady` while its next repair is
   staged. `BeginPartitionRepair` no longer clears clean eligibility of the
   *previous* committed proof; `PublishPartitionClean` after `commit_swap`
   installs the new proof.

## Alternatives considered

- **Reduce repair cadence** (skip repair while TTL fresh): keeps outages,
  grows staleness; rejected as a partial remedy.
- **Dual-generation read acceptance** (read either committed or staged
  generation): staged state is mid-mutation and unproven; accepting it
  weakens the proof chain. Rejected.
- **Copy-on-commit without renames**: same atomicity requirement with more
  data movement than RENAME. Rejected in favor of renames.
- **Status quo plus client retry expansion**: weakens the approved AF retry
  budget (3 attempts / 2s total) without fixing the availability defect.
  Rejected.

## Spike evidence (mechanism validated 2026-11-17, miniredis)

`../2593-a2-spike-staging_spike_test.go` proves: committed proof readable
during staged repair; staged records invisible pre-commit; atomic swap serves
the new proof; abandoned repairs self-clean via staged-key TTL with the
committed proof surviving; 300-record bulk rename in one atomic invocation;
concurrent-begin rejection. Spike findings folded into this decision:
in-flight flag must be staged-key existence (F1), count validation via staged
index ZCARD (F2), new ops must route through the existing no-retry EVALSHA
wrapper (F3), commit carries staged expiry forward (F4).

## Boundaries

No HA, Lease/POD_UID/RBAC, public v1 wire, parser, or retry-budget changes.
The writer fence, registration revision fencing (B1), empty-partition proof
(C1) and atomic-field ownership (A1) contracts are preserved; only the
in-place mutation of the committed view during repair is replaced by staging.
Production qualification of the amended candidate requires a new freeze, the
three approved images rebuilt, and the four amendment journeys plus the full
AF Fleet suite re-run on the live cluster.

## Verification

TDD per methodology: RED tests for (a) reads serve last-committed proof during
repair, (b) no committed proof + repair in flight = still unavailable, (c)
commit swap atomicity and rollback-on-validation-failure, (d) abandoned staged
repair self-cleanup, (e) empty-partition staged finalization, (f) count and
fence validation at swap. Existing critical contracts (26/26) must remain
green; `IT-AF-FMC-2593-001` must still pass in its narrowed scope (no
committed proof).
