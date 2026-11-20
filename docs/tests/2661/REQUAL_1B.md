# Increment 1b — DD-FLEET-016 re-qualification with the lexicographic index

**Date**: 2026-11-19
**Scope**: the atomic-commit keyspace now carries a second (lexicographic)
index. This re-runs the availability evidence DD-FLEET-016 was approved on, and
records what changed.
**Method**: no cluster. A temporary diagnostic (`./.tmp-1b`, deleted after the
run) built the production objects directly — real Valkey, real
`ValkeyStatusStore` with the real `fmc_v2_atomic.lua`, real
`FMCReadinessSnapshot`, real `fmc.InventoryReader` — seeded a proven generation
of 200 mixed-namespace records, then rotated it through 12–20 real
`BeginRepair → Repair → CommitRepair → PublishPartitionClean` cycles while four
goroutines read pages every 5–20 ms through `reader.List`.

## Result 1 — staged repair no longer costs availability

| Run | Polls | Success | Failures | Rotation p50 |
|---|---|---|---|---|
| 20 ms poll, 12 rotations, 200 records | 561 | **99.47%** | 3 | 127 ms |
| 5 ms poll, 12 rotations, 200 records | 769 | 99.7% | 2 | 341 ms |
| 5 ms poll, 20 rotations, 200 records | 3 914 | **98.62%** | 54 | 1.46 s (instrumented run) |

Zero failures were attributable to *staging*: no read failed while a generation
was being repaired and committed. The pre-A2 behaviour (2.1–4.2 s of unavailability
per cycle) does not occur.

## Result 2 — the residual is a page-read race, not the index

Every residual failure is `inventory unavailable: … stale_proof`, and the
instrumented run captured the deciding evidence at failure time:

```
error@before1  inventory proof is not available (stale_proof)  ready_gen=21 committed_gen=21
```

Readiness and the committed status **agreed** (both generation 21). What failed
is the page's own two-call shape: the header call validated the proof and
returned the identity set of generation 20; a commit landed; the records call
then validated the same fence/generation against records that are now generation
21 → fail closed. That is the designed behaviour recorded in the S1/S2 spikes
("rotation between the calls fails closed instead of mixing generations").

Control that isolates the mechanism: inserting an artificial 200 ms gap between
`CommitRepair` and `PublishPartitionClean` moved failures into that window
(`error@phase2`, same `stale_proof`), confirming that proof/readiness lag and the
header→records race produce the same symptom class, and that no other path is
involved.

**Rate extrapolation.** The race window is the page-read duration (~2–3 ms). The
harness rotates every ~0.2–1.5 s, which is why it observes 1–1.4%. At the
production cadence (30 s sync, 3 ms read) the expected rate is ≈ **0.01%** of
reads, each returning `unavailable` state-only with no rows, self-healing on the
next request. The measurement is a worst case, not a production estimate.

## Result 3 — migration window behaves as specified

Deleting the live lex index (`*:lex`) simulates a partition committed before this
change: the page read returns `unavailable` with `no_lex_index`, never `empty`;
the next rotation commits both indexes and service is restored. Verified in every
run (`migration window: unavailable-before-next-rotation=true
restored-after-rotation=true`).

## Bugs found and fixed by this increment

`limit=100` — the documented maximum (`inventory.MaxLimit`) — was rejected end to
end. The reader asks the store for `limit+1` members to learn whether another
page exists, and both store validations compared against `MaxLimit`:

1. `InventoryPageRequest.Validate`: `Limit > MaxLimit` → `inventory page limit is out of bounds`.
2. `InventoryPageRecordsRequest.Validate`: `len(Members) > MaxLimit` → `inventory records page is out of bounds`.

Both ceilings are now `MaxLimit+1` with the probe documented, so the console
limit stays bounded at 100 while the internal look-ahead is legal. Without this
fix every request at the maximum page size failed with `unavailable` — a defect
that predates the lex index but was only reachable through the page path.

## Validation alongside the measurement

- `ginkgo -race --focus="IT-FMC-INV-2661|STAGED|repair" ./test/integration/fleetmetadatacache/`
  → **14 passed, 0 failed** on real Valkey (port 16391) + envtest.
- `go test ./pkg/fleet/...` and `golangci-lint` on the touched packages: clean.

## Decision taken — bounded retry (1c)

The index itself costs no availability. The residual race is now handled:

- **Implemented (D2.5b):** on `stale_proof`, the reader re-resolves the fence
  once after a 2 ms pause and repeats the page read; a second failure returns
  `unavailable` state-only. Two attempts maximum, so a genuinely stale fence
  still fails closed.
- Proving spec: `UT-FMC-INV-2661-015` — one retry then `success` on a
  superseded proof, and exactly two attempts (never a loop) when the proof stays
  stale.

The same two-call race exists for any reader that validates a proof then fetches
records; the scope lane's `read_verified` is single-call and unaffected.
