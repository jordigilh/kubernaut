# Autonomous Mode Setup Guide (Gateway + AlertManager)

**Status**: Operator-facing setup guide, condensed from the Helm chart's own reference
(`charts/kubernaut/README.md`).
**Audience**: Operators and SREs who want alert-driven, webhook-triggered remediation —
Prometheus/AlertManager fires, Gateway ingests it as a signal, and Kubernaut investigates
and (subject to your approval policy) remediates, with no human chat interaction required
to kick it off.
**Authority**: `charts/kubernaut/README.md` ("Enable Monitoring Integration",
"Enable/Disable Gateway or APIFrontend"), Issue #2162.

---

## What you're building

```
Prometheus ──fires──▶ AlertManager ──webhook (Bearer token)──▶ Gateway
                                                                   │
                                                                   ▼
                                              RemediationRequest CRD created
                                                                   │
                                                                   ▼
                              SignalProcessing → AIAnalysis → RemediationWorkflow → WorkflowExecution
```

This is Kubernaut's webhook-driven ingestion path. **Console and APIFrontend (AF) are not
part of it** — nothing here requires a human to open a chat session. Gateway and AF are
independent entry points into the same `RemediationRequest` pipeline (Issue #2162); this
guide only sets up Gateway's.

If you also want the chat-driven path (a human investigating on demand instead of, or
alongside, alerts), see
[INTERACTIVE_MODE_SETUP_GUIDE.md](./INTERACTIVE_MODE_SETUP_GUIDE.md) — the two are
independent and can both be enabled on the same install.

---

## Prerequisites

- A Kubernetes cluster with a dynamic-provisioning `StorageClass` (for PostgreSQL/Valkey)
- Helm 3.12+
- An API key from an LLM provider (OpenAI, Anthropic, etc.)
- [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
  installed (or an existing Prometheus + AlertManager you control the config for)

If you don't already have Prometheus/AlertManager running:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace
```

---

## 1. Install Kubernaut with Gateway + monitoring enabled

Follow [charts/kubernaut/README.md](../../../charts/kubernaut/README.md)'s Quick Start for
the namespace and the three credential Secrets (PostgreSQL, Valkey, LLM provider API key),
plus your Rego policies, then install with monitoring wired to your Prometheus/AlertManager:

```bash
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set global.llmProfiles.primary.provider=openai \
  --set global.llmProfiles.primary.model=gpt-4o \
  --set global.llmProfiles.primary.endpoint=https://api.openai.com/v1 \
  --set global.llmProfiles.primary.credentialsSecretName=llm-credentials \
  --set kubernautAgent.llmProfileRef=primary \
  --set-file signalprocessing.policies.content=path/to/policy.rego \
  --set-file aianalysis.policies.content=path/to/approval.rego \
  --set apifrontend.enabled=false \
  --set monitoring.prometheus.enabled=true \
  --set monitoring.prometheus.url=http://kube-prometheus-stack-prometheus.monitoring.svc:9090 \
  --set monitoring.alertManager.enabled=true \
  --set monitoring.alertManager.url=http://kube-prometheus-stack-alertmanager.monitoring.svc:9093 \
  --set gateway.auth.signalSources[0].name=alertmanager \
  --set gateway.auth.signalSources[0].serviceAccount=alertmanager-kube-prometheus-stack-alertmanager \
  --set gateway.auth.signalSources[0].namespace=monitoring
```

`gateway.enabled` defaults to `true` — no need to set it. `apifrontend.enabled=false` above
is optional; it's set here to keep this a minimal, Gateway-only footprint (no
Console/AF Deployment, Service, RBAC, or NetworkPolicy at all) matching this guide's scope.
Leave it at its default (`true`) if you might add [Interactive
mode](./INTERACTIVE_MODE_SETUP_GUIDE.md) later without a redeploy.

`gateway.auth.signalSources` is the actual authorization gate for who can call Gateway's
webhook endpoint — it defaults to an empty list, so **nothing** can POST a signal to
Gateway until you add an entry here. Each entry creates a `ClusterRoleBinding` granting
that `ServiceAccount` `create` on `services/gateway-service`, checked via a real
`TokenReview`+`SubjectAccessReview` on every request (401 for a missing/invalid token, 403
for a valid token with no matching binding).

## 2. Point AlertManager at Gateway

```yaml
receivers:
  - name: kubernaut
    webhook_configs:
      - url: "http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus"
        send_resolved: true
        http_config:
          bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token

route:
  routes:
    - receiver: kubernaut
      matchers:
        - alertname!=""
      continue: true
```

Apply this to your `Alertmanager` CR's `alertmanager.yaml` (or the equivalent
`AlertmanagerConfig` if you're using the Prometheus Operator's namespaced config CRD). The
`bearer_token_file` path is AlertManager's own auto-mounted ServiceAccount token — the same
identity `gateway.auth.signalSources[0].serviceAccount` above authorizes.

## 3. (Optional) ServiceMonitor + PrometheusRule + autoscaling

If the Prometheus Operator CRDs are installed, the chart can generate its own
`ServiceMonitor`/`PrometheusRule` for observability parity with the Kubernaut Operator, and
`HorizontalPodAutoscaler`s for DataStorage/APIFrontend:

```bash
helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system --reuse-values \
  --set monitoring.serviceMonitor.enabled=true \
  --set monitoring.prometheusRule.enabled=true \
  --set datastorage.autoscaling.enabled=true
```

Both `serviceMonitor`/`prometheusRule` are safe to leave `true` even on clusters without the
Prometheus Operator CRDs installed — they no-op (render nothing) rather than fail.

---

## Verify

Fire a test alert (or wait for a real one) and confirm a `RemediationRequest` is created:

```bash
kubectl get remediationrequests -n kubernaut-system -w
```

Check Gateway's own logs for the `TokenReview`/`SubjectAccessReview` result if nothing
appears:

```bash
kubectl logs -n kubernaut-system deployment/gateway --tail=50
```

For an end-to-end example with a real fault (not just a synthetic test alert), see the
[Try It Out](../../../README.md#try-it-out) section of the root README —
[kubernaut-demo-scenarios](https://github.com/jordigilh/kubernaut-demo-scenarios)'
`crashloop` scenario exercises exactly this path.

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| AlertManager webhook returns `401` | `bearer_token_file` missing/wrong, or `gateway.enabled=false` | Confirm the path in step 2 and that Gateway is running |
| AlertManager webhook returns `403` | No `gateway.auth.signalSources` entry for AlertManager's ServiceAccount | Add the entry from step 1, matching AlertManager's actual ServiceAccount name/namespace |
| No `RemediationRequest` created despite a `200`/`201` from Gateway | Signal deduplicated (identical fingerprint within the dedup window), or the alert's target resource doesn't exist/isn't labeled `kubernaut.ai/managed=true` | Check Gateway logs for the dedup/scope-check decision |
| `ServiceMonitor`/`PrometheusRule` don't appear | Prometheus Operator CRDs (`monitoring.coreos.com/v1`) not installed on this cluster | Expected — both are a deliberate no-op without the CRDs, not an error |

---

## References

- [charts/kubernaut/README.md](../../../charts/kubernaut/README.md) — full Helm chart reference
- [Enable/Disable Gateway or APIFrontend](../../../charts/kubernaut/README.md#enable-disable-gateway-or-apifrontend-issue-2162)
- [INTERACTIVE_MODE_SETUP_GUIDE.md](./INTERACTIVE_MODE_SETUP_GUIDE.md) — the complementary chat-driven entry point
- [FLEET_SETUP_GUIDE.md](./FLEET_SETUP_GUIDE.md) — layering multi-cluster fleet mode on top of this
- [Issue #2162: Gateway/APIFrontend independent enable toggles](https://github.com/jordigilh/kubernaut/issues/2162)
