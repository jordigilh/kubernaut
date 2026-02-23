# Resource Quota Exhaustion Demo

## Overview

Demonstrates Kubernaut distinguishing policy constraints (ResourceQuota exhaustion) from infrastructure failures. When pods are stuck in Pending due to quota limits, the LLM escalates to human review rather than attempting automated remediation.

**Signal**: `KubePodPendingQuotaExhausted` -- pods Pending in namespace with ResourceQuota
**Root cause**: ResourceQuota limits exceeded (768Mi requested > 512Mi quota)
**Expected behavior**: LLM recognizes policy constraint → `needs_human_review: true` → ManualReviewNotification

## Pipeline Path

```
Alert (KubePodPendingQuotaExhausted) -> SP -> RR -> AA (NeedsHumanReview) -> ManualReviewNotification
```

## Quick Start

```bash
./deploy/demo/scenarios/resource-quota-exhaustion/run.sh
```

## What It Demonstrates

- LLM distinguishes policy constraints from infrastructure failures
- ResourceQuota exhaustion triggers escalation to human review
- No automated workflow is applied; manual quota increase or scaling down is required

## Business Requirement

- **BR-SEVERITY-001**: Severity classification and escalation policies

## Issue

- #171
