# BR-REMEDIATION-015: Populate Incident Type on Audit Creation

**Business Requirement ID**: BR-REMEDIATION-015
**Category**: RemediationExecutor Service
**Priority**: P0
**Target Version**: V1
**Status**: ✅ Approved
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 introduces **Multi-Dimensional Success Tracking** with `incident_type` as the PRIMARY dimension. The RemediationExecutor Service must populate the `incident_type`, `alert_name`, and `incident_severity` fields when creating audit records in the Data Storage Service.

**Current Limitations**:
- ❌ RemediationExecutor creates audit records without `incident_type` field
- ❌ Data Storage schema has `incident_type` column but it remains NULL
- ❌ Cannot calculate success rates by incident type (BR-STORAGE-031-01 blocked)
- ❌ AI cannot learn from historical incident-type effectiveness
- ❌ No classification of remediation actions by incident type

**Impact**:
- BR-STORAGE-031-01 (Incident-Type Success Rate API) cannot function with NULL incident_type
- AI cannot make data-driven playbook selections based on incident type
- No foundation for ADR-033 multi-dimensional tracking
- Historical audit records lack critical classification dimension

---

## 🎯 **Business Objective**

**Ensure RemediationExecutor populates incident_type, alert_name, and incident_severity fields in all audit records to enable incident-type-based success tracking.**

### **Success Criteria**
1. ✅ RemediationExecutor extracts incident_type from incoming signal/alert
2. ✅ RemediationExecutor populates `incident_type` field (REQUIRED) in audit records
3. ✅ RemediationExecutor populates `alert_name` field (OPTIONAL) in audit records
4. ✅ RemediationExecutor populates `incident_severity` field (REQUIRED) in audit records
5. ✅ 100% of new audit records have non-null `incident_type` and `incident_severity`
6. ✅ Incident type classification follows standardized naming convention
7. ✅ Backward compatible: existing audit records with NULL incident_type do not break queries

---

## 📊 **Use Cases**

### **Use Case 1: Extract Incident Type from Prometheus Alert**

**Scenario**: RemediationExecutor receives a Prometheus alert for `pod-oom-killer` and creates an audit record.

**Current Flow** (Without BR-REMEDIATION-015):
```
1. RemediationExecutor receives Prometheus alert:
   {
     "alertname": "PodOOMKiller",
     "severity": "critical",
     "namespace": "production",
     "pod": "webapp-6789"
   }
2. RemediationExecutor executes remediation action (increase_memory)
3. RemediationExecutor creates audit record:
   {
     "action_id": "act-12345",
     "action_type": "increase_memory",
     "incident_type": null,  ← ❌ NULL
     "alert_name": null,     ← ❌ NULL
     "incident_severity": null ← ❌ NULL
   }
4. ❌ Data Storage cannot aggregate by incident type
5. ❌ AI cannot learn pod-oom-killer remediation effectiveness
```

**Desired Flow with BR-REMEDIATION-015**:
```
1. RemediationExecutor receives Prometheus alert (same as above)
2. RemediationExecutor extracts incident classification:
   - incident_type = "pod-oom-killer" (normalized from "PodOOMKiller")
   - alert_name = "PodOOMKiller" (original Prometheus alert name)
   - incident_severity = "critical" (from alert labels)
3. RemediationExecutor executes remediation action
4. RemediationExecutor creates audit record:
   {
     "action_id": "act-12345",
     "action_type": "increase_memory",
     "incident_type": "pod-oom-killer",  ← ✅ POPULATED
     "alert_name": "PodOOMKiller",       ← ✅ POPULATED
     "incident_severity": "critical"     ← ✅ POPULATED
   }
5. ✅ Data Storage can aggregate by incident type
6. ✅ AI can query pod-oom-killer success rates
7. ✅ Foundation for data-driven playbook selection
```

---

### **Use Case 2: Handle Missing Incident Type with Default Classification**

**Scenario**: RemediationExecutor receives a signal without explicit incident type (e.g., manual remediation request).

