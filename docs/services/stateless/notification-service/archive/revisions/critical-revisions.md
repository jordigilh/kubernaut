# ⚠️ DEPRECATED: Critical Revisions Document

**Original Date**: 2025-10-03  
**Deprecation Date**: 2025-10-07  
**Status**: 📦 **ARCHIVED - CONTENT DISTRIBUTED**

---

## 🔄 Content Has Been Distributed

This document contained two critical architectural decisions that have been **extracted and distributed** to appropriate locations for better discoverability and maintenance:

---

### **CRITICAL-1: RBAC Permission Filtering → ADR-014**

**Original Content**: Lines 8-176 (RBAC architectural mistake and correction)

**Now Located At**:  
📄 **[ADR-014: Notification Service Uses External Service Authentication](../../../../architecture/decisions/ADR-014-notification-service-external-auth.md)**

**Summary**: 
- ❌ **Removed**: RBAC pre-filtering of notification actions (~500 lines of code)
- ✅ **Added**: External service authentication delegation
- **Impact**: 50ms faster notifications, simpler architecture, better separation of concerns

**Why Moved**: Major architectural decisions should be documented as Architecture Decision Records (ADRs) for visibility and traceability.

---

### **CRITICAL-3: Secret Mounting Strategy → Security Configuration**

**Original Content**: Lines 180-476 (Secret mounting options and deployment configuration)

**Now Located At**:  
📄 **[Security Configuration - Secret Management Strategy](../../security-configuration.md#secure-secret-management)**

**Summary**:
- ✅ **V1 Decision**: Use Kubernetes Projected Volumes (Option 3) - Security Score 9.5/10
- ⏳ **V2 Migration**: Add External Secrets + Vault when centralized secret management needed
- **Impact**: Excellent security with zero external dependencies

**Why Moved**: Deployment and security decisions belong in service-specific security configuration documentation where operators expect to find them.

---

### **Design Evolution Summary → Service Overview**

**Original Content**: Historical context of both critical decisions

**Now Located At**:  
📄 **[Notification Service Overview - Design Evolution](../../overview.md#design-evolution)**

**Summary**: Complete history of major architectural revisions with impact analysis and cross-references to detailed documentation.

**Why Moved**: Design evolution context helps future maintainers understand current architecture but doesn't need to be in a separate archive file.

---

## 📚 Complete Distribution Map

| Original Section | Lines | New Location | Document Type |
|------------------|-------|--------------|---------------|
| **CRITICAL-1: RBAC Removal** | 8-176 | [ADR-014](../../../../architecture/decisions/ADR-014-notification-service-external-auth.md) | Architecture Decision Record |
| **CRITICAL-3: Secret Mounting** | 180-476 | [security-configuration.md](../../security-configuration.md#secure-secret-management) | Service Security Documentation |
| **Summary & Impact** | Various | [overview.md](../../overview.md#design-evolution) | Service Overview |

---

## 🎯 Why This Document Was Deprecated

### **Before (Archive)**
- Hidden in `archive/revisions/` directory (low discoverability)
- Single monolithic document (mixed concerns)
- Not following ADR pattern for architectural decisions
- Hard to find when searching for security or architecture docs

### **After (Distributed)**
- Architecture decisions in standard ADR location (`docs/architecture/decisions/`)
- Security configuration in service security docs (where operators expect it)
- Design history in service overview (appropriate context)
- Each concern in its proper location (single responsibility)

### **Benefits of Distribution**
✅ **Improved Discoverability**: Architecture decisions visible in ADR directory  
✅ **Better Organization**: Each concern in appropriate location  
✅ **Follows Standards**: ADR pattern for architectural decisions  
✅ **Operator-Friendly**: Security config where it's expected  
✅ **Maintainability**: Easier to update individual concerns  

---

## 🔗 Quick Navigation

Need information about notification service decisions? Use these links:

### **Architecture & Design Decisions**
- [ADR-014: External Service Authentication](../../../../architecture/decisions/ADR-014-notification-service-external-auth.md)
- [All Architecture Decisions](../../../../architecture/decisions/README.md)

### **Security Configuration**
- [Secret Management Strategy](../../security-configuration.md#secure-secret-management)
- [Complete Security Configuration](../../security-configuration.md)

### **Service Documentation**
- [Notification Service Overview](../../overview.md)
- [Design Evolution History](../../overview.md#design-evolution)
- [API Specification](../../api-specification.md)
- [Integration Points](../../integration-points.md)

### **Archive Documentation** (Historical Reference)
- [Original Service Triage](../triage/service-triage.md) - Issue identification
- [Critical Issues Solutions](../solutions/critical-issues.md) - Original solutions
- [All Issues Summary](../summaries/all-issues-complete.md) - Complete resolution

---

## ⚠️ Do Not Use This File

**This file is deprecated and should not be referenced or updated.**

If you need to:
- **Understand RBAC decision**: Read [ADR-014](../../../../architecture/decisions/ADR-014-notification-service-external-auth.md)
- **Configure secrets**: Read [security-configuration.md](../../security-configuration.md#secure-secret-management)
- **Learn design history**: Read [overview.md](../../overview.md#design-evolution)
- **Update architecture**: Create new ADR in `docs/architecture/decisions/`
- **Update security**: Edit `security-configuration.md`

---

## 📊 Distribution Metadata

**Distribution Date**: 2025-10-07  
**Distribution Method**: Content extraction and reorganization  
**Validation**: ✅ All cross-references updated  
**Confidence**: 95% - Standard documentation reorganization  

**Files Created/Updated**:
1. ✅ Created: `docs/architecture/decisions/ADR-014-notification-service-external-auth.md`
2. ✅ Updated: `docs/services/stateless/notification-service/security-configuration.md`
3. ✅ Updated: `docs/services/stateless/notification-service/overview.md`
4. ✅ Deprecated: This file

---

**Last Updated**: 2025-10-07  
**Status**: 📦 Archived - Content Distributed  
**Maintainer**: Kubernaut Documentation Team