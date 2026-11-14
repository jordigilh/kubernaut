# #2593 Review Evidence

This directory holds review artifacts for the FMC presence/TTL recovery plan.

## Rules

- A review is non-gating unless it records the complete response, session/message IDs,
  exact external-plan and `TEST_PLAN.md` SHA-256 values, checkout `HEAD`, `origin/main`,
  Engram/source references, and a finding-disposition diff.
- The former three independent reviews with no actionable P0–P2 findings were the
  pre-RED approval gate for the dirty-worktree plan. They remain historical evidence
  and are not repeated for the user-approved clean-worktree fast path.
- The active fast-path boundary is recorded in `../FAST_PATH_CARRY_HOLD.md` and the
  bounded OpenChamber plan. Expected implementation changes are allowed only in the
  clean worktree; each RED/GREEN/REFACTOR pass still requires its own immutable
  source-diff capture and review.
- Historical R1–R3 responses remain discoverable through the external plan ledger's
  OpenChamber session/message IDs; they are not clean approvals.

## Historical archived round

- `R8.md`: three independent reviews of plan SHA-256
  `6eadc81b2abac06ddebd9e49e28e3621e5dd468fd743767ec473be1c49193793` and test-plan
  SHA-256 `f9454a9f790ec6d606ba38295804c28a06f056e5d67ebf146bce88eb7f6f5123`.
- Checkout: `7a4ed304580298802520e292cad0a7c4efccd17b`.
- `origin/main`: `18870e92d58b83c1c061841130bc73fde62abc70`.
- Result: A reported 6 P0 / 4 P1, B reported 1 P1 / 2 P2, C reported 1 P1 / 2 P2;
  non-clean; counter reset.

- `R9.md`: three independent reviews of plan SHA-256
  `c61539745ef0ed157a6ef101cc3dd452ec9ab165bc1a7544f2a5d209f568189f` and test-plan
  SHA-256 `fea2e450c51e99bb81c32f2ee2e8fcc7441a8ee8718b0e8b3f0871982f691da2`.
- Checkout: `7a4ed304580298802520e292cad0a7c4efccd17b`.
- `origin/main`: `18870e92d58b83c1c061841130bc73fde62abc70`.
- Result: consolidated 6 revalidated P0 / 8 open P1 / 0 P2; non-clean; counter reset.

- `R10.md`: three independent reviews of plan SHA-256
  `69dcba1e2cc94f14d5b7f29f0fdc8170c99062c33c26a33e281a09cc4e2f0f6a` and test-plan
  SHA-256 `75940498c6dedbacb0df9d105de6f9a63d5fe0e89b8c4f4b8fa5ec98797ecbe7`.
- Checkout: `7a4ed304580298802520e292cad0a7c4efccd17b`.
- `origin/main`: `18870e92d58b83c1c061841130bc73fde62abc70`.
- Result: consolidated 6 revalidated P0 / 8 open P1 / 0 P2; non-clean; four protocol
  contracts amended; counter reset.

- `R11.md`: three independent reviews of plan SHA-256
  `72aaa835a5d7caafb4d79a79baaee9ed559972107f86f3e814fd371e1a4e3a09` and test-plan
  SHA-256 `0df0fc6c333f833dde4501e601f7d4def1f731056ca904660d7925db6c7fda4a`.
- Checkout: `7a4ed304580298802520e292cad0a7c4efccd17b`.
- `origin/main`: `18870e92d58b83c1c061841130bc73fde62abc70`.
- Result: non-clean; 6 revalidated P0, five distinct plan/qualification P1 themes and
  one P2 were recorded; counter reset.

- `R12.md`: three independent reviews of plan SHA-256
  `0d5ff2e25da1892905a62f8ef8f04bc709eeb0d345369f1902e802c18a0a5d39` and test-plan
  SHA-256 `942cca42928ab04b9ec5592d7f600e29dfb800e2b831df7f129e6d5a2a6c11f3`.
- Checkout: `7a4ed304580298802520e292cad0a7c4efccd17b`.
- `origin/main`: `18870e92d58b83c1c061841130bc73fde62abc70`.
- Result: non-clean; 7 P0 including invalid-label fail-open, canonical-ID/revision
  plan gaps, and 1 denominator P2; counter reset.

- `R13.md`: three independent reviews of the unchanged v0.7 plan SHA-256
  `a1afccefbe863130da9ccba9c75e72c0cde9aae621f7ba8e57fb14399de4c1b1` and test-plan
  SHA-256 `e4bb9ff3ebc300aeafe6c66e53b699373857a98cff0cf6fddd7d4abbee53b50b`.
- Checkout: `7a4ed304580298802520e292cad0a7c4efccd17b`.
- `origin/main`: `18870e92d58b83c1c061841130bc73fde62abc70`.
- Result: non-clean; no fresh v0.7 plan findings; 7 revalidated P0 and 5 revalidated
  P1; counter remains zero.

`R4.md`, `R5.md`, `R6.md`, and `R7.md` remain historical and non-gating. R8–R13 were
non-clean, so the current counter is still zero and none of R4–R13 can be counted toward
the next clean gate.

## Historical amended baseline / reset metadata

