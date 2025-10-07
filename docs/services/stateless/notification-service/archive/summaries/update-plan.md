# Notification Service Document - Update Plan

**Date**: 2025-10-03
**Status**: 📝 **READY TO EXECUTE**

---

## 🎯 **UPDATE OBJECTIVES**

1. ✅ Remove all RBAC permission filtering logic (BR-NOT-037 corrected)
2. ✅ Confirm Option 3 (Projected Volume) for secret mounting (CRITICAL-3)
3. ✅ Add comprehensive solutions for all remaining CRITICAL issues

---

## 📋 **CHANGES REQUIRED**

### **1. Update BR-NOT-037 References** (Lines 47, 72, 76, 84)

**Find and Replace**:
```
OLD: BR-NOT-037: MUST filter notification actions based on recipient RBAC permissions
NEW: BR-NOT-037: MUST provide action links to external services for all recommended actions
```

**Impact**: ~4-5 occurrences throughout the document

---

### **2. Remove RBAC Filtering from Overview** (Lines 69-99)

**Remove**:
- Line 76: "RBAC Permission Filtering (BR-NOT-037): Query recipient permissions and show only actionable buttons"
- Line 84: "Recipient RBAC permission filtering (K8s + Git provider integration)"

**Replace With**:
- "External Service Action Links (BR-NOT-037): Provide direct links to external services for recommended actions"
- "Authentication delegation to external services (GitHub, GitLab, Grafana, K8s Dashboard)"

---

### **3. Remove RBAC-Related Sections** (Search for "RBAC", "Permission", "Recipient Mapping")

**Sections to Remove/Update**:
1. RBAC Permission Filtering architecture section
2. RecipientMapper interface
3. K8s SubjectAccessReview checks
4. Git provider permission checks
5. Permission caching logic
6. RBAC filtering in service flow

**Replace With**:
- Link Generation section (how to generate GitHub PR links, Grafana links, etc.)
- External Service Configuration (base URLs for external services)
- Authentication delegation explanation

---

### **4. Add CRITICAL Issue Solutions**

#### **CRITICAL-2: Error Handling & Retry Logic**
**Add Section**: "Error Handling and Resilience Patterns"
- Retry Policy with exponential backoff
- Circuit Breaker per channel
- Fallback channel strategy
- Code examples for retry executor and circuit breaker

#### **CRITICAL-3: Secret Mounting**
**Add Section**: "Secret Management and Configuration"
- Option 3: Projected Volume with ServiceAccount Token (CONFIRMED)
- Deployment manifest with projected volume
- Application code for loading secrets from files
- Security best practices (read-only root FS, file permissions)

#### **CRITICAL-4: Channel Adapter Robustness**
**Add Section**: "Channel Adapter Implementation"
- Tiered payload strategy (summary + link for oversized payloads)
- Rate limiting per channel (Slack: 1 msg/sec)
- Graceful degradation for payload size limits
- Code examples for SlackAdapter with payload strategies

#### **CRITICAL-6: Template Management**
**Add Section**: "Template Loading and Hot Reload"
- ConfigMap-based template storage
- Hot reload mechanism (watch ConfigMap every 30s)
- Template validation on startup
- Fallback template rendering
- Code examples for TemplateManager

#### **CRITICAL-7: API Authentication**
**Add Section**: "API Authentication via OAuth2 JWT"
- OAuth2 JWT from Kubernetes ServiceAccount tokens
- TokenReview API validation
- CRD controllers use Bearer token from projected volume
- RBAC configuration (no additional permissions needed)
- Code examples for auth middleware and controller usage

#### **CRITICAL-8: Observability**
**Add Section**: "Distributed Tracing and Observability"
- OpenTelemetry tracing integration
- Structured logging with correlation IDs
- Audit event emission
- Metrics + Tracing + Logging correlation
- Code examples for tracing service and audit events

---

### **5. Update Testing Strategy**

**Update**:
- Remove RBAC permission mocking tests
- Add link generation validation tests
- Update integration tests to use EphemeralNotifier (already done)
- Update E2E tests with Gitea for Git provider (add reference to Gitea E2E strategy)

---

### **6. Update Example Notifications**

**Remove**: RBAC filtering examples showing "hidden actions"

**Add**: Complete action link examples:
```
Recommended Actions:
1. 📊 View Logs → https://grafana.company.com/logs?pod=webapp
2. 🔄 Restart Pod → https://k8s-dashboard.company.com/pods/webapp/restart
3. 📝 Approve PR → https://github.com/company/manifests/pull/123
```