**Current Flow**:
```
1. RemediationExecutor receives manual remediation request
2. No incident_type field in request
3. ❌ RemediationExecutor creates audit with NULL incident_type
4. ❌ Audit record excluded from incident-type aggregations
```

**Desired Flow with BR-REMEDIATION-015**:
```
1. RemediationExecutor receives manual remediation request
2. No incident_type in request
3. RemediationExecutor applies default classification:
   - incident_type = "manual-remediation" (default for manual requests)
   - alert_name = "" (empty for manual)
   - incident_severity = "unknown" (default)
4. RemediationExecutor creates audit with default values
5. ✅ Audit record included in aggregations (as "manual-remediation" category)
6. ✅ No NULL incident_type records
7. ✅ Distinguishes manual vs automated remediation in analytics
```

---

### **Use Case 3: Normalize Incident Type Naming**

**Scenario**: Different alert sources use different naming conventions (e.g., "PodOOMKiller" vs "pod_oom_killer" vs "Pod OOM Killer").

**Current Flow**:
```
1. Alert source A sends "PodOOMKiller"
2. Alert source B sends "pod_oom_killer"
3. Alert source C sends "Pod OOM Killer"
4. ❌ Three different incident_type values for same incident
5. ❌ Fragmented success rate data (not aggregated together)
```

**Desired Flow with BR-REMEDIATION-015**:
```
1. RemediationExecutor receives alerts from 3 sources
2. RemediationExecutor normalizes incident_type:
   - "PodOOMKiller" → "pod-oom-killer"
   - "pod_oom_killer" → "pod-oom-killer"
   - "Pod OOM Killer" → "pod-oom-killer"
3. All audits use consistent incident_type = "pod-oom-killer"
4. ✅ Aggregated success rate data (single incident type)
5. ✅ AI learns from unified historical data
```

---

## 🔧 **Functional Requirements**

### **FR-REMEDIATION-015-01: Incident Type Extraction Logic**

**Requirement**: RemediationExecutor SHALL extract incident_type from incoming signals using a priority-based extraction strategy.

**Extraction Priority**:
1. **Explicit field**: If signal has `incident_type` field, use it directly
2. **Prometheus alertname**: Extract from `alertname` label and normalize
3. **Signal type**: Derive from signal metadata (e.g., "cpu-throttling" from CPU metrics)
4. **Default fallback**: Use "unknown-incident" if no classification possible

**Implementation Example**:
```go
package remediationexecutor

// ExtractIncidentType extracts and normalizes incident type from signal
func ExtractIncidentType(signal *Signal) string {
    // Priority 1: Explicit incident_type field
    if signal.IncidentType != "" {
        return NormalizeIncidentType(signal.IncidentType)
    }

    // Priority 2: Prometheus alertname
    if signal.Labels != nil && signal.Labels["alertname"] != "" {
        return NormalizeAlertName(signal.Labels["alertname"])
    }

    // Priority 3: Signal type derivation
    if incidentType := DeriveFromSignalType(signal); incidentType != "" {
        return incidentType
    }

    // Priority 4: Default fallback
    return "unknown-incident"
}

// NormalizeIncidentType converts to lowercase-hyphen format
func NormalizeIncidentType(raw string) string {
    // "PodOOMKiller" → "pod-oom-killer"
    // "pod_oom_killer" → "pod-oom-killer"
    // "Pod OOM Killer" → "pod-oom-killer"
    normalized := strings.ToLower(raw)
    normalized = strings.ReplaceAll(normalized, "_", "-")
    normalized = strings.ReplaceAll(normalized, " ", "-")
    return normalized
}
```

**Acceptance Criteria**:
- ✅ Extracts incident_type from explicit field (highest priority)
- ✅ Falls back to alertname extraction if explicit field missing
- ✅ Normalizes to lowercase-hyphen format ("pod-oom-killer")
- ✅ Returns "unknown-incident" for unclassifiable signals
- ✅ Never returns empty string or null

---

