# Concurrent Cross-Namespace Demo

## Quick Start

```bash
./run.sh
```

## What It Demonstrates

Same incident (CrashLoopBackOff), different risk tolerances → different workflow selection.

- **Team Alpha** (high risk tolerance): Gets `restart-pods-v1` — simpler, less disruptive.
- **Team Beta** (low risk tolerance): Gets `crashloop-rollback-v1` — more thorough, safer.

## Key Mechanism

1. **SignalProcessing Rego policy** maps namespace label `kubernaut.ai/risk-tolerance` into customLabels (`risk_tolerance`).
2. **DataStorage** scores workflows by customLabels match.
3. **LLM** selects the workflow that aligns with the team's risk tolerance.

## Pipeline Path (Parallel)

| Team  | Path                                                                 |
|-------|----------------------------------------------------------------------|
| Alpha | Alert → SP (risk_tolerance=high) → AA → restart-pods-v1               |
| Beta  | Alert → SP (risk_tolerance=low) → AA → crashloop-rollback-v1         |

## Business Requirement

- **BR-SP-102**: Custom labels enrichment for workflow scoring

## Issue

- **#172**: Concurrent cross-namespace scenario

## Note

Custom labels Rego policies use `package signalprocessing.customlabels` with a `labels` rule. The engine queries `data.signalprocessing.customlabels.labels`, keeping custom labels in a dedicated package separate from the 5 mandatory label classifiers.
