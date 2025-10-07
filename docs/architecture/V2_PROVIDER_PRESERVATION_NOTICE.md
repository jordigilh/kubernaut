# V2 Provider Structures - Preservation Notice

**Date**: October 5, 2025
**Purpose**: Protect V2 provider structures from accidental deletion during code triages
**Authority**: Architectural decision - Alternative 1 Multi-Provider Design

---

## 🚨 CRITICAL NOTICE FOR CODE TRIAGES

**DO NOT DELETE** the following provider structures during unused code cleanup:

### AWS Provider Structures ⏸️ V2
- `AWSProviderData` struct
- `buildAWSProviderData()` function
- AWS-related fields in `NormalizedSignal`
- AWS adapter code (if present)
- AWS provider data validation

### Azure Provider Structures ⏸️ V2
- `AzureProviderData` struct
- `buildAzureProviderData()` function (if present)
- Azure-related fields in `NormalizedSignal`
- Azure adapter code (if present)
- Azure provider data validation

### Datadog Provider Structures ⏸️ V2
- `DatadogProviderData` struct
- `buildDatadogProviderData()` function
- Datadog-related fields in `NormalizedSignal`
- Datadog adapter code (if present)
- Datadog provider data validation

### GCP Provider Structures ⏸️ V2
- `GCPProviderData` struct
- `buildGCPProviderData()` function (if present)
- GCP-related fields in `NormalizedSignal`
- GCP adapter code (if present)
- GCP provider data validation

---

## 📋 V1 vs V2 Scope

### V1 Implementation (Current)
**Active Providers**:
- ✅ Kubernetes (Prometheus alerts)
- ✅ Kubernetes (Kubernetes events)

**Target Type**: `"kubernetes"` only

**Provider Data**: Kubernetes provider data only

---

### V2 Implementation (Planned)
**Additional Providers** (Schema complete, code preserved):
- ⏸️ AWS (CloudWatch alerts)
- ⏸️ Azure (Azure Monitor alerts)
- ⏸️ Datadog (Datadog monitors)
- ⏸️ GCP (Cloud Monitoring alerts)

**Target Types**: `"kubernetes"`, `"aws"`, `"azure"`, `"datadog"`, `"gcp"`

**Provider Data**: All provider data schemas documented and ready

---

## 🎯 Why These Structures Exist

### Architectural Decision
**Alternative 1 - Raw JSON Provider Data** (90% confidence, approved)

**Design Principle**: Create schema and structures that support all providers **without requiring CRD version bumps**

**Implementation Strategy**:
1. **V1**: Implement Kubernetes provider only
2. **V2**: Add AWS, Azure, Datadog, GCP providers using existing schema
3. **No Schema Changes**: Same `RemediationRequest` CRD supports all providers

**Benefit**: Forward-looking architecture that avoids future breaking changes

---

## 🔍 How to Identify V2 Structures

### Documentation Markers
Look for these markers in code and documentation:

```go
// ⏸️ V2 - Planned
// 🚨 DO NOT DELETE: This structure is valid and preserved for V2
type AWSProviderData struct {
    // ...
}
```

### File Comments
```go
// pkg/gateway/provider_data.go
// buildAWSProviderData creates AWS-specific provider data
// Status: V2 - Planned, structure preserved for future implementation
// DO NOT DELETE during unused code cleanup
func buildAWSProviderData(signal *NormalizedSignal) json.RawMessage {
    // ...
}
```

### Documentation References
- `docs/architecture/CRD_SCHEMAS.md` - Authoritative provider schemas
- All non-Kubernetes providers marked as "⏸️ V2 - Planned"
- Explicit "DO NOT DELETE" warnings

---

## ✅ What CAN Be Deleted

### Unused Code (Safe to Delete)
- ❌ Experimental code not part of design
- ❌ Duplicate implementations
- ❌ Deprecated approaches (marked as "DEPRECATED")
- ❌ Test fixtures not referenced in tests
- ❌ Old prototype code

### V2 Structures (DO NOT DELETE)
- ✅ AWS, Azure, Datadog, GCP provider data structs
- ✅ Builder functions for non-K8s providers
- ✅ Non-K8s adapter code (if present)
- ✅ Provider-specific validation logic

---

## 📊 V2 Provider Implementation Checklist

**When V2 implementation begins**, these structures are ready:

### AWS CloudWatch
- [x] Provider data schema documented
- [x] Go helper type defined (`AWSProviderData`)
- [x] Builder function documented (`buildAWSProviderData`)
- [x] Field reference documented
- [ ] Adapter implementation (V2)
- [ ] Integration tests (V2)

### Azure Monitor
- [x] Provider data schema documented
- [x] Go helper type defined (`AzureProviderData`)
- [x] Builder function conceptual design
- [x] Field reference documented
- [ ] Adapter implementation (V2)
- [ ] Integration tests (V2)

### Datadog
- [x] Provider data schema documented
- [x] Go helper type defined (`DatadogProviderData`)
- [x] Builder function documented (`buildDatadogProviderData`)
- [x] Field reference documented
- [ ] Adapter implementation (V2)
- [ ] Integration tests (V2)

### GCP Cloud Monitoring
- [x] Provider data schema documented
- [x] Go helper type defined (`GCPProviderData`)
- [x] Builder function conceptual design
- [x] Field reference documented
- [ ] Adapter implementation (V2)
- [ ] Integration tests (V2)

---

## 🛡️ Protection Mechanisms

### 1. Documentation Warnings
- Explicit "DO NOT DELETE" in schema docs
- "V2 - Planned" status markers
- Preservation notices in this document

### 2. Code Comments
```go
// IMPORTANT: V2 Provider Structure - DO NOT DELETE
// This code is intentionally present for V2 multi-provider support
// See: docs/architecture/V2_PROVIDER_PRESERVATION_NOTICE.md
```

### 3. Test Coverage (When Implemented)
```go
// Test exists even if provider not active
func TestAWSProviderDataBuilder_V2Planned(t *testing.T) {
    // Ensures structure doesn't break during V1
}
```

### 4. Linter Exceptions
```yaml
# .golangci.yml or similar
issues:
  exclude-rules:
    - path: provider_data.go
      linters:
        - unused  # V2 provider structures preserved
      text: "AWSProviderData.*is unused"
```

---

## 📚 Related Documentation

**Authoritative Schema**:
- `docs/architecture/CRD_SCHEMAS.md`

**Alternative Analysis**:
- `docs/architecture/MULTI_PROVIDER_CRD_ALTERNATIVES.md`

**Implementation Status**:
- `docs/services/ALTERNATIVE_1_IMPLEMENTATION_COMPLETE.md`

**Gateway Integration**:
- `docs/services/stateless/gateway-service/crd-integration.md`

---

## 🎯 Summary

### DO NOT DELETE
- ✅ AWS provider structures
- ✅ Azure provider structures
- ✅ Datadog provider structures
- ✅ GCP provider structures
- ✅ Non-Kubernetes provider builder functions
- ✅ Provider-specific validation logic

### CAN DELETE
- ❌ Truly unused experimental code
- ❌ Deprecated implementations
- ❌ Old prototypes not part of current design

### V1 Implementation
- ✅ Kubernetes provider only (active)
- ⏸️ Non-Kubernetes providers (schema ready, code preserved)

### V2 Implementation
- ⏸️ All provider structures ready
- ⏸️ Schema supports all providers
- ⏸️ No CRD version bump required

---

**Authority**: Approved architectural decision (Alternative 1, 90% confidence)
**Status**: Active preservation policy
**Review**: This document should be referenced during all code triages and cleanup operations