### **FR-REMEDIATION-015-02: Incident Severity Mapping**

**Requirement**: RemediationExecutor SHALL extract and normalize incident_severity from signal labels.

**Severity Mapping**:
```go
// Standard severity levels (aligned with Prometheus)
const (
    SeverityCritical = "critical"  // Immediate action required
    SeverityWarning  = "warning"   // Degraded performance
    SeverityInfo     = "info"      // Informational only
    SeverityUnknown  = "unknown"   // Cannot determine
)

// ExtractSeverity extracts severity from signal labels
func ExtractSeverity(signal *Signal) string {
    // Priority 1: Explicit severity label
    if severity := signal.Labels["severity"]; severity != "" {
        return NormalizeSeverity(severity)
    }

    // Priority 2: Infer from alertname (critical alerts have "Critical" suffix)
    if strings.Contains(signal.Labels["alertname"], "Critical") {
        return SeverityCritical
    }

    // Priority 3: Default to unknown
    return SeverityUnknown
}

// NormalizeSeverity converts to standard values
func NormalizeSeverity(raw string) string {
    switch strings.ToLower(raw) {
    case "critical", "crit", "emergency", "alert":
        return SeverityCritical
    case "warning", "warn":
        return SeverityWarning
    case "info", "informational", "notice":
        return SeverityInfo
    default:
        return SeverityUnknown
    }
}
```

**Acceptance Criteria**:
- ✅ Maps to 4 standard severity levels (critical, warning, info, unknown)
- ✅ Handles common Prometheus severity labels
- ✅ Returns "unknown" for unrecognized severities
- ✅ Never returns empty string or null

---

### **FR-REMEDIATION-015-03: Audit Record Population**

**Requirement**: RemediationExecutor SHALL populate incident classification fields when creating audit records via Data Storage REST API.

**Implementation Example**:
```go
// CreateNotificationAudit sends audit record to Data Storage Service
func (r *RemediationExecutor) CreateNotificationAudit(ctx context.Context, signal *Signal, action *Action) error {
    audit := &datastorage.NotificationAudit{
        ActionID:        action.ID,
        ActionType:      action.Type,
        ActionTimestamp: time.Now(),
        Status:          action.Status,

        // ADR-033: DIMENSION 1 (INCIDENT TYPE) - BR-REMEDIATION-015
        IncidentType:     ExtractIncidentType(signal),        // REQUIRED
        AlertName:        ExtractAlertName(signal),           // OPTIONAL
        IncidentSeverity: ExtractSeverity(signal),            // REQUIRED

        // ... other fields ...
    }

    // Validate incident_type is non-empty
    if audit.IncidentType == "" {
        return fmt.Errorf("incident_type cannot be empty (BR-REMEDIATION-015 validation)")
    }

    // Send to Data Storage Service
    return r.dataStorageClient.CreateNotificationAudit(ctx, audit)
}
```

**Acceptance Criteria**:
- ✅ `incident_type` field is always non-empty (REQUIRED)
- ✅ `incident_severity` field is always non-empty (REQUIRED)
- ✅ `alert_name` field may be empty for non-alert signals (OPTIONAL)
- ✅ Validation error if incident_type is empty
- ✅ Audit creation succeeds for 100% of signals

---

## 📈 **Non-Functional Requirements**

### **NFR-REMEDIATION-015-01: Performance**

- ✅ Incident type extraction adds <5ms latency to audit creation
- ✅ No additional network calls (extraction is in-memory)
- ✅ Normalization logic is stateless and thread-safe

### **NFR-REMEDIATION-015-02: Reliability**

- ✅ Extraction logic never causes audit creation to fail
- ✅ Graceful degradation: uses "unknown-incident" fallback if extraction fails
- ✅ Logging for unexpected signal formats (but still creates audit)

### **NFR-REMEDIATION-015-03: Backward Compatibility**

