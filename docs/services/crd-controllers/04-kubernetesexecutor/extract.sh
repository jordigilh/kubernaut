#!/bin/bash
SRC="../04-kubernetes-executor.md"

# 1. Overview (35-73 + any summary at end)
sed -n '35,73p' $SRC > overview.md
sed -n '2245,$p' $SRC >> overview.md

# 2. CRD Schema (880-1147)
sed -n '880,1147p' $SRC > crd-schema.md

# 3. Controller Implementation (1148-1479)
sed -n '1148,1479p' $SRC > controller-implementation.md

# 4. Reconciliation Phases (170-727)
sed -n '170,727p' $SRC > reconciliation-phases.md

# 5. Predefined Actions (728-815) - unique to this service
sed -n '728,815p' $SRC > predefined-actions.md

# 6. Finalizers & Lifecycle (1480-2244)
sed -n '1480,2244p' $SRC > finalizers-lifecycle.md

# 7. Testing Strategy (placeholder - appears missing)
echo "## Testing Strategy" > testing-strategy.md
echo "" >> testing-strategy.md
echo "See [01-alertprocessor/testing-strategy.md](../01-alertprocessor/testing-strategy.md) for comprehensive patterns." >> testing-strategy.md
echo "" >> testing-strategy.md
echo "**Service-Specific Adaptations for Kubernetes Executor**:" >> testing-strategy.md
echo "- Test Job creation and execution" >> testing-strategy.md
echo "- Test RBAC isolation per action" >> testing-strategy.md
echo "- Test rollback strategies" >> testing-strategy.md

# 8. Security Configuration (placeholder)
echo "## Security Configuration" > security-configuration.md
echo "" >> security-configuration.md
echo "See [01-alertprocessor/security-configuration.md](../01-alertprocessor/security-configuration.md) for comprehensive patterns." >> security-configuration.md

# 9. Observability & Logging (placeholder)
echo "## Observability & Logging" > observability-logging.md
echo "" >> observability-logging.md
echo "See [01-alertprocessor/observability-logging.md](../01-alertprocessor/observability-logging.md) for comprehensive patterns." >> observability-logging.md

# 10. Metrics & SLOs (placeholder)
echo "## Metrics & SLOs" > metrics-slos.md
echo "" >> metrics-slos.md
echo "See [01-alertprocessor/metrics-slos.md](../01-alertprocessor/metrics-slos.md) for comprehensive patterns." >> metrics-slos.md

# 11. Database Integration (placeholder)
echo "## Database Integration" > database-integration.md
echo "" >> database-integration.md
echo "See [01-alertprocessor/database-integration.md](../01-alertprocessor/database-integration.md) for comprehensive patterns." >> database-integration.md

# 12. Integration Points (extract from reconciliation section if exists)
echo "## Integration Points" > integration-points.md
echo "" >> integration-points.md
echo "See Reconciliation Architecture section for integration details." >> integration-points.md

# 13. Migration & Current State (816-879)
sed -n '816,879p' $SRC > migration-current-state.md

# 14. Implementation Checklist (placeholder)
echo "## Implementation Checklist" > implementation-checklist.md
echo "" >> implementation-checklist.md
echo "See [01-alertprocessor/implementation-checklist.md](../01-alertprocessor/implementation-checklist.md) for APDC-TDD phases." >> implementation-checklist.md

echo "Extraction complete"
wc -l *.md | tail -1
