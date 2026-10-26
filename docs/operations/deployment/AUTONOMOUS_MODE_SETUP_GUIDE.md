# Autonomous Mode Setup Guide (Gateway + AlertManager)

**Status**: Operator-facing setup guide for the Kubernaut Operator deployment.
**Audience**: Operators and SREs who want alert-driven, webhook-triggered remediation —
Prometheus/AlertManager fires, Gateway ingests it as a signal, and Kubernaut investigates
and (subject to your approval policy) remediates, with no human chat interaction required
to kick it off.
**Authority**: [Kubernaut Operator installation guide](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md),
Issue #2162.

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

Install the Kubernaut Operator and apply a `Kubernaut` CR using the
[operator installation guide](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md).
Set the CR's LLM, policy, Gateway, and monitoring fields for this mode; the
application Helm chart is no longer shipped by this repository.

For a minimal Gateway-only deployment, apply a CR like this after installing the
Operator. Set `apiFrontend.enabled: false` only when Interactive mode is not
needed. The `gateway.auth.signalSources` entry is required for AlertManager to
call Gateway; without it, valid tokens receive `403`.

```yaml
apiVersion: kubernaut.ai/v1alpha2
kind: Kubernaut
metadata:
  name: kubernaut
  namespace: kubernaut-system
spec:
  apiFrontend:
    enabled: false
  gateway:
    auth:
      enabled: true
      signalSources:
        - name: alertmanager
          serviceAccount: alertmanager-kube-prometheus-stack-alertmanager
          namespace: monitoring
```

Each signal source creates the RBAC binding used by Gateway's
`TokenReview`/`SubjectAccessReview` check. Missing or invalid tokens receive
`401`; valid tokens without a matching binding receive `403`.

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

If the Prometheus Operator CRDs are installed, configure the corresponding
`ServiceMonitor`/`PrometheusRule` and autoscaling behavior through the Operator CR
and its dependency stack.

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

- [Kubernaut Operator installation guide](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md)
- [INTERACTIVE_MODE_SETUP_GUIDE.md](./INTERACTIVE_MODE_SETUP_GUIDE.md) — the complementary chat-driven entry point
- [FLEET_SETUP_GUIDE.md](./FLEET_SETUP_GUIDE.md) — layering multi-cluster fleet mode on top of this
- [Issue #2162: Gateway/APIFrontend independent enable toggles](https://github.com/jordigilh/kubernaut/issues/2162)