**Amendment/reset event:** 2026-11-13 — `TP-2593-v0.9` and the bounded OpenChamber plan
were amended under the user-approved pre-GA v1/no-migration public contract and the
user-approved internal `PartitionStateKey`/tokenized repair/epoch-scoped LeaseSession
contract. This correction also resolves RED/Step 0 sequencing, replaces the circular capture
recipe with two-phase utility-bound validation, and fixes neutral observation-reader package
ownership. It additionally makes same-UID `RegistrationRevision` invalidation and current-
revision read proof explicit, and requires canonical `approved-inputs.json.sha256` validation
before Phase B accepts approved inputs. The authority update was documentation-only
before the later clean-worktree fast-path boundary; it authorizes no deployment,
source completion, or review approval.

This amended pair invalidated prior clean-review eligibility for the former dirty-worktree
approval gate. No clean review or approval is claimed here, and the historical counter
remains `0/3`; the user-approved clean-worktree fast path is recorded separately and does
not create a second three-review cycle.

- Bounded/OpenChamber plan: knowledge plan ID
  `44585f5b-5673-4cd9-ad59-7b0ba98dff66`, registered path
  `/Users/jgil/.config/openchamber/projects/path_L1VzZXJzL2pnaWwvZ28vc3JjL2dpdGh1Yi5jb20vam9yZGlnaWxoL2t1YmVybmF1dA/plans/1794533840933-fmc-bounded-implementation-and-review.md`,
  amended fast-path SHA-256 `c3d2df305a5afe4ed2ad03bf1c4b7940eff318b705aff4cd6cea944b9f556884`.
- Test plan: `TP-2593-v0.9`, fast-path SHA-256
  `d603a0a53bbfb97b83dc82bac95b1dd8f127627e890ad1dc2b908887fc350015`.
- Source checkout `HEAD`: `7a4ed304580298802520e292cad0a7c4efccd17b`.
- Source checkout `origin/main`: `18870e92d58b83c1c061841130bc73fde62abc70`.
- Test-plan identity: `TP-2593-v0.9`, Fast-path implementation baseline — Step 0 complete; RED pending.
- Reset reason: user-approved internal repair/Lease contract supersedes the old internal
  StatusStore/Lease/read/repair signatures without a compatibility shim; `BeginRepair`
  precedes MCP collection; `CommitRepair`/proof-bearing `ReadVerified` and the exact
  proof-returning readiness handoff are authoritative; P1-1 now makes Step 0/three reviews a
  pre-RED plan-approval baseline and moves v1 route/media consumer conversion into
  RED/GREEN/REFACTOR; P1-2 defines the non-circular two-phase capture with immutable utility
  binding and post-approval per-pass source captures; P1-3 assigns neutral observation types
  and `ObservedReader` to `pkg/fleet/observation`, concrete MCP transport to
  `pkg/fleet/mcpclient`, and the factory/Syncer adapter to `pkg/fleet/fmc`; active denominators
  remain 12 UT, 4 IT, 16 overall, 15 critical, and 19 wiring; same-UID endpoint/metadata
  changes increment `RegistrationRevision`, invalidate clean/readiness state, and require a
  new generation repair plus current-revision read proof; Phase B requires the exact
  `approved-inputs.json.sha256` sidecar and records its digest/hash outside the stable digest;
  `IT-FMC-MIGRATION-017` remains N/A/out-of-scope with zero credit. Follow-up documentation
  corrections bind `verify-ledger` to the fixed capture utility instead of `shasum -c`, remove
  the legacy `CacheReader` from the production constructor contract, use `scope.LabelState`
  for observed labels, distinguish complete scans from verified-empty scans, clarify that
  expected post-approval source captures do not reset the review counter, and align the TP's
  internal-contract statuses with the bounded plan.
- Step 0 review meaning: the current unreleased v2 source/test inventory is held context,
  not accepted behavior or cleanup; expected source changes after the clean boundary are
  reviewed in immutable RED/GREEN/REFACTOR pass captures and do not reset the historical
  `0/3` counter.
- Hash command: `shasum -a 256 <file>` on 2026-11-13.

Prior reviews and R13 remain immutable historical evidence and are not edited. The
pre-amendment **`0/3`** clean-review state remains historical/non-clean; no clean review
or approval carries forward. The fast path authorizes only bounded source work in the
clean worktree. Cluster, issue, ACM inventory, Valkey qualification, deployment, and
release work remain unauthorized. The historical clean-review counter is **`0/3`**.

## Active clean-worktree boundary

- Implementation worktree: `/private/var/folders/r7/gktmmltd1zq7wqhsjjslwsm80000gn/T/opencode/2593-fmc-implementation`.
- Branch and baseline: `work/2593-fmc-presence` from `7a4ed304580298802520e292cad0a7c4efccd17b`.
- Original draft worktree: `/Users/jgil/.local/share/opencode/worktree/d1eaa5e864fe7029869e1624207d8e393b28d5ca/v16-acm-inventory`; preserved at `72` modified tracked and `23` standard untracked paths.
- Carry/hold evidence: `docs/tests/2593/evidence/FAST_PATH_CARRY_HOLD.md`.
- Boundary result: complete; RED is the next phase. No public v2 route/media, ACM/AF inventory, RO/audit/security-event, generated external-gate, or ambiguous held work was carried.
