# Memory Escalation Demo

## Quick Start

```bash
./run.sh
```

## What It Demonstrates

**Diminishing returns detection** — when the same remedy is applied multiple times without resolving the issue, the platform escalates to human review instead of repeatedly applying automated remediation.

## Pipeline Path

- **Cycle 1**: OOMKill alert → increase limits (64Mi → 128Mi) → OOMKill recurs
- **Cycle 2**: OOMKill alert → increase limits (128Mi → 256Mi) → OOMKill recurs
- **Cycle 3**: RO blocks (ConsecutiveFailures threshold) → human review

## Business Requirement

BR-WORKFLOW-004

## Issue

#168
