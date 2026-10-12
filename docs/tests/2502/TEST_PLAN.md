# TP-2502-v1.0 — bounded Fleet Service-path failure evidence

**Status:** Approved 2026-10-10; local implementation verification complete;
batched PR CI pending.
**Issue:** [#2502](https://github.com/jordigilh/kubernaut/issues/2502)
**Approved full plan:** [IEEE 829 hybrid plan](https://github.com/jordigilh/kubernaut/issues/2502#issuecomment-6102903139)

## Purpose, requirements and exclusions

BR-FLEET-054/BR-INTEGRATION-065 and BR-PLATFORM-001: preserve the original node-side
IPv4 Service VIP TCP probe, two-second cadence and 60-second readiness gate while
retaining discriminating failure-time evidence. Direct backend success is
**diagnostic only**. No timeout increase, OIDC/RBAC/TLS alteration, host firewall,
kube-proxy mode, topology or networking repair. AC-6/SC-8 stay intact.

## Scenarios

| IDs | Contract |
|---|---|
| UT-FLEET-2502-001 | Exact validated address/probe; bounded streaming output and explicit truncation/invoke failure. |
| UT-FLEET-2502-002 | Healthy direct backend never substitutes for failed VIP. |
| IT-FLEET-2502-001 | Real loop with isolated external executor refusal/hang/invoke/success; useful command/time/stdout/stderr/context. |
| IT-FLEET-2502-002 | Caller cancellation/deadline remains primary even if secondary collection fails; bounded child exit. |
| IT-FLEET-2502-003 | Owned Kind node/path comparison captures Service, EndpointSlices, proxy/API status/logs/config and node routing/netfilter during failure. |
| IT-FLEET-2502-004 | Runner/collector includes only named lane snapshots; no raw kubeconfigs/tokens/unrelated cluster; bounded archive and credential exclusion. |
| E2E-FLEET-2502-001 | Fresh hub-only OIDC setup uses original gate; records timing and preserves evidence without historical closure claims. |

## Wiring manifest

| Existing path | Wiring | Proof |
|---|---|---|
| Fleet OIDC setup | `patchAPIServerForOIDCConfig` → `waitForInClusterAPIServer` | IT-001/002, E2E-001 |
| Failure-time snapshot | Actual owned-node/helper commands, explicit kubeconfig | IT-003 |
| Durable archive | AF failure callback → owned snapshot mount → `gather.sh`/`fleet-service-path.sh` | IT-004 |

## TDD, budgets and controls

RED reproduces lost refusal/context and missing diagnostic delivery. GREEN captures
first-failure snapshots concurrently with authoritative probes, bounded at ten
seconds; each command stores at most 8192 output bytes. Archive copies at most ten
128-KiB named regular log files, not credential/manifest files or symlinks.
REFACTOR centralizes bounded command execution and makes cancellation/primary
error preservation explicit. Diagnostics cannot turn a failed VIP into success.

## Environment, verification and rollback

Clean #2493 source, dedicated Helios08 account/netns, distinct owned cluster/node/
kubeconfig/cert/ports. Real Kind/image comparison plus four-process Fleet seed
`1791470285` and fresh seed, affected suites, build/lint/compile and readiness.
Any induced failure must stay inside an owned node; permission/environment changes
require approval. Never alter host/shared-node rules. Rollback only this follow-up.

Current executor TDD evidence establishes retention defects. The historical
failure lacks failure-time direct/VIP/watch/routing state; post-recovery success
does not establish its cause. No networking-fix or historical RCA closure claim.

## Execution record

- Fresh hub-only setup, race/four processes, seed `1791470285`, reproduced
  VIP refusal for the unchanged 60-second gate. The failure callback delivered
  a 16,551-byte named snapshot in the actual production-image archive, including
  the authoritative first refusal, Service/EndpointSlice state, proxy/API
  status/logs/config and owned-node routing/netfilter output.
- This run exposed a diagnostic defect: a deprecated Endpoints warning in the
  combined output prevented parsing the backend address. RED reproduces it;
  GREEN selects the already-required EndpointSlice address and proves direct
  backend success still cannot substitute for VIP readiness.
- The gate failure is not a passing Fleet E2E run. Subsequent readonly probes
  found both paths recovered; they are not failure-time evidence and do not
  close either this run's or the historical run's underlying networking RCA.
- Infrastructure regression before the final redaction-cap case: **268/268**
  with race, three repeated runs,
  after allowing the direct-comparison executor fixture to complete before its
  test-only deadline. A separate hung-snapshot spec still proves bounded
  cancellation without extending the production gate.
- Archive-cap RED proved oldest-first selection could displace the current
  failure. GREEN selects the ten newest named regular snapshots, caps each at
  128 KiB, and still rejects symlinks/credential files.
- Fresh seed `1791680693` captured a real first-attempt VIP refusal and successful
  direct backend comparison. The **original VIP probe** passed on attempt 2;
  direct success did not release the readiness gate. Later setup failed before
  specs because UID 1006 exhausted the host's 200-key quota, not disk space.
- The existing external runner and rebuilt production image delivered that
  24,817-byte failure-time comparison in an actual archive; all **232 members**
  were checked for the actual collector credential. This standalone verification
  is archive/wiring proof, not a Fleet E2E pass or historical networking RCA.
- User approved a temporary `kernel.keys.maxkeys` increase from 200 to 512 for
  sequential deployed verification, with restoration to 200 afterward. All other
  host limits and retained diagnostic nodes stay untouched.
- Full hub-only Fleet label selection passed **11/11**, zero failed/pending,
  with race detection, four processes and fresh seed `1791683311` (702.9 seconds).
  The other 220 specs belong to excluded labels, not deferred tests. This reused
  the explicitly owned cluster. The original Service VIP probe recovered on
  attempt 16, within its unchanged 60-second gate; direct-backend success remained
  diagnostic only. This is deployed regression GREEN, not historical RCA closure.
- Final archive RED found that redacting short markers could expand a capped
  input to 262,139 output bytes. GREEN bounds post-redaction output while still
  consuming the input pipe; no hidden SIGPIPE or credential-copy workaround.
- Repeated infrastructure coverage passes **269/269**, zero failed/pending/skipped,
  with race detection in three runs. A scheduling race in the cancellation
  fixture was removed by waiting for actual probe startup before canceling;
  production timing is unchanged and the independent 300-ms hung-snapshot
  deadline case remains intact.
- Rebuilt production image `impl-pr2493-green-v5` and the existing external
  runner delivered the latest 15,062-byte failure-time VIP/direct comparison
  together with the retained named snapshots. All **237 archive members** were
  checked for the actual short-lived collector credential. Archive SHA-256:
  `03ad405812c6f0574da6f89c7e13c063250192db43c517f877b84f3565913a65`.
- The approved temporary key-count limit was **restored to 200** at
  `2026-10-11T02:02:43Z`; `maxbytes` stayed 20,000 and no retained node was stopped,
  deleted or reconfigured. This host verification accommodation is not a
  repository setting or a proposed CI/networking fix.
