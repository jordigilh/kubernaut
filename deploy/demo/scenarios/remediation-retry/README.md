# Remediation Retry Demo

## Quick Start

```bash
./run.sh
```

## What It Demonstrates

The platform escalates through workflow options when the first remediation attempt fails. A rolling restart cannot fix a bad configuration—only a rollback can. The demo shows how the LLM selects a different workflow on the second cycle after the first one fails.

## Pipeline Path

- **Cycle 1**: Alert → SP → AA (restart-pods) → WE (FAIL) → EM (effectiveness: failed)
- **Cycle 2**: Alert re-fires → AA (rollback) → WE (SUCCESS) → EM (effectiveness: resolved)

## Business Requirement

BR-WORKFLOW-004

## Issue

#167
