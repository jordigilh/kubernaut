---
name: "submit-result"
description: "Submit the final investigation result. Use when RCA is complete and workflow selection is done (or determined not possible)."
---

# Submit Investigation Result

When you have completed your investigation, submit the structured result using `execute_code`:

```python
import json

result = {
    "root_cause": "BRIEF_DESCRIPTION",
    "severity": "critical",  # critical | high | warning | info
    "confidence": 0.85,      # 0.0 - 1.0
    "signal_type": "ImagePullBackOff",
    "remediation_target": {
        "kind": "Deployment",
        "name": "payment-api",
        "namespace": "demo-storefront",
        "apiVersion": "apps/v1"
    },
    "workflow_id": "rollback-deployment-v1",  # empty string if no match
    "workflow_version": "1.0.0",
    "execution_bundle": "quay.io/kubernaut-ai/workflows/rollback:v1.0.0@sha256:...",
    "workflow_parameters": {
        "TARGET_RESOURCE_NAME": "payment-api",
        "TARGET_NAMESPACE": "demo-storefront",
        "REVISION": "4"
    },
    "rationale": "Deployment was updated to non-existent image tag. Previous revision 4 is healthy and running.",
    "causal_chain": [
        "Deployment payment-api updated to image quay.io/kubernaut-ai/demo-payment:v2.broken",
        "Image tag v2.broken does not exist in registry",
        "Pod stuck in ImagePullBackOff after repeated pull failures",
        "Previous revision 4 (ubi-minimal:latest) is still running and healthy"
    ],
    "needs_human_review": False,
    "human_review_reason": ""
}

# Write to known output location for the HTTP adapter to pick up
with open("/workspace/output/investigation_result.json", "w") as f:
    json.dump(result, f, indent=2)

print("INVESTIGATION_COMPLETE")
print(json.dumps(result, indent=2))
```

## When to set needs_human_review = True

- Confidence below 0.6
- No matching workflow in catalog
- Workflow preconditions not met (e.g., no rollback target)
- Ambiguous root cause (multiple possible causes with similar likelihood)
- Target resource is in a namespace marked as critical/production AND confidence < 0.8

## When to leave workflow_id empty

- No workflow matches the identified action type
- Workflow exists but preconditions are not satisfied
- Root cause requires code changes (not infrastructure fix)
- Issue is transient and already resolving

In these cases, set `needs_human_review: true` and explain in `human_review_reason`.
