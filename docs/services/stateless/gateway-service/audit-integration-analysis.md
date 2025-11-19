# Gateway Service - Audit Integration Analysis

**Date**: November 19, 2025  
**Status**: 📋 ANALYSIS COMPLETE - READY FOR IMPLEMENTATION  
**Version**: 1.0

---

## 📊 **Executive Summary**

This document analyzes where and how to integrate audit trace calls in the Gateway service to track events using the Data Storage Service's unified audit API (`POST /api/v1/audit/events`).

**Key Findings**:
- **7 audit integration points** identified in Gateway signal processing pipeline
- **Type-safe event builders** already exist (`pkg/datastorage/audit/gateway_event.go`)
- **HTTP client** needs to be created for Gateway → Data Storage communication
- **Async audit writes** recommended to avoid blocking signal processing (p95 latency target: <5ms overhead)

---

## 🎯 **Business Requirements**

| BR ID | Description | Audit Event Type |
|-------|-------------|------------------|
| **BR-GATEWAY-001** | Signal ingestion tracking | `gateway.signal.received` |
| **BR-GATEWAY-002** | Deduplication decision audit | `gateway.signal.deduplicated` |
| **BR-GATEWAY-003** | Storm detection audit | `gateway.storm.detected` |
| **BR-GATEWAY-004** | CRD creation audit | `gateway.crd.created` |
| **BR-GATEWAY-016** | Storm aggregation audit | `gateway.storm.aggregated` |
| **BR-GATEWAY-008** | Sliding window audit | `gateway.storm.window_extended` |
| **BR-GATEWAY-009** | State-based deduplication audit | `gateway.deduplication.state_checked` |

---

## 📡 **Data Storage Audit API - Quick Reference**

### **Endpoint**: `POST /api/v1/audit/events`

**URL**: `http://data-storage.kubernaut-system:8080/api/v1/audit/events`

**Request Format**:
```json
{
  "version": "1.0",
  "service": "gateway",
  "event_type": "gateway.signal.received",
  "event_timestamp": "2025-11-19T10:00:00.123456Z",
  "correlation_id": "rr-2025-001",
  "resource_type": "pod",
  "resource_id": "api-server-xyz-123",
  "resource_namespace": "production",
  "outcome": "success",
  "operation": "signal_received",
  "severity": "critical",
  "event_data": {
    "version": "1.0",
    "service": "gateway",
    "event_type": "gateway.signal.received",
    "timestamp": "2025-11-19T10:00:00Z",
    "data": {
      "gateway": {
        "signal_type": "prometheus",
        "alert_name": "PodOOMKilled",
        "fingerprint": "sha256:abc123",
        "namespace": "production",
        "resource_type": "pod",
        "resource_name": "api-server-xyz-123",
        "severity": "critical",
        "priority": "P0",
        "environment": "production",
        "deduplication_status": "new",
        "storm_detected": false
      }
    }
  }
}
```

**Response** (201 Created):
```json
{
  "event_id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
  "created_at": "2025-11-19T10:00:01Z",
  "message": "Audit event created successfully"
}
```

**Security**: Network-level via Kubernetes Network Policies (ADR-036)
- No authentication required (internal cluster services)
- Traffic restricted to authorized service namespaces

**Rate Limiting**: 500 writes/sec per service IP

---

## 🏗️ **Gateway Signal Processing Pipeline - Audit Integration Points**

### **Pipeline Overview**

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Gateway Signal Processing Pipeline                │
└─────────────────────────────────────────────────────────────────────┘

1. Signal Ingestion
   ├─ Prometheus AlertManager webhook
   ├─ Kubernetes Event API
   └─ [AUDIT POINT 1] gateway.signal.received ✅

2. Deduplication Check
   ├─ Redis fingerprint lookup (legacy)
   ├─ K8s CRD state query (DD-GATEWAY-009)
   └─ [AUDIT POINT 2] gateway.signal.deduplicated (if duplicate) ✅
   └─ [AUDIT POINT 3] gateway.deduplication.state_checked (DD-009) ✅

3. Storm Detection
   ├─ Rate-based detection (10 alerts/min)
   ├─ Pattern-based detection (5 similar alerts)
   └─ [AUDIT POINT 4] gateway.storm.detected ✅

