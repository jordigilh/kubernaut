# Fleet Readiness Gate - Production Runbooks

**Version**: v1.0
**Last Updated**: 2026-07-04
**Status**: ✅ Production Ready
**Applies to**: Gateway (GW), RemediationOrchestrator (RO), EffectivenessMonitor (EM),
SignalProcessing (SP), WorkflowExecution (WE), APIFrontend (AF), KubernautAgent (KA)
**Related**: [ADR-068: Fleet Federation Architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md#fleet-readiness-gate-fail-closed-pod-wide-readyz-1553)

---

## 📚 Runbook Index

| ID | Runbook | Triggers On | Automation |
|----|---------|-------------|------------|
| RB-FLEET-001 | [Pod stuck NotReady due to Fleet dependency](#rb-fleet-001-pod-stuck-notready-due-to-fleet-dependency) | `/readyz` returns 503, pod removed from Service endpoints | Alert + Dashboard |
| RB-FLEET-002 | [ACM backend permanently NotReady (#1556)](#rb-fleet-002-acm-backend-permanently-notready-1556) | `fleet.backend: "acm"` deployments never reach Ready | Alert |

---

## Background: what changed in #1553

Before #1553, a Fleet-dependent service (any of the 7 listed above) that lost connectivity to its
Fleet dependency (scope-checker backend, MCP Gateway, or OAuth2 provider) **after** a successful
startup kept reporting `/readyz=200` — it silently degraded (served stale/local-only data, or
silently treated remote resources as "unmanaged") instead of signaling unhealthy. Static
misconfiguration at startup was always fail-closed (`Validate()` + `os.Exit(1)`); only *runtime*
unreachability was fail-open.

As of #1553, runtime unreachability of a Fleet dependency is **also fail-closed**: a shared,
periodically-probed `pkg/fleet/readiness.Gate` flips the pod's `/readyz` to `NotReady` (503)
**pod-wide** — not just for the specific Fleet-routed request path — until connectivity recovers.
Kubernetes removes the pod from Service endpoints for the duration.

**This is an intentional trade-off**: favor detectability and operational simplicity (treat a pod
that can't reach its Fleet dependency as unhealthy as a whole) over partial availability (continuing
to serve non-Fleet traffic through the same pod while silently degrading Fleet-routed traffic). If
your deployment scales the affected service to N>1 replicas, only the replicas actually experiencing
the connectivity issue drop out of rotation — this is not necessarily a full outage of the service
unless *every* replica loses the same dependency (e.g., MCP Gateway itself is down).

---

## RB-FLEET-001: Pod stuck NotReady due to Fleet dependency

### Symptoms

- `kubectl get pods` shows one or more Fleet-dependent service pods as `0/1 Running` (readiness
  probe failing) or the pod is missing from `kubectl get endpoints <service>`.
- `/readyz` returns HTTP 503.
- No corresponding crash/restart — the pod is up and the process is healthy; only readiness fails.

### Diagnosis Steps

```bash
# Step 1: Confirm it's the Fleet readiness gate specifically (not another readyz check)
kubectl exec -n kubernaut-system deploy/<service> -- curl -sv localhost:<health-port>/readyz
# Look for a body/log line mentioning "fleet" in the failing check name

# Step 2: Check which Fleet dependency is unreachable — logs will show the failing prober
kubectl logs -n kubernaut-system deploy/<service> --since=5m | grep -i "fleet\|readiness\|prober"

# Step 3: Confirm the dependency's own health independently
#   - MCP Gateway:
kubectl exec -n kubernaut-system deploy/<service> -- curl -sk https://<mcp-gateway-endpoint>/healthz
#   - FMC scope-check backend:
kubectl exec -n kubernaut-system deploy/<service> -- curl -s http://<fmc-endpoint>/healthz
#   - ACM Search backend: see RB-FLEET-002 first — a permanently-unreachable ACM backend is a
#     known, separate issue (#1556), not a transient outage.

# Step 4: Check OAuth2 provider reachability if MCP Gateway auth is failing
kubectl exec -n kubernaut-system deploy/<service> -- curl -sk https://<oauth2-token-url>
```

### Resolution by Root Cause

| Root Cause | Resolution |
|------------|------------|
| MCP Gateway pod down/restarting | Check MCP Gateway (EAIGW/Kuadrant) deployment health; the readiness gate auto-recovers once the gateway is back — no action needed on the dependent service beyond waiting for the next probe tick (probe interval: see `pkg/fleet/readiness.Gate` ticker config, default probes are bounded to 10s via `DefaultProbeTimeout`) |
| OAuth2 IdP (Keycloak/Dex) unreachable | Check IdP health; `ResilientClient` retries with backoff, gate recovers automatically once token issuance succeeds again |
| Network policy / DNS change blocking egress to Fleet endpoint | Check `NetworkPolicy`/`egress` rules and DNS resolution from the pod's namespace |
| TLS CA rotation mismatch (`tlsCAFile`) | Confirm the mounted CA bundle matches what the Fleet endpoint is currently presenting; `sharedtls.CAReloader` picks up file changes without a restart, but a missing/stale mount requires redeploying the ConfigMap/Secret |
| Fleet endpoint itself is genuinely down for an extended period | This is by design: the pod stays `NotReady` (and out of the Service's endpoint list) until the dependency recovers. If this is unacceptable for your availability requirements, this is a deliberate architectural trade-off from #1553 (pod-wide fail-closed) — escalate to the team that owns this decision rather than working around it locally, since a per-request partial-degradation model was explicitly rejected in favor of detectability. |

### Verification

```bash
# Gate recovers automatically on next probe tick once dependency is reachable again
kubectl get pods -n kubernaut-system -w
# Confirm /readyz returns 200
kubectl exec -n kubernaut-system deploy/<service> -- curl -s -o /dev/null -w "%{http_code}\n" localhost:<health-port>/readyz
```

---

## RB-FLEET-002: ACM backend permanently NotReady (#1556)

### Symptoms

- Only affects deployments with `fleet.backend: "acm"` configured.
- Pod is permanently `NotReady`, and RB-FLEET-001's diagnosis steps show the ACM Search endpoint
  itself is reachable (e.g., `curl` to the endpoint from outside the pod succeeds, or returns 401
  rather than a connection error).

### Root Cause

`pkg/fleet/acm.Client` does not currently send the `Authorization: Bearer <token>` header that the
real ACM Search GraphQL API (`stolostron/search-v2-api`) mandatorily requires (confirmed against the
upstream project — every request, including the project's own dev-testing `curl` targets, requires
a bearer token validated via Kubernetes `TokenReview`-backed RBAC). This is a **pre-existing gap**
in the ACM adapter (it never worked, but previously failed silently — see below), not something
introduced by #1553. It's tracked separately as **[#1556](https://github.com/jordigilh/kubernaut/issues/1556)**.

Before #1553: `acm.Client.IsManagedResource` fail-safe-swallowed the resulting 401 into
`(false, nil)` — scope checks against the ACM backend silently always reported "unmanaged" with no
visible error.

After #1553: `acm.Client.Ping` (used by `readiness.ScopeCheckerProber`) does **not** swallow the
error, so the same 401 now correctly surfaces as `/readyz=NotReady` instead of being silently
absorbed.

### Resolution

There is currently no workaround short of not using the ACM backend (`fleet.backend: "acm"`) until
[#1556](https://github.com/jordigilh/kubernaut/issues/1556) ships bearer-token support. If you need
Fleet federation today and cannot wait, use the FMC backend (`fleet.backend: "fleetmetadatacache"`,
the chart default) instead.

Do **not** work around this by disabling the readiness gate or reverting to fail-open behavior for
the ACM backend specifically — that would just restore the previous silent-failure behavior (scope
checks always reporting "unmanaged" with no operator visibility), which is strictly worse than a
visible `NotReady`.

### Verification

Track [#1556](https://github.com/jordigilh/kubernaut/issues/1556) for the fix. Once shipped, confirm
via:

```bash
kubectl exec -n kubernaut-system deploy/<service> -- curl -sk -H "Authorization: Bearer $(cat /path/to/token)" \
  -H 'Content-Type: application/json' --data-raw '{"query":"{search(input:[{filters:[]}]){count}}"}' \
  https://<acm-search-endpoint>/searchapi/graphql
```