- ✅ Existing signals without incident_type continue to work (use fallback)
- ✅ Audit creation API accepts empty incident_type (but RemediationExecutor always provides value)
- ✅ No breaking changes for downstream consumers

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Remediation Playbook Catalog (defines incident_type as primary dimension)
- ✅ BR-STORAGE-031-03: Schema migration (incident_type, alert_name, incident_severity columns exist)
- ✅ Data Storage REST API: POST /api/v1/incidents/actions endpoint accepts new fields

### **Downstream Impacts**
- ✅ BR-STORAGE-031-01: Incident-Type Success Rate API can now aggregate by incident_type
- ✅ BR-AI-057: AI Service can query incident-type success rates
- ✅ BR-EFFECTIVENESS-001: Effectiveness Monitor receives incident-classified audits

---

## 🚀 **Implementation Phases**

### **Phase 1: Extraction Logic** (Day 8 - 4 hours)
- Implement `ExtractIncidentType()` function with priority-based extraction
- Implement `NormalizeIncidentType()` normalization logic
- Implement `ExtractSeverity()` function
- Add unit tests for extraction logic (20+ test cases)

### **Phase 2: Audit Creation Integration** (Day 8 - 3 hours)
- Update `CreateNotificationAudit()` to populate incident fields
- Add validation for non-empty incident_type
- Add logging for incident classification decisions

### **Phase 3: Testing** (Day 9 - 3 hours)
- Unit tests: Extraction logic, normalization, fallback scenarios
- Integration tests: Full audit creation with real Data Storage Service
- Test edge cases: Missing alertname, unknown severity, manual requests

### **Phase 4: Monitoring** (Day 9 - 2 hours)
- Add Prometheus metrics: `incident_type_classifications_total{incident_type="..."}`
- Add logging: incident_type extraction results for troubleshooting
- Add alerting: if >10% of audits use "unknown-incident" fallback

**Total Estimated Effort**: 12 hours (1.5 days)

---

## 📊 **Success Metrics**

### **Population Rate**
- **Target**: 100% of new audit records have non-null `incident_type`
- **Measure**: Query Data Storage for NULL incident_type records

### **Fallback Usage**
- **Target**: <5% of audits use "unknown-incident" fallback
- **Measure**: Prometheus metric `incident_type_classifications_total{incident_type="unknown-incident"}`

### **Normalization Effectiveness**
- **Target**: <10 distinct incident_type values for same logical incident
- **Measure**: Manual audit of top 20 incident types for naming consistency

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Data Storage Service Extracts Incident Type**

**Approach**: RemediationExecutor sends raw signal, Data Storage extracts incident_type

**Rejected Because**:
- ❌ Violates separation of concerns (Data Storage is persistence layer, not business logic)
- ❌ Requires Data Storage to understand signal formats (tight coupling)
- ❌ Harder to test and maintain extraction logic in Data Storage

---

### **Alternative 2: Incident Type Registry Service**

**Approach**: Create dedicated microservice for incident type classification

**Rejected Because**:
- ❌ Over-engineering: extraction logic is simple (doesn't warrant separate service)
- ❌ Latency: Additional network call for every audit creation
- ❌ Complexity: More services to deploy and maintain

---

### **Alternative 3: Allow NULL Incident Type**

**Approach**: Make incident_type optional, handle NULLs in aggregation queries

**Rejected Because**:
- ❌ Breaks ADR-033 primary dimension requirement
- ❌ Complicates aggregation queries (must handle NULL case)
- ❌ Reduces data quality and AI learning effectiveness

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P0 priority (required for ADR-033 multi-dimensional tracking)
**Rationale**: Without incident_type population, ADR-033 incident-type aggregation cannot function
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-STORAGE-031-01: Incident-Type Success Rate API (depends on this BR)
- BR-STORAGE-031-03: Schema Migration (provides incident_type column)
- BR-REMEDIATION-016: Populate playbook metadata
- BR-AI-057: AI uses incident-type success rates for playbook selection

### **Related Documents**
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-037: BR Template Standard](../architecture/decisions/ADR-037-business-requirement-template-standard.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