4. Storm Aggregation (DD-GATEWAY-008)
   ├─ Buffer first-alert (threshold: 5)
   ├─ Sliding window (inactivity timeout: 60s)
   ├─ Multi-tenant isolation (per-namespace limits)
   └─ [AUDIT POINT 5] gateway.storm.buffered ✅
   └─ [AUDIT POINT 6] gateway.storm.window_extended ✅

5. Environment Classification
   ├─ Namespace label lookup
   └─ ConfigMap overrides

6. Priority Assignment
   ├─ Rego policy evaluation
   └─ Fallback priority table

7. CRD Creation
   ├─ RemediationRequest CRD (individual)
   ├─ RemediationRequest CRD (aggregated storm)
   └─ [AUDIT POINT 7] gateway.crd.created ✅
```

---

## 🔧 **Detailed Integration Points**

### **AUDIT POINT 1: Signal Received** ✅

**Location**: `pkg/gateway/server.go:ProcessSignal()` (line ~790)

**Trigger**: Immediately after signal ingestion, before deduplication

**Event Type**: `gateway.signal.received`

**Outcome**: Always `success` (if we reach this point, ingestion succeeded)

**Code Location**:
```go
// pkg/gateway/server.go:785-791
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	start := time.Now()
	logger := middleware.GetLogger(ctx)

	// Record ingestion metric
	s.metricsInstance.AlertsReceivedTotal.WithLabelValues(signal.SourceType, signal.Severity, "unknown").Inc()

	// 🔴 AUDIT POINT 1: Signal received
	// INSERT AUDIT CALL HERE
	// Event: gateway.signal.received
	// Outcome: success
	// Data: signal_type, alert_name, fingerprint, namespace, resource, severity
```

**Event Data Builder**:
```go
eventData, err := audit.NewGatewayEvent("gateway.signal.received").
    WithSignalType(signal.SourceType).  // "prometheus" or "kubernetes"
    WithAlertName(signal.AlertName).    // Prometheus alert name
    WithFingerprint(signal.Fingerprint).
    WithNamespace(signal.Namespace).
    WithResource(signal.ResourceType, signal.ResourceName).
    WithSeverity(signal.Severity).
    Build()
```

**Correlation ID**: `signal.Fingerprint` (until CRD is created, then use CRD name)

**Performance Impact**: ~3-5ms (async write)

---

### **AUDIT POINT 2: Signal Deduplicated** ✅

**Location**: `pkg/gateway/server.go:processDuplicateSignal()` (line ~1000-1050)

**Trigger**: When deduplication check returns `isDuplicate=true`

**Event Type**: `gateway.signal.deduplicated`

**Outcome**: `success` (duplicate correctly identified)

**Code Location**:
```go
// pkg/gateway/server.go:801-804
if isDuplicate {
	// TDD REFACTOR: Extracted duplicate handling
	return s.processDuplicateSignal(ctx, signal, metadata), nil
	// 🔴 AUDIT POINT 2: Signal deduplicated
	// INSERT AUDIT CALL IN processDuplicateSignal()
}
```

**Event Data Builder**:
```go
eventData, err := audit.NewGatewayEvent("gateway.signal.deduplicated").
    WithSignalType(signal.SourceType).
    WithFingerprint(signal.Fingerprint).
    WithNamespace(signal.Namespace).
    WithResource(signal.ResourceType, signal.ResourceName).
    WithDeduplicationStatus("duplicate").
    Build()
