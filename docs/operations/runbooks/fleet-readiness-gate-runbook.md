# Fleet Readiness Gate - Production Runbooks

**Version**: v1.1
**Last Updated**: 2026-07-07
**Status**: ✅ Production Ready
**Applies to**: Gateway (GW), RemediationOrchestrator (RO), EffectivenessMonitor (EM),
SignalProcessing (SP), WorkflowExecution (WE), APIFrontend (AF), KubernautAgent (KA)
**Related**: [ADR-068: Fleet Federation Architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md#fleet-readiness-gate-fail-closed-pod-wide-readyz-1553)

---

## 📚 Runbook Index

| ID | Runbook | Triggers On | Automation |
|----|---------|-------------|------------|
| RB-FLEET-001 | [Pod stuck NotReady due to Fleet dependency](#rb-fleet-001-pod-stuck-notready-due-to-fleet-dependency) | `/readyz` returns 503, pod removed from Service endpoints | Alert + Dashboard |
| RB-FLEET-002 | [ACM backend NotReady due to auth/RBAC misconfiguration](#rb-fleet-002-acm-backend-notready-due-to-authrbac-misconfiguration) | `fleet.backend: "acm"` deployments fail to start or never reach Ready | Alert |

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
#   - ACM Search backend: see RB-FLEET-002 first — a permanently-unreachable ACM backend is
#     usually an auth/RBAC misconfiguration (missing/invalid bearer token, missing RBAC
#     bindings), not a transient outage.

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

## RB-FLEET-002: ACM backend NotReady due to auth/RBAC misconfiguration

> **History**: `pkg/fleet/acm.Client` originally never sent the `Authorization: Bearer <token>`
> header the real ACM Search GraphQL API (`stolostron/search-v2-api`) mandatorily requires — a
> pre-existing gap tracked as **[#1556](https://github.com/jordigilh/kubernaut/issues/1556)**.
> Before #1553, this failed silently (`IsManagedResource` fail-safe-swallowed the 401 into
> `(false, nil)`). After #1553 and before #1556 landed, it surfaced as a **permanent**
> `/readyz=NotReady` for every `fleet.backend: "acm"` deployment. **#1556 is now resolved**:
> `pkg/fleet/scope_factory.go` composes `auth.AuthTransport` (reading the token from
> `FleetConfig.TokenPath`) into the ACM client's HTTP transport, and `FleetConfig.Validate()`
> hard-rejects `backend: "acm"` without `TokenPath` set (fail-closed at startup instead of at
> the readiness probe). A related defect in `acm.Client.Ping()` (sent an empty search filter,
> which real ACM Search rejects with a GraphQL error) was also found and fixed during a
> live-cluster spike against ACM 2.16.2. This runbook now covers the auth/RBAC misconfigurations
> that can still cause ACM-backend `NotReady` after those fixes.

### Symptoms

- Only affects deployments with `fleet.backend: "acm"` configured.
- **Fails at startup** (pod crash-loops, does not reach `Running`) if `fleet.tokenPath` /
  `<service>.fleet.tokenSecretRef` is unset — this is expected, fail-closed behavior per #1556,
  not a bug. `FleetConfig.Validate()` rejects the config before the process starts serving.
- **Pod is `Running` but permanently `NotReady`** if `TokenPath` is set but the token is invalid,
  expired, or lacks RBAC visibility — RB-FLEET-001's diagnosis steps will show the ACM Search
  endpoint itself is reachable (e.g., `curl` from outside the pod succeeds or returns 401/403,
  rather than a connection error).

### Root Cause

Most commonly one of:

1. **Missing `tokenPath`/`tokenSecretRef`** — `<service>.fleet.backend: "acm"` was set without
   `<service>.fleet.tokenSecretRef` in Helm values. `Validate()` now catches this at startup
   (`fleet: tokenPath is required when backend=acm`); it is not a `/readyz` symptom.
2. **Invalid or expired token** — the ServiceAccount token referenced by `tokenSecretRef` was
   revoked, rotated out-of-band, or the Secret contains a stale value.
3. **RBAC not configured on the ACM hub** — per the ADR-068 setup guide below, ACM Search
   enforces two independent RBAC layers (K8s RBAC + a `view` RoleBinding in each managed
   cluster's hub namespace). Missing either layer returns `count: 0` or a 403, not a connection
   error.
4. **`fine-grained-rbac` MCH component not enabled** — see ADR-068's ACM Search Production
   Setup Guide, Step 1.

### Resolution

1. Confirm `<service>.fleet.tokenSecretRef` is set in Helm values for the affected service, and
   that the referenced Secret exists in the pod's namespace with a `token` key:
   ```bash
   kubectl get secret -n kubernaut-system <tokenSecretRef> -o jsonpath='{.data.token}' | base64 -d | head -c 20
   ```
2. Confirm the mounted token is current and matches what was issued for the `kubernaut-fleet-reader`
   ServiceAccount on the ACM hub (see [ADR-068](../../architecture/decisions/ADR-068-fleet-federation-architecture.md#acm-search-production-setup-guide),
   Step 6 for cross-cluster token generation/rotation).
3. Confirm RBAC per the ADR-068 setup guide: `fine-grained-rbac` MCH component enabled, K8s
   ClusterRole/ClusterRoleBinding for the SA, and a `view` RoleBinding in each managed cluster's
   hub namespace.
4. If none of the above resolves it, use the Verification step to reproduce the exact request
   the pod is making and inspect the GraphQL response for `errors`.

Do **not** work around this by disabling the readiness gate or reverting to fail-open behavior for
the ACM backend specifically — that would just restore the previous silent-failure behavior (scope
checks always reporting "unmanaged" with no operator visibility), which is strictly worse than a
visible `NotReady`.

### Verification

```bash
# Exec into the pod and confirm the mounted token authenticates successfully
kubectl exec -n kubernaut-system deploy/<service> -- sh -c \
  'curl -sk -H "Authorization: Bearer $(cat /etc/<service>/<tokenSecretRef>/token)" \
   -H "Content-Type: application/json" \
   --data-raw "{\"query\":\"query(\$input:[SearchInput]){searchResult:search(input:\$input){count}}\",\"variables\":{\"input\":[{\"filters\":[{\"property\":\"kind\",\"values\":[\"Namespace\"]}]}]}}" \
   https://<acm-search-endpoint>/searchapi/graphql'
# Expected: {"data":{"searchResult":[{"count":N}]}} with N > 0 and no "errors" key.
# A 401/403 means the token or RBAC is the problem; a populated "errors" array with count:0
# despite a 200 status usually means the RBAC "view" RoleBinding (Step 5 of the ADR-068 setup
# guide) is missing on the managed cluster namespace being queried.
```
