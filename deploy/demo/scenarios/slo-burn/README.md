# Scenario #128: SLO Error Budget Burn -> Proactive Rollback

## Overview

This scenario demonstrates Kubernaut detecting that a service is burning its SLO error budget at an unsustainable rate, and proactively rolling back the deployment to preserve the SLO before it breaches.

This is arguably the **highest enterprise-value** demo scenario because it connects **business objectives** (SLOs) directly to automated remediation. The LLM adds unique value by:

- Correlating the error budget burn timing with the most recent deployment revision
- Reasoning about which revision caused the degradation
- Choosing rollback to the specific revision that introduced the errors
- Distinguishing between: bad deploy (rollback), traffic spike (scale out), dependency failure (wait)
- Explaining in the audit trail: "Error budget burning at 14x sustainable rate since revision N. Rolling back to preserve SLO."

## Failure Mode

The injection is realistic: a **bad nginx config** that returns 500 on `/api/` but passes health checks (`/healthz` returns 200). This mirrors a real production issue where readiness probes pass but the service is functionally broken.

## Prerequisites

- Kind cluster created with `deploy/demo/overlays/kind/kind-cluster-config.yaml`
- Kubernaut services deployed with HAPI configured for a real LLM backend
- Prometheus with nginx metrics collection (stub_status or exporter)
- `ProactiveRollback` action type registered in DataStorage (migration 026)
- `proactive-rollback-v1` workflow registered in the workflow catalog

## BDD Specification

```gherkin
Feature: SLO Error Budget Burn -> Proactive Rollback

  Scenario: Error budget burning at unsustainable rate triggers proactive rollback
    Given an nginx api-gateway Deployment with 2 replicas in demo-slo namespace
      And a traffic generator sending steady requests to /api/status
      And the service is healthy with ~0% error rate (SLO: 99.9%)
      And the Kubernaut pipeline is active with a real LLM
      And the "proactive-rollback-v1" workflow is registered in the catalog

    When a bad ConfigMap is deployed (returns 500 on /api/)
      And the Deployment is rolling-restarted to pick up the new config
      And the error rate spikes to ~100% on the /api/ path
      And health checks (/healthz) continue to pass

    Then Prometheus detects the error rate exceeds 14.4x sustainable burn rate
      And ErrorBudgetBurn alert fires after 5 minutes
      And Signal Processing enriches with deployment revision history
      And the LLM correlates the error spike with the recent deployment change
      And the LLM selects the ProactiveRollback action type
      And Remediation Orchestrator creates a WorkflowExecution
      And the WE Job rolls back the deployment to the previous revision
      And the rollout completes with all replicas ready
      And the error rate drops back within SLO (< 0.1%)
      And EffectivenessAssessment confirms the SLO is preserved
```

## Automated Execution

```bash
./deploy/demo/scenarios/slo-burn/run.sh
```

This script:
1. Deploys the namespace, API gateway, traffic generator, and Prometheus rules
2. Establishes a 30s healthy traffic baseline
3. Injects the bad config and triggers a rolling restart
4. Prints monitoring instructions

## Manual Step-by-Step

```bash
# 1. Deploy namespace, ConfigMap, API gateway, and traffic generator
kubectl apply -f deploy/demo/scenarios/slo-burn/manifests/
kubectl wait --for=condition=Available deployment/api-gateway \
  -n demo-slo --timeout=60s

# 2. Deploy Prometheus SLO alerting rules
kubectl apply -f deploy/demo/scenarios/slo-burn/manifests/prometheus-rule.yaml

# 3. Let healthy traffic run for ~30 seconds (establishes baseline)
sleep 30

# 4. Check Prometheus for healthy metrics
#    kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090 &
#    then open http://localhost:9090 -> job:http_requests:error_rate_5m should be ~0

# 5. Inject: deploy bad config (500 errors on /api/)
./deploy/demo/scenarios/slo-burn/inject-bad-config.sh

# 6. Watch error rate climb in Prometheus
#    The SLO burn rate alert should fire within ~5 minutes

# 7. Watch Kubernaut pipeline
kubectl get rr,sp,aa,we,ea -n demo-slo -w

# 8. Verify: deployment rolled back, error rate within SLO
kubectl rollout history deployment/api-gateway -n demo-slo
kubectl get pods -n demo-slo

# 9. Cleanup
./deploy/demo/scenarios/slo-burn/cleanup.sh
```

## Acceptance Criteria

- [ ] nginx API gateway + ConfigMap + traffic generator manifests
- [ ] Injection script to deploy bad config
- [ ] Prometheus SLO burn rate alerting rule
- [ ] nginx metrics exported to Prometheus (stub_status or exporter)
- [ ] Full pipeline with real LLM: Gateway -> RO -> SP -> AA -> WE -> EM
- [ ] LLM correlates error spike with deployment revision and selects rollback
- [ ] EffectivenessAssessment shows error rate within SLO, alert resolved
- [ ] Demo documentation with step-by-step instructions

## Memory Budget

| Component | Estimate |
|---|---|
| nginx (api-gateway, 2 replicas) | ~64MB |
| Traffic generator | ~16MB |
| nginx-prometheus-exporter | ~16MB |
| **Additional overhead** | **~100MB** |
| **Total cluster** | **~4.7GB** |
| **Headroom on 12GB** | **~7.3GB** |

## Cleanup

```bash
./deploy/demo/scenarios/slo-burn/cleanup.sh
```

## Notes

- **Readiness vs. Functionality**: The /healthz endpoint still returns 200 while /api/ returns 500. This is a realistic production failure mode where health checks pass but the service is broken for users.
- **Proactive vs. Reactive**: Per ADR-054, the `signal_mode` classification happens at the SP layer (runtime), not in the workflow schema labels. The workflow schema uses the normalized base signal type.
- **Shared Rollback**: The remediation action (`kubectl rollout undo`) is the same as #120 (CrashLoopBackOff). The difference is the trigger (SLO burn rate vs. pod crash) and the LLM's reasoning (business objective vs. health check).