**Explanation**: All actions shown, external services handle authentication

---

### **7. Update Metrics**

**Remove**: RBAC-related metrics (permission checks, filtering decisions)

**Keep**: Channel delivery metrics, sanitization metrics, freshness metrics

**Add**: Link generation metrics

---

### **8. Update Package Structure**

**Remove**:
```
pkg/notification/
  ├── rbac/           # ❌ REMOVE
  │   ├── checker.go
  │   ├── k8s.go
  │   └── git.go
```

**Add**:
```
pkg/notification/
  ├── links/          # ✅ ADD
  │   ├── generator.go   # Generate GitHub/Grafana/K8s links
  │   ├── config.go      # Base URLs for external services
  │   └── types.go
  ├── retry/          # ✅ ADD (CRITICAL-2)
  │   ├── policy.go
  │   └── circuit_breaker.go
  ├── config/         # ✅ ADD (CRITICAL-3)
  │   ├── loader.go
  │   └── validator.go
  ├── templates/      # ✅ ENHANCE (CRITICAL-6)
  │   └── manager.go  # Hot reload from ConfigMap
  ├── auth/           # ✅ ADD (CRITICAL-7)
  │   └── middleware.go
  ├── observability/  # ✅ ADD (CRITICAL-8)
  │   ├── tracing.go
  │   └── audit.go
```

---

### **9. Update Business Logic Flow**

**OLD Flow**:
```
1. Receive notification request
2. Sanitize payload
3. Query RBAC permissions ← REMOVE
4. Filter actions based on permissions ← REMOVE
5. Format for channel
6. Deliver notification
```

**NEW Flow**:
```
1. Receive notification request
2. Sanitize payload
3. Generate action links (GitHub, Grafana, K8s) ← ADD
4. Format for channel
5. Deliver notification with retry + circuit breaker
6. Emit observability events (tracing, logging, metrics)
```

---

### **10. Update Implementation Checklist**

**Remove**:
- Implement RBAC checker
- Implement RecipientMapper
- Implement permission caching

**Add**:
- Implement link generator (GitHub, GitLab, Grafana, K8s Dashboard, Prometheus)
- Implement retry policy and circuit breaker (CRITICAL-2)
- Implement secret loading from projected volume (CRITICAL-3)
- Implement tiered payload strategy (CRITICAL-4)
- Implement template hot reload (CRITICAL-6)
- Implement OAuth2 JWT auth middleware (CRITICAL-7)
- Implement OpenTelemetry tracing (CRITICAL-8)

---

## 📊 **IMPACT ASSESSMENT**

**Lines to Change**: ~300-400 lines (out of 2583 total)
**New Code Examples**: ~800-1000 lines (for CRITICAL solutions)
**Final Document Size**: ~3200-3400 lines

**Complexity Reduction**:
- 🟢 ~500 lines of RBAC code **REMOVED**
- 🟢 ~1000 lines of resilience/observability code **ADDED**
- 🟢 Net result: Simpler architecture with production-ready patterns

**Confidence**: **95%** that these changes improve architectural quality

---

## ✅ **EXECUTION STRATEGY**

Due to file size (2583 lines), we'll execute updates in batches:

### **Batch 1: BR-NOT-037 Updates** (5-10 replacements)
- Find all references to old BR-NOT-037
- Replace with new BR-NOT-037 description

### **Batch 2: Remove RBAC Sections** (3-5 sections)
- Remove RBAC filtering architecture
- Remove permission checking code examples
- Remove RecipientMapper interfaces

### **Batch 3: Add CRITICAL Solutions** (6 new sections)
- Add Error Handling section (CRITICAL-2)
- Add Secret Management section (CRITICAL-3)
- Add Channel Adapter section (CRITICAL-4)
- Add Template Management section (CRITICAL-6)
- Add API Authentication section (CRITICAL-7)
- Add Observability section (CRITICAL-8)

### **Batch 4: Update Examples** (10-15 updates)
- Update notification examples
- Update metrics examples
- Update testing strategy

### **Batch 5: Final Cleanup** (package structure, checklist)
- Update package structure diagram
- Update implementation checklist
- Update architectural overview

---

## 🎯 **READY TO EXECUTE**

**Approval**: Awaiting user confirmation to proceed with updates

**Estimated Time**: 15-20 targeted file updates

