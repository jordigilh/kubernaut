# Gateway Service - BR Prefix Standardization Complete

**Date**: October 6, 2025
**Status**: ✅ **COMPLETED**

---

## Summary

Gateway Service previously used **FOUR different BR prefixes** (BR-WH-*, BR-ENV-*, BR-GITOPS-*, BR-NOT-*).

All references have been **updated to BR-GATEWAY-*** (BR-GATEWAY-001 to BR-GATEWAY-180) for consistency with other services.

---

## Quick Reference Mapping

### **Alert Ingestion (001-023)**

| Old BR | New BR | Description |
|--------|--------|-------------|
| BR-WH-001 | BR-GATEWAY-001 | Alert ingestion endpoint |
| BR-WH-002 | BR-GATEWAY-002 | Prometheus adapter |
| BR-WH-005 | BR-GATEWAY-005 | Kubernetes event adapter |
| BR-WH-006 | BR-GATEWAY-006 | Alert normalization |
| BR-WH-010 | BR-GATEWAY-010 | Fingerprint-based deduplication |
| BR-WH-011 | BR-GATEWAY-011 | Redis deduplication storage |
| BR-WH-015 | BR-GATEWAY-015 | Alert storm detection |
| BR-WH-016 | BR-GATEWAY-016 | Storm aggregation |
| BR-WH-020 | BR-GATEWAY-020 | Priority assignment (Rego) |
| BR-WH-021 | BR-GATEWAY-021 | Priority fallback matrix |
| BR-WH-022 | BR-GATEWAY-022 | Remediation path decision |
| BR-WH-023 | BR-GATEWAY-023 | CRD creation |

### **Environment Classification (051-053)**

| Old BR | New BR | Description |
|--------|--------|-------------|
| BR-ENV-001 | BR-GATEWAY-051 | Environment detection (namespace labels) |
| BR-ENV-010 | BR-GATEWAY-052 | ConfigMap fallback for environment |
| BR-ENV-020 | BR-GATEWAY-053 | Default environment (unknown) |

### **GitOps Integration (071-072)**

| Old BR | New BR | Description |
|--------|--------|-------------|
| BR-GITOPS-001 | BR-GATEWAY-071 | CRD-only integration (no direct GitOps) |
| BR-GITOPS-014 | BR-GATEWAY-072 | CRD as GitOps trigger |

### **Downstream Notification (091-092)**

| Old BR | New BR | Description |
|--------|--------|-------------|
| BR-NOT-026 | BR-GATEWAY-091 | Escalation notification trigger |
| BR-NOT-037 | BR-GATEWAY-092 | Notification metadata |

---

## Migration Complete

**No backwards compatibility maintained.**

All documentation now uses **BR-GATEWAY-*** exclusively.

**Reserved Range**: BR-GATEWAY-024 to BR-GATEWAY-180 for future Gateway features.

---

**Document Maintainer**: Kubernaut Documentation Team
**Completed**: October 6, 2025

