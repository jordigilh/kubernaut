# Documentation Maintenance Guide

**Last Updated**: 2025-01-15
**Structure Version**: 1.0
**Applies To**: All CRD service documentation in `docs/services/crd-controllers/`

---

## 📋 Table of Contents

1. [Overview](#overview)
2. [Common Pattern Files](#common-pattern-files)
3. [Updating Common Patterns](#updating-common-patterns)
4. [Adding New Service](#adding-new-service)
5. [Service-Specific Updates](#service-specific-updates)
6. [Directory Structure Rules](#directory-structure-rules)
7. [File Naming Conventions](#file-naming-conventions)
8. [Cross-Reference Validation](#cross-reference-validation)
9. [Troubleshooting](#troubleshooting)

---

## Overview

The CRD service documentation is organized into **self-contained directories** (one per service). Each directory contains **13-15 documents** that follow a consistent structure across all services.

### **Key Principles**

1. **Self-Containment**: Everything for a service lives in one directory
2. **Controlled Duplication**: Common patterns are duplicated with clear markers
3. **Progressive Disclosure**: README provides navigation hub
4. **Parallel-Friendly**: Multiple developers can work without conflicts

---

## Common Pattern Files

These files are **duplicated across all 5 services** with service-specific adaptations:

| File | Purpose | Services | Duplication Strategy |
|------|---------|----------|----------------------|
| `testing-strategy.md` | Unit/Integration/E2E test patterns | All 5 | Copy from pilot with adaptations |
| `security-configuration.md` | RBAC, network policies, secrets | All 5 | Copy from pilot with adaptations |
| `observability-logging.md` | Structured logging, tracing | All 5 | Copy from pilot with adaptations |
| `metrics-slos.md` | Prometheus metrics, Grafana dashboards | All 5 | Copy from pilot with adaptations |

### **Common Pattern Header**

All common pattern files have this header:

```markdown
<!-- COMMON-PATTERN: This file is duplicated across all CRD services -->
<!-- LAST-UPDATED: 2025-01-15 -->
<!-- SERVICES: 01-signalprocessing, 02-aianalysis, 03-workflowexecution, 04-kubernetesexecutor, 05-remediationorchestrator -->
```

**Why the header?**
- Clearly identifies duplicated content
- Tracks last update date
- Lists all services using this pattern

---

## Updating Common Patterns

### **Step-by-Step Process**

#### **1. Update Pilot Service (01-signalprocessing/)**

This is the "source of truth" for common patterns.

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/services/crd-controllers

# Edit the common pattern file
vi 01-signalprocessing/testing-strategy.md

# Make your changes to the common pattern
```

#### **2. Test Changes**

Validate the changes work correctly:
- Review the content
- Check all code examples
- Verify markdown formatting
- Ensure service-agnostic language

#### **3. Copy to Other Services**

Copy the updated file to all other services:

```bash
# Copy to all services
for service in 02-aianalysis 03-workflowexecution 04-kubernetesexecutor 05-remediationorchestrator; do
  cp 01-signalprocessing/testing-strategy.md ${service}/
  echo "Copied to ${service}/"
done
```

#### **4. Make Service-Specific Adaptations**

Each service may need slight adaptations:

```bash
# Example: Update AI Analysis service with AI-specific test adaptations
vi 02-aianalysis/testing-strategy.md

# Replace generic examples with service-specific ones:
# - "RemediationProcessing" → "AIAnalysis"
# - "BR-SP-001" → "BR-AI-001"
# - Add service-specific test scenarios
```

**Common Adaptations**:
- **Service Name**: Replace controller/CRD names
- **Business Requirements**: Update BR references
- **Example Code**: Use service-specific examples
- **Test Scenarios**: Add service-unique test cases

#### **5. Update Header Date**

Update the `LAST-UPDATED` date in ALL updated files:

```bash
# Update all common pattern files with new date
for service in 01-signalprocessing 02-aianalysis 03-workflowexecution 04-kubernetesexecutor 05-remediationorchestrator; do
  for file in testing-strategy security-configuration observability-logging metrics-slos; do
    if [ -f ${service}/${file}.md ]; then
      sed -i '' 's/LAST-UPDATED: [0-9-]*/LAST-UPDATED: 2025-01-15/' ${service}/${file}.md
    fi
  done
done
```

#### **6. Validate Changes**

```bash
# Check all files were updated
grep -r "LAST-UPDATED: 2025-01-15" */testing-strategy.md

# Should show all 5 services
```

---

## Adding New Service

### **Step-by-Step Process**

#### **1. Create Service Directory**

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/services/crd-controllers

# Create new service directory
mkdir 06-newservice
```

#### **2. Copy Common Pattern Files**

```bash
# Copy all common pattern files from pilot
for file in testing-strategy security-configuration observability-logging metrics-slos; do
  cp 01-signalprocessing/${file}.md 06-newservice/
done

echo "Common patterns copied to 06-newservice/"
```

#### **3. Create README.md**

Copy and adapt the README template:

```bash
# Copy pilot README as template
cp 01-signalprocessing/README.md 06-newservice/README.md

# Edit with service-specific information
vi 06-newservice/README.md
```

**What to Update in README**:
- Service name and description
- CRD name (e.g., `NewServiceExecution`)
- Controller name (e.g., `NewServiceReconciler`)
- Priority and effort estimates
- Related services table
- Business requirements coverage
- Document index (adjust file list)

#### **4. Create Service-Specific Documents**

```bash
cd 06-newservice

# Create core service-specific files
touch overview.md
touch crd-schema.md
touch controller-implementation.md
touch reconciliation-phases.md
touch finalizers-lifecycle.md
touch database-integration.md
touch integration-points.md
touch migration-current-state.md
touch implementation-checklist.md
```

#### **5. Update Common Pattern Headers**

Add the new service to all common pattern headers:

```bash
# Update common pattern headers to include new service
for file in testing-strategy security-configuration observability-logging metrics-slos; do
  sed -i '' 's/05-remediationorchestrator -->/05-remediationorchestrator, 06-newservice -->/' ${file}.md
done
```

#### **6. Populate Service-Specific Content**

Use pilot service as a template:

```bash
# Copy structure from pilot and adapt
cp 01-signalprocessing/overview.md 06-newservice/overview.md
# Edit with new service details

cp 01-signalprocessing/crd-schema.md 06-newservice/crd-schema.md
# Edit with new CRD structure

# ... repeat for other files
```

#### **7. Update All Existing Services**

Add new service to common pattern headers in ALL existing services:

```bash
for service in 01-signalprocessing 02-aianalysis 03-workflowexecution 04-kubernetesexecutor 05-remediationorchestrator; do
  for file in testing-strategy security-configuration observability-logging metrics-slos; do
    sed -i '' 's/05-remediationorchestrator -->/05-remediationorchestrator, 06-newservice -->/' ${service}/${file}.md
  done
done
```

---

## Service-Specific Updates

### **When to Update Only One Service**

Update only the target service when changes are:
- CRD schema specific
- Controller implementation unique
- Service-specific reconciliation logic
- Unique architectural patterns

### **Example: Update AI Analysis HolmesGPT Integration**

```bash
# Only update AI Analysis service
vi 02-aianalysis/ai-holmesgpt-approval.md

# No need to copy to other services (unique file)
```

### **Files That Are Always Service-Specific**

- `README.md` - Service navigation
- `overview.md` - Service architecture
- `crd-schema.md` - CRD types
- `controller-implementation.md` - Reconciler logic
- `reconciliation-phases.md` - Phase details
- `finalizers-lifecycle.md` - Cleanup patterns
- `database-integration.md` - Audit schema
- `integration-points.md` - Service coordination
- `migration-current-state.md` - Existing code
- `implementation-checklist.md` - APDC phases

---

## Directory Structure Rules

### **Mandatory Files**

Every service MUST have:

1. ✅ `README.md` - Navigation hub
2. ✅ `overview.md` - Architecture
3. ✅ `crd-schema.md` - CRD definition
4. ✅ `controller-implementation.md` - Reconciler
5. ✅ `reconciliation-phases.md` - Phase transitions
6. ✅ `finalizers-lifecycle.md` - Cleanup
7. ✅ `testing-strategy.md` - Test patterns (common)
8. ✅ `security-configuration.md` - Security (common)
9. ✅ `observability-logging.md` - Logging (common)
10. ✅ `metrics-slos.md` - Metrics (common)
11. ✅ `database-integration.md` - Audit
12. ✅ `integration-points.md` - Coordination
13. ✅ `migration-current-state.md` - Existing code
14. ✅ `implementation-checklist.md` - APDC

### **Optional Service-Specific Files**

Services MAY have additional unique files:

- **02-aianalysis/**:
  - `ai-holmesgpt-approval.md` - HolmesGPT & Rego policies

- **04-kubernetesexecutor/**:
  - `predefined-actions.md` - Action catalog

- **05-remediationorchestrator/**:
  - `data-handling-architecture.md` - Targeting Data Pattern

---

## File Naming Conventions

### **Rules**

1. **Lowercase with hyphens**: `controller-implementation.md` (not `Controller_Implementation.md`)
2. **Descriptive names**: `ai-holmesgpt-approval.md` (not `ai-stuff.md`)
3. **No abbreviations**: `reconciliation-phases.md` (not `recon-phases.md`)
4. **Consistent suffixes**: Always `.md` for markdown

### **Special Prefixes**

- `00-` for meta documents (e.g., `00-PILOT-SUMMARY.md`)
- No numeric prefixes for regular documents

---

## Cross-Reference Validation

### **Check Internal Links**

Validate links between documents in the same service:

```bash
cd 01-signalprocessing

# Check all relative links
grep -r "\[.*\](\./" *.md

# Manually verify each link exists
```

### **Check External Links**

Validate links to other services or documentation:

```bash
# Check cross-service references
grep -r "\.\./0[2-5]-" *.md

# Check architecture references
grep -r "docs/architecture" *.md
```

### **Common Link Patterns**

**Within Service**:
```markdown
See [CRD Schema](./crd-schema.md) for details.
```

**To Other Service**:
```markdown
See [01-signalprocessing/testing-strategy.md](../01-signalprocessing/testing-strategy.md) for patterns.
```

**To Architecture Docs**:
```markdown
See [docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
```

---

## Troubleshooting

### **Problem: Inconsistent Common Patterns**

**Symptom**: Common pattern files differ across services

**Solution**:
```bash
# Diff common patterns across services
diff 01-signalprocessing/testing-strategy.md 02-aianalysis/testing-strategy.md

# If differences are unexpected, recopy from pilot
cp 01-signalprocessing/testing-strategy.md 02-aianalysis/
# Then make service-specific adaptations
```

### **Problem: Outdated Common Pattern Headers**

**Symptom**: `LAST-UPDATED` dates don't match

**Solution**:
```bash
# Find all outdated headers
grep -r "LAST-UPDATED: 2025-01-10" */testing-strategy.md

# Bulk update all to latest date
for service in */; do
  for file in testing-strategy security-configuration observability-logging metrics-slos; do
    if [ -f ${service}${file}.md ]; then
      sed -i '' 's/LAST-UPDATED: .*/LAST-UPDATED: 2025-01-15/' ${service}${file}.md
    fi
  done
done
```

### **Problem: Missing README Navigation**

**Symptom**: Service directory has no README

**Solution**:
```bash
# Copy pilot README as template
cp 01-signalprocessing/README.md 06-newservice/README.md

# Update service-specific details
vi 06-newservice/README.md
```

### **Problem: Broken Cross-References**

**Symptom**: Links return 404 or point to wrong files

**Solution**:
```bash
# Find all markdown links in a service
cd 01-signalprocessing
grep -r "\[.*\](.*\.md)" *.md

# Verify each link exists
for link in $(grep -oh "\(\.\/[^)]*\.md\)" *.md | tr -d '()'); do
  if [ ! -f "$link" ]; then
    echo "BROKEN: $link"
  fi
done
```

---

## Quick Reference

### **Update Common Pattern**
```bash
# 1. Update pilot
vi 01-signalprocessing/testing-strategy.md

# 2. Copy to others
for s in 02-aianalysis 03-workflowexecution 04-kubernetesexecutor 05-remediationorchestrator; do
  cp 01-signalprocessing/testing-strategy.md $s/
done

# 3. Adapt each service
# (manually edit service-specific examples)

# 4. Update headers
sed -i '' 's/LAST-UPDATED: .*/LAST-UPDATED: 2025-01-15/' */testing-strategy.md
```

### **Add New Service**
```bash
# 1. Create directory
mkdir 06-newservice

# 2. Copy common patterns
for f in testing-strategy security-configuration observability-logging metrics-slos; do
  cp 01-signalprocessing/$f.md 06-newservice/
done

# 3. Copy README template
cp 01-signalprocessing/README.md 06-newservice/

# 4. Create service-specific files
cd 06-newservice
touch overview.md crd-schema.md controller-implementation.md ...

# 5. Update all common pattern headers
# (add 06-newservice to SERVICES list)
```

### **Validate Structure**
```bash
# Check all services have required files
for service in 0*/; do
  echo "=== $service ==="
  ls -1 ${service}README.md ${service}overview.md ${service}crd-schema.md 2>/dev/null | wc -l
  echo "Found files (should be 3+)"
done
```

---

## Best Practices

### **DO** ✅
- Update common patterns in pilot first
- Keep headers updated with latest date
- Test changes before copying to all services
- Make service-specific adaptations explicit
- Validate cross-references after changes

### **DON'T** ❌
- Update common patterns in multiple services independently
- Forget to update `LAST-UPDATED` date
- Copy pilot verbatim without adaptations
- Create new common patterns without marking them
- Break cross-references when renaming files

---

## Maintenance Checklist

**Before Committing Changes**:
- [ ] Common pattern updated in pilot first
- [ ] Common pattern copied to all services
- [ ] Service-specific adaptations made
- [ ] Headers updated with latest date
- [ ] Cross-references validated
- [ ] README navigation checked
- [ ] No broken links

---

## Support

**Questions?** See:
- [RESTRUCTURE_COMPLETE.md](./RESTRUCTURE_COMPLETE.md) - Full restructure documentation
- [CRD Service Specification Template](../../development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md) - Service template
- [01-signalprocessing/README.md](./01-signalprocessing/README.md) - Example service navigation

**Last Updated**: 2025-01-15
**Maintained By**: Kubernaut Documentation Team

