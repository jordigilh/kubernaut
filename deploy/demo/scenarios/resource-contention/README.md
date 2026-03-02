# Resource Contention Demo

## Quick Start

```bash
./run.sh
```

## What It Demonstrates

**External actor interference detection** — when an external actor (GitOps controller, another operator, or manual intervention) repeatedly reverts Kubernaut's remediation, the platform detects the ineffective remediation chain via DataStorage hash analysis (spec_drift) and escalates to human review.

## Pipeline Path

- **Cycle 1**: OOMKill alert → increase limits (64Mi → 128Mi) → external actor reverts → OOMKill recurs
- **Cycle 2**: OOMKill alert → increase limits (64Mi → 128Mi) → external actor reverts → OOMKill recurs
- **Cycle 3**: RO detects ineffective chain (spec_drift) → blocks with ManualReviewRequired

## Business Requirement

BR-WORKFLOW-004

## Issue

#231
