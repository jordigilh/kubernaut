#!/bin/bash
SOURCE="../${PWD##*/}.md"
SOURCE=${SOURCE//execution/execution}
SOURCE=${SOURCE//executor/executor}
SOURCE=${SOURCE//controller/controller}

# Extract based on common ## headers
awk '
/^## Overview/,/^## [^O]/ {if (!/^## [^O]/) print}
/^## Summary/,EOF {print}
' $SOURCE > overview.md

awk '/^## CRD Schema/,/^## [^C]/ {if (!/^## [^C]/) print}' $SOURCE > crd-schema.md
awk '/^## Controller Implementation/,/^## Current|^## Finalizer/ {if (!/^## (Current|Finalizer)/) print}' $SOURCE > controller-implementation.md
awk '/^## Reconciliation Architecture/,/^## Integration/ {if (!/^## Integration/) print}' $SOURCE > reconciliation-phases.md
awk '/^## Finalizer Implementation/,/^## CRD Lifecycle/ {if (!/^## CRD Lifecycle/) print}' $SOURCE > finalizers-part1.tmp
awk '/^## CRD Lifecycle Management/,/^## Testing/ {if (!/^## Testing/) print}' $SOURCE > finalizers-part2.tmp
cat finalizers-part1.tmp finalizers-part2.tmp > finalizers-lifecycle.md && rm finalizers-part*.tmp

awk '/^## Testing Strategy/,/^## Prometheus|^## Performance/ {if (!/^## (Prometheus|Performance)/) print}' $SOURCE > testing-strategy.md
awk '/^## Security Configuration/,/^## Observability/ {if (!/^## Observability/) print}' $SOURCE > security-configuration.md
awk '/^## Observability/,/^## Enhanced Metrics/ {if (!/^## Enhanced Metrics/) print}' $SOURCE > observability-logging.md
awk '/^## Prometheus Metrics/,/^## Database|^## Testing/ {if (!/^## (Database|Testing)/) print}' $SOURCE > metrics-part1.tmp
awk '/^## Enhanced Metrics/,/^## Implementation/ {if (!/^## Implementation/) print}' $SOURCE > metrics-part2.tmp
cat metrics-part1.tmp metrics-part2.tmp > metrics-slos.md && rm metrics-part*.tmp

awk '/^## Database Integration/,/^## Integration Points|^## Rego|^## Historical/ {if (!/^## (Integration Points|Rego|Historical)/) print}' $SOURCE > database-integration.md
awk '/^## Integration Points/,/^## RBAC|^## Critical/ {if (!/^## (RBAC|Critical)/) print}' $SOURCE > integration-points.md
awk '/^## Current State/,/^## Finalizer/ {if (!/^## Finalizer/) print}' $SOURCE > migration-current-state.md
awk '/^## Implementation Checklist/,/^## Summary/ {if (!/^## Summary/) print}' $SOURCE > implementation-checklist.md

# Count lines
wc -l *.md | tail -1
