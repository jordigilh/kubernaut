#!/bin/bash
SRC="../05-central-controller.md"

# 1. Overview (14-49 + 2963-3046)
sed -n '14,49p' $SRC > overview.md
sed -n '2963,3046p' $SRC >> overview.md

# 2. CRD Schema (918-1098)
sed -n '918,1098p' $SRC > crd-schema.md

# 3. Controller Implementation (1099-1381)
sed -n '1099,1381p' $SRC > controller-implementation.md

# 4. Reconciliation Phases (147-462)
sed -n '147,462p' $SRC > reconciliation-phases.md

# 5. Data Handling Architecture (2422-2741) - unique to central controller
sed -n '2422,2741p' $SRC > data-handling-architecture.md

# 6. Finalizers & Lifecycle (1504-2288)
sed -n '1504,2288p' $SRC > finalizers-lifecycle.md

# 7. Testing Strategy (2289-2361)
sed -n '2289,2361p' $SRC > testing-strategy.md

# 8. Security Configuration (2793-2832)
sed -n '2793,2832p' $SRC > security-configuration.md

# 9. Observability & Logging (placeholder)
echo "## Observability & Logging" > observability-logging.md
echo "" >> observability-logging.md
echo "See [01-alertprocessor/observability-logging.md](../01-alertprocessor/observability-logging.md) for comprehensive patterns." >> observability-logging.md

# 10. Metrics & SLOs (2362-2421)
sed -n '2362,2421p' $SRC > metrics-slos.md

# 11. Database Integration (2742-2792)
sed -n '2742,2792p' $SRC > database-integration.md

# 12. Integration Points (463-917)
sed -n '463,917p' $SRC > integration-points.md

# 13. Migration & Current State (placeholder)
echo "## Current State & Migration Path" > migration-current-state.md
echo "" >> migration-current-state.md
echo "TBD - Central controller is new implementation." >> migration-current-state.md

# 14. Implementation Checklist (2833-2949)
sed -n '2833,2949p' $SRC > implementation-checklist.md

echo "Extraction complete"
wc -l *.md | tail -1
