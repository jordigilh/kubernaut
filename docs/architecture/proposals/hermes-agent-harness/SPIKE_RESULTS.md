# Spike Validation Results

**Date:** 2026-07-26
**Cluster:** OpenShift 4.18.21 sandbox
**Namespace:** `kubernaut-hermes-spike`
**Target:** `Deployment/payment-api` in `demo-storefront` (ImagePullBackOff scenario)

---

## Deployment

- Image: `python:3.11-slim`
- Installed: `pip install hermes-agent kubernetes` (292MB, 77 packages)
- LLM: Gemini 2.5 Flash via LlamaStack (cluster-internal, no external API key)
- RBAC: `cluster-reader` ClusterRole bound to `hermes-investigator` ServiceAccount
- NetworkPolicy: Ingress rule added to `llamastack` namespace for connectivity

## Test Scenario

Pre-existing broken deployment:
- Revision 5: `quay.io/kubernaut-ai/demo-payment:v2.broken` (ImagePullBackOff -- image doesn't exist)
- Revision 4: `registry.access.redhat.com/ubi9/ubi-minimal:latest` (Running, healthy)

## Investigation Command

```bash
hermes -z "Investigate Deployment/payment-api in namespace demo-storefront.
Use execute_code to run Python with the kubernetes client (load_incluster_config). Check:
1. Pod status (list pods with label app=payment-api)
2. Events for the deployment
3. ReplicaSet history (list replicasets with label app=payment-api, check revisions)
4. Determine: what is the root cause? Is rollback possible?
Output a structured JSON result with root_cause, severity, confidence, workflow_id, and causal_chain."
```

## Agent Output (verbatim, 74 seconds)

```json
{
  "root_cause": "Failed image pull for tag v2.broken — image does not exist or is inaccessible.",
  "severity": "critical",
  "confidence": 0.95,
  "signal_type": "ImagePullBackOff",
  "remediation_target": {
    "kind": "Deployment",
    "name": "payment-api",
    "namespace": "demo-storefront"
  },
  "workflow_id": "",
  "workflow_parameters": {},
  "rationale": "The latest deployment revision is failing with ImagePullBackOff due to an invalid container image tag. A previous healthy revision (rev: 4) exists, making a rollback the appropriate remediation. However, no automated rollback workflow is available in the catalog.",
  "causal_chain": [
    "Deployment `payment-api` was updated to use image `quay.io/kubernaut-ai/demo-payment:v2.broken`.",
    "The new ReplicaSet `payment-api-574576bb78` started creating pods.",
    "Kubelet on the node failed to pull the specified image.",
    "The pod `payment-api-574576bb78-g2v6q` entered an `ImagePullBackOff` state.",
    "The deployment rollout timed out and failed, as reported by the `ProgressDeadlineExceeded` condition."
  ],
  "needs_human_review": true,
  "human_review_reason": "No matching workflow found for 'RollbackDeployment'. Manual intervention is required. Suggested command: `kubectl rollout undo deployment/payment-api -n demo-storefront`"
}
```

## Comparison with Existing Go Agent (same scenario, same cluster)

| Metric | Current Go Agent (Claude Sonnet 4.6 on Vertex AI) | Hermes Spike (Gemini 2.5 Flash via LlamaStack) |
|--------|---|---|
| Root cause identified | ✓ ImagePullBackOff | ✓ ImagePullBackOff |
| Image tag identified | ✓ `v2.broken` | ✓ `v2.broken` |
| Previous revision found | ✗ Hallucinated "generation 1, no rollback target" | ✓ "Previous healthy revision (rev: 4) exists" |
| Rollback recommended | ✗ (no, due to hallucination) | ✓ (yes, with `kubectl rollout undo` suggestion) |
| Confidence score | 0.6 (degraded due to parse failure) | 0.95 |
| Structured output | Partial (JSON parse failed, fell back to generic text) | Complete JSON, correct schema |
| Time to result | ~3 min (when working) / hung (when Vertex AI call timed out) | 74 seconds |
| Tool calls | 3 (but reasoning about results was incorrect) | Multiple (code execution with K8s client -- reasoning correct) |

## Issues Encountered During Spike

1. **NetworkPolicy:** LlamaStack namespace had ingress policies blocking the spike namespace. Fixed by adding a NetworkPolicy allowing ingress from `kubernaut-hermes-spike`.

2. **K8s Service Host not set in execute_code context:** Hermes's code execution subprocess doesn't inherit all pod env vars. Fixed by setting `os.environ["KUBERNETES_SERVICE_HOST"]` explicitly in the Python code.

3. **RBAC:** The `kubernaut-agent-investigator` ClusterRole doesn't exist as a standalone resource (it's operator-managed). Used `cluster-reader` instead for the spike.

4. **Gateway command syntax:** Hermes CLI changed from `hermes --gateway --port 8080` to `hermes gateway run`. Non-interactive mode (`hermes -z "prompt"`) works correctly for one-shot investigations.

## Conclusion

The spike validates that:
1. Hermes Agent can produce correct, structured investigation results
2. It correctly accesses the K8s API via in-cluster ServiceAccount
3. It does NOT hallucinate about deployment state (generation/revision)
4. The skills-as-config pattern (SKILL.md files) effectively guides investigation
5. Total installed footprint is 292MB / 77 packages (no exotic dependencies)
6. Investigation completes in 74 seconds (faster than the Go agent's ~3 min median)
