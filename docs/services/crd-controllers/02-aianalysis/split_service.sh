#!/bin/bash
# Extract sections from 02-ai-analysis.md

SOURCE="../02-ai-analysis.md"

# 1. Overview (lines 52-159 + 5233-5249)
sed -n '52,159p' $SOURCE > overview.md
sed -n '5233,5249p' $SOURCE >> overview.md

# 2. CRD Schema (lines 1318-1477)  
sed -n '1318,1477p' $SOURCE > crd-schema.md

# 3. Controller Implementation (lines 1937-2265)
sed -n '1937,2265p' $SOURCE > controller-implementation.md

# 4. Reconciliation Phases (lines 234-633 + 1640-1936)
sed -n '234,633p' $SOURCE > reconciliation-phases.md
echo "" >> reconciliation-phases.md
sed -n '1640,1936p' $SOURCE >> reconciliation-phases.md

# 5. Finalizers & Lifecycle (lines 2496-3254)
sed -n '2496,3254p' $SOURCE > finalizers-lifecycle.md

# 6. Testing Strategy (lines 3255-3550)
sed -n '3255,3550p' $SOURCE > testing-strategy.md

# 7. Security Configuration (lines 4691-5079)
sed -n '4691,5079p' $SOURCE > security-configuration.md

# 8. Observability & Logging (lines 5080-5104)
sed -n '5080,5104p' $SOURCE > observability-logging.md

# 9. Metrics & SLOs (lines 5105-5121 + 3551-3757)
sed -n '3551,3757p' $SOURCE > metrics-slos.md
echo "" >> metrics-slos.md
sed -n '5105,5121p' $SOURCE >> metrics-slos.md

# 10. Database Integration (lines 3758-3937)
sed -n '3758,3937p' $SOURCE > database-integration.md

# 11. Integration Points (lines 634-1128)
sed -n '634,1128p' $SOURCE > integration-points.md

# 12. AI-Specific: HolmesGPT & Approval (lines 1478-1639 + 3938-4690)
sed -n '1478,1639p' $SOURCE > ai-holmesgpt-approval.md
echo "" >> ai-holmesgpt-approval.md
sed -n '3938,4690p' $SOURCE >> ai-holmesgpt-approval.md

# 13. Migration & Current State (lines 2266-2495)
sed -n '2266,2495p' $SOURCE > migration-current-state.md

# 14. Implementation Checklist (lines 5122-5232)
sed -n '5122,5232p' $SOURCE > implementation-checklist.md

echo "All documents extracted"
