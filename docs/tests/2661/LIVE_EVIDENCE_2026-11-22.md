# Live evidence: E2E-FMC-INV-2661-001 (ACM lane, TP-2661 §8 Tier 3)

**Date**: 2026-11-22
**Run host**: `helios08` (root; the suite executes on-host, loopback FMC/Console
listeners, remote ACM Search and Valkey as recorded below)
**Suite**: `test/e2e/fleetmetadatacache/live` (`E2E-FMC-INV-2661-001`, 4 Its),
invoked as:

```sh
go test -v -count=1 -timeout=15m ./test/e2e/fleetmetadatacache/live/...
```

with `LIVE_ACM_SEARCH_ENDPOINT=https://search-spike-2592-open-cluster-management.apps.kubernaut492-sno-b.redhat-internal.com`
(route root; the production adapter appends `/searchapi/graphql` itself,
`pkg/fleet/fmc/acm_search.go`), `LIVE_ACM_TOKEN_PATH`/`LIVE_ACM_CA_FILE`/
`LIVE_VALKEY_ADDR=127.0.0.1:6390`/`LIVE_ACM_KUBECONFIG` as recorded below.

## Version identification (TP-2661 §6)

| Component | Recorded value |
|---|---|
| ACM / MultiClusterHub | 2.17.4 (`Running`, "All hub components ready"), hub `kubernaut492-sno-b` |
| Search API image | `registry.redhat.io/rhacm2/acm-search-v2-api-rhel9@sha256:85c53e64f719b91a91c1eaed06111e0c9eefe27d7f62e4114568bfe5b141a3ac` (pod `search-api-794696cb48-n6rpl`, verified via `oc get pod -o jsonpath='{.spec.containers[0].image}'`) — exactly the digest recorded in SPIKE_2026-11-18.md |
| Search route | `search-spike-2592` (reencrypt, service `search-search-api`), edge cert issuer `CN=ingress-operator@1791331040`; CA captured from `secret/router-ca -n openshift-ingress-operator` |
| FMC | in-process production composition from branch `work/2661-e2e-live-row` @ `4c69d7ad5` (`fmc.NewACMHTTPSearch`/`NewACMSource`/`NewSyncer`/`NewInventoryReader`/`NewHandlerWithInventory` + `scopecache.NewClientWithStatusStore` — the exact exported calls `cmd/fleetmetadatacache/main.go`'s `buildACMSource`/`buildFMCSyncer`/`wireFMCDependencies` make); no Helm chart is involved in this lane, so no chart version applies |
| Valkey | native `valkey-server` on helios08, `Valkey server v=8.1.10` (`redis_version:7.2.4`), port 6390, no persistence |
| Console (AF) | in-process production dispatch from the same commit: `fleet.NewInventoryReader` over `fleet.fmc.endpoint`, `policyread.NewService`, `handler.NewConsolePolicyReadHandler` |

## Principal (AC-6)

`ServiceAccount open-cluster-management/dd2661-live` bound via
`ClusterRoleBinding dd2661-live-view` to the cluster-wide `view` ClusterRole —
the same least-privilege shape the spike's `dd2661-probe` principal used
(SPIKE_2026-11-18.md "Principal"). A 24 h token was minted with
`oc create token dd2661-live -n open-cluster-management --duration=24h` and
stored mode 0600 for the collector; the SA and binding remain on the hub for
reruns (read-only, no credentials after token expiry). Search visibility for
this principal was established by the run itself (non-zero collected records).

## Fixture

Namespace `dd2661-live-inv-<unixnano>` labeled `kubernaut.ai/managed: "true"`
containing an unlabeled, replicas-0 `apps/v1 Deployment`
(`dd2661-live-inv-deploy`) — the namespace-inheritance half of ADR-053 and a
group-ful kind, so the separate `apigroup`/`apiversion` wire shape is exercised
end to end. Deleted by the suite's cleanup.

## Verbatim run output (the one proving live run)

```
=== RUN   TestACMInventoryLiveE2E
Running Suite: FMC ACM live inventory E2E Suite (Issue #2661, helios08) - /root/kubernaut-2661-live/repo/test/e2e/fleetmetadatacache/live
=========================================================================================================================================
Random Seed: 1795396757

Will run 4 of 4 specs
------------------------------
• • • •

Ran 4 of 4 Specs in 7.780 seconds
SUCCESS! -- 4 Passed | 0 Failed | 0 Pending | 0 Skipped
--- PASS: TestACMInventoryLiveE2E (7.78s)
PASS
ok  	github.com/jordigilh/kubernaut/test/e2e/fleetmetadatacache/live	7.842s
```

(Full log with timeline retained at `helios08:/root/kubernaut-2661-live/live-run.log`.)

What the four green Its prove, per the plan's Tier 3 row: FMC collected from
live ACM Search under the least-privilege principal and committed through the
fenced repair; Console listed the managed fixture with `freshness=fresh`
derived from proof; paged via cursor; read one exact resource by opaque ID
(re-verified through the FMC scope-check route); rejected a malformed cursor
with 400 and an unregistered cluster with 503 state-only `unavailable`;
served only the sanitized metadata-only payload; and the local-backend probe
recorded zero calls (AC-4: no fallback).

## Findings recorded during qualification (no production-code changes required)

Three composition facts surfaced while standing the live lane up; each is a
property of the harness, not a defect in the merged phases:

1. `fleet.NewInventoryReader`'s transport is an auth token source — outside a
   pod the default projected-volume path does not exist and every request
   fails closed. The live spec therefore sets `TokenPath` (the production
   knob) to a locally minted file.
2. Live Search indexes asynchronously (spike: 0.4-6.5 s settle lag), so the
   spec re-runs the real collection cycle per journey — exactly what the
   production sync loop does every `SyncInterval`.
3. A remote exact read re-verifies managed scope through the FMC scope-check
   route (`policyread.authorizeInventoryTarget`), so the live composition
   wires `scopecache.NewClientWithStatusStore` on the FMC side and the
   `fmc.HTTPClient` scope checker on the AF side — the same two-key
   production shape (`fleet.endpoint` for scope, `fleet.fmc.endpoint` for
   inventory).

## Boundary

One live run, one hub, one recorded Search build — no claim about other ACM
2.17.x patch levels, no per-user remote visibility (#2600), no Console
rendering evidence, no certification. E2E-FMC-INV-2661-002 is shipped as shared scenario `-017` in both FMC E2E lanes (CI-runnable) and
was run focused on the Kuadrant lane (branch `work/2661-e2e-live-row`, helios08):

```
Ran 4 of 20 Specs in 821.607 seconds
SUCCESS! -- 4 Passed | 0 Failed | 0 Pending | 16 Skipped
```

(log: `helios08:/root/kubernaut-2661-live/kuadrant-run2.log`; the 16 skipped are
the suite's other scenarios excluded by `GINKGO_FOCUS`).
