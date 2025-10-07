#!/bin/bash
SRC="../03-workflow-execution.md"

# 1. Overview (53-93 + 2741-2807)
sed -n '53,93p' $SRC > overview.md
sed -n '2741,2807p' $SRC >> overview.md

# 2. CRD Schema (828-1333)
sed -n '828,1333p' $SRC > crd-schema.md

# 3. Controller Implementation (1334-1484)
sed -n '1334,1484p' $SRC > controller-implementation.md

# 4. Reconciliation Phases (195-780)
sed -n '195,780p' $SRC > reconciliation-phases.md

# 5. Finalizers & Lifecycle (1485-2127)
sed -n '1485,2127p' $SRC > finalizers-lifecycle.md

# 6. Testing Strategy (2214-2405)
sed -n '2214,2405p' $SRC > testing-strategy.md

# 7. Security Configuration (2537-2563)
sed -n '2537,2563p' $SRC > security-configuration.md

# 8. Observability & Logging (create placeholder from security section)
echo "## Observability & Logging" > observability-logging.md
echo "" >> observability-logging.md
echo "See [01-alertprocessor/observability-logging.md](../01-alertprocessor/observability-logging.md) for comprehensive patterns." >> observability-logging.md

# 9. Metrics & SLOs (2128-2213)
sed -n '2128,2213p' $SRC > metrics-slos.md

# 10. Database Integration (2406-2458)
sed -n '2406,2458p' $SRC > database-integration.md

# 11. Integration Points (2459-2536)
sed -n '2459,2536p' $SRC > integration-points.md

# 12. Migration & Current State (781-827)
sed -n '781,827p' $SRC > migration-current-state.md

# 13. Implementation Checklist (2564-2679)
sed -n '2564,2679p' $SRC > implementation-checklist.md

echo "Extraction complete"
wc -l *.md | tail -1