```

**Correlation ID**: `metadata.CRDName` (from existing CRD)

**Performance Impact**: ~3-5ms (async write)

---

### **AUDIT POINT 3: State-Based Deduplication Check** ✅

**Location**: `pkg/gateway/processing/deduplication.go:Check()` (line ~150-250)

**Trigger**: When DD-GATEWAY-009 state-based deduplication queries K8s API

**Event Type**: `gateway.deduplication.state_checked`

**Outcome**: 
- `success` if K8s API query succeeds
- `failure` if K8s API unavailable (graceful degradation to Redis)

**Code Location**:
```go
// pkg/gateway/processing/deduplication.go:~200
func (d *Deduplicator) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *DeduplicationMetadata, error) {
	// DD-GATEWAY-009: Query K8s CRD state
	existingCRDs, err := d.k8sClient.List(ctx, &remediationv1.RemediationRequestList{}, ...)
	
	// 🔴 AUDIT POINT 3: State-based deduplication check
	// INSERT AUDIT CALL HERE
	// Event: gateway.deduplication.state_checked
	// Outcome: success (if err == nil), failure (if err != nil)
	// Data: query_result (found/not_found), fallback_to_redis (true/false)
```

**Event Data Builder**:
```go
eventData, err := audit.NewGatewayEvent("gateway.deduplication.state_checked").
    WithSignalType(signal.SourceType).
    WithFingerprint(signal.Fingerprint).
    WithNamespace(signal.Namespace).
    WithLabels(map[string]string{
        "query_result": "found",  // or "not_found"
        "fallback_to_redis": "false",  // or "true"
        "existing_crd_count": "1",
    }).
    Build()
```

**Correlation ID**: `signal.Fingerprint` (before CRD creation)

**Performance Impact**: ~3-5ms (async write)

---

### **AUDIT POINT 4: Storm Detected** ✅

**Location**: `pkg/gateway/server.go:processStormAggregation()` (line ~1082-1090)

**Trigger**: When storm detection returns `isStorm=true`

**Event Type**: `gateway.storm.detected`

**Outcome**: `success` (storm correctly detected)

**Code Location**:
```go
// pkg/gateway/server.go:1085-1091
s.metricsInstance.AlertStormsDetectedTotal.WithLabelValues(stormMetadata.StormType, signal.AlertName).Inc()

logger.Warn("Alert storm detected",
	zap.String("fingerprint", signal.Fingerprint),
	zap.String("stormType", stormMetadata.StormType),
	zap.String("stormWindow", stormMetadata.Window),
	zap.Int("alertCount", stormMetadata.AlertCount))

// 🔴 AUDIT POINT 4: Storm detected
// INSERT AUDIT CALL HERE
// Event: gateway.storm.detected
// Outcome: success
// Data: storm_type, storm_window, alert_count
```

**Event Data Builder**:
```go
eventData, err := audit.NewGatewayEvent("gateway.storm.detected").
    WithSignalType(signal.SourceType).
    WithAlertName(signal.AlertName).
    WithFingerprint(signal.Fingerprint).
    WithNamespace(signal.Namespace).
    WithStorm(stormMetadata.Window).  // Storm ID
    WithLabels(map[string]string{
        "storm_type": stormMetadata.StormType,  // "rate" or "pattern"
        "alert_count": fmt.Sprintf("%d", stormMetadata.AlertCount),
    }).
    Build()
```

**Correlation ID**: `signal.Fingerprint` (will become aggregated CRD name)

**Performance Impact**: ~3-5ms (async write)

---

### **AUDIT POINT 5: Storm Buffered** ✅

**Location**: `pkg/gateway/server.go:processStormAggregation()` (line ~1137-1149)

**Trigger**: When `StartAggregation()` returns empty `windowID` (buffering, threshold not reached)

**Event Type**: `gateway.storm.buffered`

**Outcome**: `success` (alert buffered for aggregation)

**Code Location**:
```go
// pkg/gateway/server.go:1137-1149
if windowID == "" {
	// Alert buffered, threshold not reached yet
	logger.Info("Alert buffered for storm aggregation",
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("alertName", signal.AlertName),
		zap.String("namespace", signal.Namespace))

	// DD-GATEWAY-008: Record namespace buffer utilization (BR-GATEWAY-011)
	if utilization, err := s.stormAggregator.GetNamespaceUtilization(ctx, signal.Namespace); err == nil {
		s.metricsInstance.NamespaceBufferUtilization.WithLabelValues(signal.Namespace).Set(utilization)
	}

	// 🔴 AUDIT POINT 5: Storm buffered
	// INSERT AUDIT CALL HERE
	// Event: gateway.storm.buffered
	// Outcome: success
	// Data: buffer_utilization, namespace_limit
```

**Event Data Builder**:
```go
eventData, err := audit.NewGatewayEvent("gateway.storm.buffered").
    WithSignalType(signal.SourceType).
    WithAlertName(signal.AlertName).
    WithFingerprint(signal.Fingerprint).
    WithNamespace(signal.Namespace).
    WithLabels(map[string]string{
        "buffer_utilization": fmt.Sprintf("%.2f", utilization),
        "buffer_threshold": "5",  // From config
    }).
    Build()
```

**Correlation ID**: `signal.Fingerprint`

**Performance Impact**: ~3-5ms (async write)

---

### **AUDIT POINT 6: Storm Window Extended** ✅

**Location**: `pkg/gateway/server.go:processStormAggregation()` (line ~1102-1120)

**Trigger**: When `ShouldAggregate()` returns `true` and alert is added to existing window

**Event Type**: `gateway.storm.window_extended`

**Outcome**: `success` (alert added to sliding window)

**Code Location**:
```go
// pkg/gateway/server.go:1102-1120
if shouldAggregate {
	// DD-GATEWAY-008: Add to existing aggregation window (sliding window behavior)
	if err := s.stormAggregator.AddResource(ctx, windowID, signal); err != nil {
		logger.Warn("Failed to add resource to storm aggregation, falling back to individual CRD creation",
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("windowID", windowID),
			zap.Error(err))
		return true, nil // Continue to individual CRD creation
	}

	// Successfully added to aggregation window
	resourceCount, _ := s.stormAggregator.GetResourceCount(ctx, windowID)

	logger.Info("Alert added to storm aggregation window",
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("windowID", windowID),
		zap.Int("resourceCount", resourceCount))

	// 🔴 AUDIT POINT 6: Storm window extended
	// INSERT AUDIT CALL HERE
	// Event: gateway.storm.window_extended
	// Outcome: success
	// Data: window_id, resource_count, inactivity_timeout
```

**Event Data Builder**:
```go
eventData, err := audit.NewGatewayEvent("gateway.storm.window_extended").
    WithSignalType(signal.SourceType).
    WithAlertName(signal.AlertName).
    WithFingerprint(signal.Fingerprint).
    WithNamespace(signal.Namespace).
    WithStorm(windowID).  // Storm ID
    WithLabels(map[string]string{
        "resource_count": fmt.Sprintf("%d", resourceCount),
        "inactivity_timeout": "60s",  // From config
    }).
    Build()
```

**Correlation ID**: `windowID` (will become aggregated CRD name)

**Performance Impact**: ~3-5ms (async write)

---

### **AUDIT POINT 7: CRD Created** ✅

**Location**: 
- `pkg/gateway/server.go:createRemediationRequestCRD()` (line ~1190-1210) - Individual CRD
- `pkg/gateway/server.go:createAggregatedCRDAfterWindow()` (line ~1304-1330) - Aggregated CRD

**Trigger**: After successful CRD creation in Kubernetes API

**Event Type**: `gateway.crd.created`

**Outcome**: `success` (if CRD created), `failure` (if K8s API error)

**Code Location (Individual CRD)**:
```go
// pkg/gateway/server.go:1190-1210
// 6. Create RemediationRequest CRD
rr, err := s.crdCreator.CreateRemediationRequest(ctx, signal, priority, environment)
if err != nil {
	logger.Error("Failed to create RemediationRequest CRD",
		zap.String("fingerprint", signal.Fingerprint),
		zap.Error(err))
	return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
}

// 🔴 AUDIT POINT 7A: Individual CRD created
// INSERT AUDIT CALL HERE
// Event: gateway.crd.created
// Outcome: success
// Data: crd_name, crd_type (individual), environment, priority, remediation_path
```

**Code Location (Aggregated CRD)**:
```go
// pkg/gateway/server.go:1304-1330
// Create single aggregated RemediationRequest CRD
rr, err := s.crdCreator.CreateRemediationRequest(ctx, &aggregatedSignal, priority, environment)
if err != nil {
	s.logger.Error("Failed to create aggregated RemediationRequest CRD",
		zap.String("windowID", windowID),
		zap.Int("resourceCount", resourceCount),
		zap.Error(err))

	// Record metric for failed aggregation
	s.metricsInstance.CRDCreationErrors.WithLabelValues("k8s_api_error").Inc()
	return
}

// 🔴 AUDIT POINT 7B: Aggregated CRD created
// INSERT AUDIT CALL HERE
// Event: gateway.crd.created
// Outcome: success
// Data: crd_name, crd_type (aggregated), resource_count, storm_type, window_duration
```

**Event Data Builder (Individual)**:
```go
eventData, err := audit.NewGatewayEvent("gateway.crd.created").
    WithSignalType(signal.SourceType).
    WithAlertName(signal.AlertName).
    WithFingerprint(signal.Fingerprint).
    WithNamespace(signal.Namespace).
    WithResource(signal.ResourceType, signal.ResourceName).
    WithSeverity(signal.Severity).
    WithPriority(priority).
    WithEnvironment(environment).
    WithLabels(map[string]string{
        "crd_name": rr.Name,
        "crd_type": "individual",
        "remediation_path": remediationPath,
    }).
    Build()
```

**Event Data Builder (Aggregated)**:
```go
eventData, err := audit.NewGatewayEvent("gateway.crd.created").
    WithSignalType(signal.SourceType).
    WithAlertName(signal.AlertName).
    WithFingerprint(signal.Fingerprint).
    WithNamespace(signal.Namespace).
    WithSeverity(signal.Severity).
    WithPriority(priority).
    WithEnvironment(environment).
    WithStorm(windowID).
    WithLabels(map[string]string{
        "crd_name": rr.Name,
        "crd_type": "aggregated",
        "resource_count": fmt.Sprintf("%d", resourceCount),
        "storm_type": stormMetadata.StormType,
        "window_duration": fmt.Sprintf("%.2fs", windowDuration.Seconds()),
    }).
    Build()
```

**Correlation ID**: `rr.Name` (RemediationRequest CRD name)

**Performance Impact**: ~3-5ms (async write)

---

## 🏗️ **Implementation Architecture**

### **Component Diagram**

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Gateway Service                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │              server.go (Signal Processing)                    │  │
│  │  ┌────────────────────────────────────────────────────────┐  │  │
│  │  │  ProcessSignal()                                        │  │  │
│  │  │    ├─ [AUDIT 1] Signal received                        │  │  │
│  │  │    ├─ [AUDIT 2] Signal deduplicated                    │  │  │
│  │  │    ├─ [AUDIT 3] State-based dedup check               │  │  │
│  │  │    ├─ [AUDIT 4] Storm detected                         │  │  │
│  │  │    ├─ [AUDIT 5] Storm buffered                         │  │  │
│  │  │    ├─ [AUDIT 6] Storm window extended                  │  │  │
│  │  │    └─ [AUDIT 7] CRD created                            │  │  │
│  │  └────────────────────────────────────────────────────────┘  │  │
│  │                           │                                    │  │
│  │                           │ Async audit writes                 │  │
│  │                           ▼                                    │  │
│  │  ┌────────────────────────────────────────────────────────┐  │  │
│  │  │  audit.Client (NEW)                                     │  │  │
│  │  │    ├─ WriteAuditEvent(ctx, event) error               │  │  │
│  │  │    ├─ HTTP client (5s timeout)                         │  │  │
│  │  │    ├─ Async goroutine (non-blocking)                   │  │  │
│  │  │    └─ Error logging (no retry)                         │  │  │
│  │  └────────────────────────────────────────────────────────┘  │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                           │                                         │
│                           │ HTTP POST /api/v1/audit/events         │
│                           ▼                                         │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                      Data Storage Service                            │
├─────────────────────────────────────────────────────────────────────┤
│  POST /api/v1/audit/events                                          │
│    ├─ Validate request body (version, service, event_type, ...)    │
│    ├─ Write to audit_events table (PostgreSQL)                     │
│    ├─ Record metrics (audit_traces_total, audit_lag_seconds)       │
│    └─ Return 201 Created (event_id, created_at)                    │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 📦 **New Components to Create**

### **1. Audit Client** (NEW)

**File**: `pkg/gateway/audit/client.go`

**Purpose**: HTTP client for posting audit events to Data Storage Service

**Interface**:
```go
package audit

import (
	"context"
	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
)

// Client writes Gateway audit events to Data Storage Service.
type Client interface {
	// WriteAuditEvent writes a single audit event asynchronously.
	// Returns immediately (non-blocking).
	// Errors are logged but not returned.
	WriteAuditEvent(ctx context.Context, event *AuditEvent) error
}

// AuditEvent represents a Gateway audit event.
type AuditEvent struct {
	EventType      string                 // e.g., "gateway.signal.received"
	CorrelationID  string                 // RemediationRequest name or fingerprint
	ResourceType   string                 // e.g., "pod"
	ResourceID     string                 // e.g., "api-server-xyz-123"
	ResourceNS     string                 // Kubernetes namespace
	Outcome        string                 // "success" or "failure"
	Operation      string                 // e.g., "signal_received"
	Severity       string                 // "critical", "warning", "info"
	EventData      map[string]interface{} // Built using audit.GatewayEventBuilder
}
```

**Implementation**:
```go
package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// ClientImpl implements the Client interface.
type ClientImpl struct {
	storageServiceURL string
	httpClient        *http.Client
	logger            *zap.Logger
}

// NewClient creates a new audit client.
func NewClient(storageServiceURL string, logger *zap.Logger) *ClientImpl {
	return &ClientImpl{
		storageServiceURL: storageServiceURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// WriteAuditEvent writes an audit event asynchronously.
// This method returns immediately and does not block signal processing.
func (c *ClientImpl) WriteAuditEvent(ctx context.Context, event *AuditEvent) error {
	// Async write to avoid blocking signal processing
	go func() {
		if err := c.writeAuditEventSync(context.Background(), event); err != nil {
			c.logger.Warn("Failed to write audit event (non-blocking)",
				zap.Error(err),
				zap.String("event_type", event.EventType),
				zap.String("correlation_id", event.CorrelationID))
		}
	}()

	return nil
}

// writeAuditEventSync performs the actual HTTP POST (synchronous).
func (c *ClientImpl) writeAuditEventSync(ctx context.Context, event *AuditEvent) error {
	// Build request payload
	payload := map[string]interface{}{
		"version":            "1.0",
		"service":            "gateway",
		"event_type":         event.EventType,
		"event_timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"correlation_id":     event.CorrelationID,
		"resource_type":      event.ResourceType,
		"resource_id":        event.ResourceID,
		"resource_namespace": event.ResourceNS,
		"outcome":            event.Outcome,
		"operation":          event.Operation,
		"severity":           event.Severity,
		"event_data":         event.EventData,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	// POST to Data Storage Service
	url := fmt.Sprintf("%s/api/v1/audit/events", c.storageServiceURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	c.logger.Debug("Audit event written successfully",
		zap.String("event_type", event.EventType),
		zap.String("correlation_id", event.CorrelationID))

	return nil
}
```

---

### **2. Server Integration** (MODIFY EXISTING)

**File**: `pkg/gateway/server.go`

**Changes**:
1. Add `auditClient` field to `Server` struct
2. Initialize `auditClient` in `NewServer()`
3. Add audit calls at 7 integration points

**Server Struct Update**:
```go
// pkg/gateway/server.go
type Server struct {
	// ... existing fields ...
	auditClient      audit.Client  // NEW: Audit client for Data Storage Service
}
```

**NewServer() Update**:
```go
// pkg/gateway/server.go:NewServer()
func NewServer(cfg *Config, ...) (*Server, error) {
	// ... existing initialization ...

	// Initialize audit client
	auditClient := audit.NewClient(
		cfg.DataStorageServiceURL,  // e.g., "http://data-storage.kubernaut-system:8080"
		logger,
	)

	return &Server{
		// ... existing fields ...
		auditClient: auditClient,
	}, nil
}
```

---

### **3. Configuration Update** (MODIFY EXISTING)

**File**: `pkg/gateway/config/config.go`

**Add**:
```go
// Config for Gateway Service
type Config struct {
	// ... existing fields ...

	// Data Storage Service URL for audit writes
	DataStorageServiceURL string `yaml:"data_storage_service_url" env:"DATA_STORAGE_SERVICE_URL"`
}
```

**File**: `config/gateway.yaml`

**Add**:
```yaml
# Data Storage Service integration
data_storage_service_url: http://data-storage.kubernaut-system:8080
```

---

## 📝 **Example Audit Call Implementation**

### **AUDIT POINT 1: Signal Received**

**Location**: `pkg/gateway/server.go:ProcessSignal()` (after line 790)

```go
// pkg/gateway/server.go:ProcessSignal()
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	start := time.Now()
	logger := middleware.GetLogger(ctx)

	// Record ingestion metric
	s.metricsInstance.AlertsReceivedTotal.WithLabelValues(signal.SourceType, signal.Severity, "unknown").Inc()

	// 🟢 AUDIT POINT 1: Signal received
	if s.auditClient != nil {
		eventData, err := audit.NewGatewayEvent("gateway.signal.received").
			WithSignalType(signal.SourceType).
			WithAlertName(signal.AlertName).
			WithFingerprint(signal.Fingerprint).
			WithNamespace(signal.Namespace).
			WithResource(signal.ResourceType, signal.ResourceName).
			WithSeverity(signal.Severity).
			Build()

		if err == nil {
			s.auditClient.WriteAuditEvent(ctx, &audit.AuditEvent{
				EventType:     "gateway.signal.received",
				CorrelationID: signal.Fingerprint,  // Will be updated to CRD name later
				ResourceType:  signal.ResourceType,
				ResourceID:    signal.ResourceName,
				ResourceNS:    signal.Namespace,
				Outcome:       "success",
				Operation:     "signal_received",
				Severity:      signal.Severity,
				EventData:     eventData,
			})
		}
	}

	// 1. Deduplication check
	isDuplicate, metadata, err := s.deduplicator.Check(ctx, signal)
	// ... rest of processing ...
}
```

---

## 🎯 **Performance Considerations**

### **Async Audit Writes**

**Design Decision**: All audit writes are **asynchronous** (non-blocking)

**Rationale**:
- Signal processing p95 latency target: **<100ms**
- Audit write latency: **~50ms** (HTTP POST + database write)
- **Blocking audit writes would add 50ms to every signal** → Unacceptable
- **Async writes add <1ms overhead** (goroutine spawn) → Acceptable

**Trade-offs**:
- ✅ **Pro**: No impact on signal processing latency
- ✅ **Pro**: Gateway remains responsive even if Data Storage is slow
- ❌ **Con**: Audit writes may be lost if Gateway crashes before write completes
- ❌ **Con**: No error feedback to caller (errors are logged only)

**Mitigation**:
- Data Storage Service has **Dead Letter Queue (DLQ)** for database failures (DD-009)
- Gateway audit client logs errors for observability
- Prometheus metrics track audit write failures

---

### **Latency Budget**

| Component | Latency (p95) | Notes |
|-----------|---------------|-------|
| **Signal Processing** | 80ms | Existing (dedup, storm, CRD creation) |
| **Audit Write (Sync)** | 50ms | HTTP POST + database write |
| **Audit Write (Async)** | <1ms | Goroutine spawn overhead |
| **Total (with async audit)** | **81ms** | ✅ Within 100ms target |
| **Total (with sync audit)** | **130ms** | ❌ Exceeds 100ms target |

**Conclusion**: Async audit writes are **mandatory** to meet latency requirements.

---

## 🚨 **Error Handling Strategy**

### **Audit Write Failures**

**Principle**: Audit writes are **best-effort** and **never block signal processing**

**Error Scenarios**:

| Error | Handling | Impact |
|-------|----------|--------|
| **Data Storage unavailable** | Log warning, continue processing | Audit event lost |
| **HTTP timeout (>5s)** | Log warning, continue processing | Audit event lost |
| **Invalid event data** | Log error, continue processing | Audit event lost |
| **Rate limit exceeded (429)** | Log warning, continue processing | Audit event lost |

**Monitoring**:
- Gateway logs audit write failures at `WARN` level
- Prometheus metric: `gateway_audit_write_failures_total{event_type, reason}`
- Data Storage Service tracks `datastorage_audit_traces_total{service="gateway", status="success|failure"}`

**Alerting**:
- Alert if `gateway_audit_write_failures_total` > 5% of `gateway_signals_received_total`
- Alert if Data Storage Service is unavailable for >5 minutes

---

## 🧪 **Testing Strategy**

### **Unit Tests**

**File**: `pkg/gateway/audit/client_test.go`

**Test Cases**:
1. ✅ Successful audit write (201 Created)
2. ✅ HTTP timeout (5s)
3. ✅ Data Storage unavailable (connection refused)
4. ✅ Invalid response (non-201 status)
5. ✅ Async write does not block caller

**Coverage Target**: 90%+

---

### **Integration Tests**

**File**: `test/integration/gateway/audit_integration_test.go`

**Test Cases**:
1. ✅ Signal received → audit event created in Data Storage
2. ✅ Signal deduplicated → audit event created
3. ✅ Storm detected → audit event created
4. ✅ CRD created → audit event created with correct correlation_id
5. ✅ Audit write failure does not block signal processing

**Coverage Target**: 100% of 7 audit integration points

---

### **E2E Tests**

**File**: `test/e2e/gateway/06_audit_integration_test.go`

**Test Cases**:
1. ✅ End-to-end signal processing with audit trail
2. ✅ Query Data Storage API to verify audit events
3. ✅ Verify correlation_id links signal → CRD → audit events

**Coverage Target**: 3 critical user journeys

---

## 📊 **Metrics**

### **New Metrics** (Gateway Service)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_audit_write_attempts_total` | Counter | `event_type` | Total audit write attempts |
| `gateway_audit_write_failures_total` | Counter | `event_type`, `reason` | Failed audit writes |
| `gateway_audit_write_duration_seconds` | Histogram | `event_type` | Audit write latency (async goroutine) |

### **Existing Metrics** (Data Storage Service)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `datastorage_audit_traces_total` | Counter | `service="gateway"`, `status` | Audit events received from Gateway |
| `datastorage_audit_lag_seconds` | Histogram | `service="gateway"` | Time between event occurrence and audit write |

---

## 🗓️ **Implementation Roadmap**

### **Phase 1: Foundation** (Day 1-2)

**Tasks**:
1. Create `pkg/gateway/audit/client.go` (HTTP client)
2. Add `auditClient` field to `Server` struct
3. Update `Config` with `data_storage_service_url`
4. Write unit tests for `audit.Client`

**Deliverables**:
- ✅ Audit client with async write capability
- ✅ Unit tests (90%+ coverage)

---

### **Phase 2: Integration** (Day 3-4)

**Tasks**:
1. Add audit call at **AUDIT POINT 1** (Signal Received)
2. Add audit call at **AUDIT POINT 2** (Signal Deduplicated)
3. Add audit call at **AUDIT POINT 3** (State-Based Dedup Check)
4. Add audit call at **AUDIT POINT 4** (Storm Detected)
5. Write integration tests

**Deliverables**:
- ✅ 4 audit integration points implemented
- ✅ Integration tests (100% coverage of 4 points)

---

### **Phase 3: Storm Aggregation** (Day 5-6)

**Tasks**:
1. Add audit call at **AUDIT POINT 5** (Storm Buffered)
2. Add audit call at **AUDIT POINT 6** (Storm Window Extended)
3. Add audit call at **AUDIT POINT 7** (CRD Created - both individual and aggregated)
4. Write integration tests

**Deliverables**:
- ✅ 3 audit integration points implemented
- ✅ Integration tests (100% coverage of 3 points)

---

### **Phase 4: Validation** (Day 7-8)

**Tasks**:
1. Write E2E tests (3 critical user journeys)
2. Performance testing (verify <1ms overhead)
3. Error handling testing (Data Storage unavailable)
4. Update deployment manifests (config)

**Deliverables**:
- ✅ E2E tests (3 scenarios)
- ✅ Performance validation report
- ✅ Updated deployment manifests

---

### **Phase 5: Documentation** (Day 9)

**Tasks**:
1. Update Gateway service documentation
2. Create audit event catalog (7 event types)
3. Update architecture diagrams
4. Create runbook for audit troubleshooting

**Deliverables**:
- ✅ Comprehensive documentation
- ✅ Audit event catalog
- ✅ Troubleshooting runbook

---

## 📚 **References**

### **OpenAPI Specifications**
- [Data Storage Audit Write API](../data-storage/api/audit-write-api.openapi.yaml)
- [Data Storage Service API v1](../../../api/openapi/data-storage-v1.yaml)

### **Design Decisions**
- [DD-GATEWAY-008: Storm Buffering](DD-GATEWAY-008-storm-aggregation-first-alert-handling.md)
- [DD-GATEWAY-009: State-Based Deduplication](DD-GATEWAY-009-state-based-deduplication.md)
- [ADR-034: Unified Audit Table Design](../../architecture/decisions/ADR-034-unified-audit-table.md)
- [ADR-036: Authentication and Authorization Strategy](../../architecture/decisions/ADR-036-auth-strategy.md)

### **Implementation Plans**
- [DD-GATEWAY-008 Implementation Plan](DD_GATEWAY_008_IMPLEMENTATION_PLAN.md)
- [DD-GATEWAY-009 Implementation Plan](DD_GATEWAY_009_IMPLEMENTATION_PLAN.md)

### **Code References**
- [Gateway Event Builder](../../../pkg/datastorage/audit/gateway_event.go)
- [Gateway Server](../../../pkg/gateway/server.go)
- [Data Storage Audit Events Handler](../../../pkg/datastorage/server/audit_events_handler.go)

---

## ✅ **Next Steps**

1. **Review this analysis** with the team
2. **Approve implementation approach** (async audit writes)
3. **Create implementation plan** following APDC methodology
4. **Start Phase 1** (Foundation - audit client creation)

---

**Document Status**: ✅ READY FOR REVIEW  
**Confidence**: 95% (comprehensive analysis with existing audit infrastructure)  
**Estimated Effort**: 9 days (1 developer, full-time)

