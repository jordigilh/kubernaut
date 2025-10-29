# Day 9 Phase 2.4: Webhook Handler Metrics Integration

**Date**: 2025-10-26
**Status**: 🔧 **PLANNING**
**Estimated Time**: 45 minutes

---

## 🎯 **Objective**

Wire centralized metrics (`s.metrics`) into webhook handler processing flow for signal ingestion tracking.

---

## 📋 **Current State Analysis**

### **Existing Metrics in handlers.go**

The webhook handlers currently use **TWO types of metrics**:

#### **1. Basic Signal Processing Metrics** (Legacy - Individual Fields)
```go
// Lines 112, etc.
s.webhookRequestsTotal.Inc()
s.webhookErrorsTotal.Inc()
s.crdCreationTotal.Inc()
s.webhookProcessingSeconds.Observe(duration)
```

**These SHOULD be replaced with centralized metrics**:
- `s.webhookRequestsTotal` → `s.metrics.SignalsReceived`
- `s.webhookErrorsTotal` → `s.metrics.SignalsFailed`
- `s.crdCreationTotal` → `s.metrics.CRDsCreated`
- `s.webhookProcessingSeconds` → `s.metrics.ProcessingDuration`

#### **2. Redis Health Monitoring Metrics** (v2.10 - DD-GATEWAY-003)
```go
// Lines 148-152, 166-168, 176, 190-191
s.redisOperationErrorsTotal.WithLabelValues("check", "deduplication", "unavailable").Inc()
s.requestsRejectedTotal.WithLabelValues("redis_unavailable", "deduplication").Inc()
s.duplicateCRDsPreventedTotal.Inc()
s.duplicatePreventionActive.Set(1)
s.consecutive503Responses.WithLabelValues(signal.Namespace).Inc()
```

**These should REMAIN as individual fields** (not in centralized metrics):
- Part of Redis health monitoring feature (v2.10)
- Documented in DD-GATEWAY-003
- Specific to Redis failure handling
- NOT part of basic signal processing metrics

---

## 🎯 **Scope for Phase 2.4**

### **IN SCOPE** ✅
Replace these legacy metrics with centralized `s.metrics`:

| Legacy Field | Centralized Metric | Label Values |
|---|---|---|
| `s.webhookRequestsTotal` | `s.metrics.SignalsReceived` | `source`, `signal_type` |
| `s.webhookErrorsTotal` | `s.metrics.SignalsFailed` | `source`, `error_type` |
| `s.crdCreationTotal` | `s.metrics.CRDsCreated` | `namespace`, `priority` |
| `s.webhookProcessingSeconds` | `s.metrics.ProcessingDuration` | `source`, `stage` |
| N/A (new) | `s.metrics.DuplicateSignals` | `source` |
| N/A (new) | `s.metrics.SignalsProcessed` | `source`, `priority`, `environment` |

### **OUT OF SCOPE** ❌
Keep these Redis-specific metrics as-is:

- `s.redisOperationErrorsTotal` - Redis operation failures
- `s.requestsRejectedTotal` - 503 rejections
- `s.duplicateCRDsPreventedTotal` - Duplicate prevention tracking
- `s.duplicatePreventionActive` - Deduplication health gauge
- `s.consecutive503Responses` - Prometheus retry risk tracking
- `s.duration503Seconds` - 503 period duration
- `s.alertsQueuedEstimate` - Estimated queued alerts
- `s.redisMasterChangesTotal` - Sentinel failovers
- `s.redisFailoverDurationSeconds` - Failover duration

**Rationale**: These are part of DD-GATEWAY-003 (Redis Outage Risk Tracking) and should remain separate.

---

## 🔧 **Implementation Plan**

### **Step 1: Add Nil Check** (5 min)
Add nil check at the start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... rest of function
}
```

### **Step 2: Replace Legacy Metrics** (20 min)

#### **2.1: Signal Reception** (Line 112)
```go
// OLD:
s.webhookRequestsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
}
```

#### **2.2: Signal Processing Errors** (Multiple locations)
```go
// OLD:
s.webhookErrorsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsFailed.WithLabelValues(sourceType, errorType).Inc()
}
```

**Error Types**:
- `"read_error"` - Failed to read request body
- `"parse_error"` - Failed to parse webhook payload
- `"crd_creation_error"` - Failed to create CRD

#### **2.3: Duplicate Detection** (Lines 159-172)
```go
// NEW: Track duplicate detection
if s.metrics != nil {
	s.metrics.DuplicateSignals.WithLabelValues(sourceType).Inc()
}
```

#### **2.4: CRD Creation** (After successful CRD creation)
```go
// OLD:
s.crdCreationTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.CRDsCreated.WithLabelValues(signal.Namespace, priority).Inc()
}
```

#### **2.5: Processing Duration** (End of successful processing)
```go
// OLD:
s.webhookProcessingSeconds.Observe(duration)

// NEW:
if s.metrics != nil {
	s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
}
```

#### **2.6: Successful Processing** (After CRD creation)
```go
// NEW: Track successful signal processing
if s.metrics != nil {
	s.metrics.SignalsProcessed.WithLabelValues(sourceType, priority, environment).Inc()
}
```

### **Step 3: Add Processing Duration Tracking** (10 min)

Add timer at start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	start := time.Now()
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... processing logic ...

	// Track processing duration at end
	if s.metrics != nil {
		duration := time.Since(start).Seconds()
		s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
	}
}
```

### **Step 4: Verify Compilation** (5 min)
```bash
go build -o /dev/null ./pkg/gateway/...
```

### **Step 5: Run Unit Tests** (5 min)
```bash
go test -v ./test/unit/gateway/...
```

---

## ✅ **Success Criteria**

- [ ] All legacy signal processing metrics replaced with `s.metrics.*`
- [ ] Redis-specific metrics remain unchanged (DD-GATEWAY-003)
- [ ] Nil checks prevent panics when metrics disabled
- [ ] Code compiles successfully
- [ ] Unit tests pass (186/187 expected)
- [ ] No new lint errors

---

## 📊 **Metrics Mapping**

| Handler Location | Old Metric | New Metric | Labels |
|---|---|---|---|
| Line 112 | `webhookRequestsTotal` | `SignalsReceived` | `source=Prometheus/K8s`, `signal_type=webhook` |
| Line ~130 | `webhookErrorsTotal` | `SignalsFailed` | `source=Prometheus/K8s`, `error_type=parse_error` |
| Line ~159 | N/A | `DuplicateSignals` | `source=Prometheus/K8s` |
| Line ~250 | `crdCreationTotal` | `CRDsCreated` | `namespace=X`, `priority=P0-P3` |
| Line ~260 | `webhookProcessingSeconds` | `ProcessingDuration` | `source=Prometheus/K8s`, `stage=complete` |
| Line ~260 | N/A | `SignalsProcessed` | `source=Prometheus/K8s`, `priority=P0-P3`, `environment=prod/staging/dev` |

---

## 🚨 **Important Notes**

### **Redis Metrics Are Separate**
The Redis health monitoring metrics (v2.10 - DD-GATEWAY-003) are **intentionally separate** from the centralized metrics:
- They track Redis-specific failure scenarios
- They're part of the Redis outage risk tracking feature
- They're documented in `REDIS_OUTAGE_METRICS.md`
- They should NOT be moved to centralized metrics

### **Nil Safety**
All metrics calls MUST have `if s.metrics != nil` checks to prevent panics when metrics are disabled (e.g., in tests).

### **Label Consistency**
- `source`: "Prometheus AlertManager" or "Kubernetes Event"
- `signal_type`: "webhook"
- `error_type`: "read_error", "parse_error", "crd_creation_error"
- `priority`: "P0", "P1", "P2", "P3"
- `environment`: "production", "staging", "development", "unknown"

---

## 🔗 **Related Files**

- **Handler**: `pkg/gateway/server/handlers.go`
- **Metrics**: `pkg/gateway/metrics/metrics.go`
- **Server**: `pkg/gateway/server/server.go`
- **Redis Metrics Doc**: `docs/services/stateless/gateway-service/REDIS_OUTAGE_METRICS.md`
- **DD-GATEWAY-003**: `docs/decisions/DD-GATEWAY-003-redis-outage-metrics.md`

---

**Status**: 🔧 **READY TO IMPLEMENT**
**Estimated Time**: 45 minutes
**Confidence**: 90% - Clear scope, straightforward implementation



**Date**: 2025-10-26
**Status**: 🔧 **PLANNING**
**Estimated Time**: 45 minutes

---

## 🎯 **Objective**

Wire centralized metrics (`s.metrics`) into webhook handler processing flow for signal ingestion tracking.

---

## 📋 **Current State Analysis**

### **Existing Metrics in handlers.go**

The webhook handlers currently use **TWO types of metrics**:

#### **1. Basic Signal Processing Metrics** (Legacy - Individual Fields)
```go
// Lines 112, etc.
s.webhookRequestsTotal.Inc()
s.webhookErrorsTotal.Inc()
s.crdCreationTotal.Inc()
s.webhookProcessingSeconds.Observe(duration)
```

**These SHOULD be replaced with centralized metrics**:
- `s.webhookRequestsTotal` → `s.metrics.SignalsReceived`
- `s.webhookErrorsTotal` → `s.metrics.SignalsFailed`
- `s.crdCreationTotal` → `s.metrics.CRDsCreated`
- `s.webhookProcessingSeconds` → `s.metrics.ProcessingDuration`

#### **2. Redis Health Monitoring Metrics** (v2.10 - DD-GATEWAY-003)
```go
// Lines 148-152, 166-168, 176, 190-191
s.redisOperationErrorsTotal.WithLabelValues("check", "deduplication", "unavailable").Inc()
s.requestsRejectedTotal.WithLabelValues("redis_unavailable", "deduplication").Inc()
s.duplicateCRDsPreventedTotal.Inc()
s.duplicatePreventionActive.Set(1)
s.consecutive503Responses.WithLabelValues(signal.Namespace).Inc()
```

**These should REMAIN as individual fields** (not in centralized metrics):
- Part of Redis health monitoring feature (v2.10)
- Documented in DD-GATEWAY-003
- Specific to Redis failure handling
- NOT part of basic signal processing metrics

---

## 🎯 **Scope for Phase 2.4**

### **IN SCOPE** ✅
Replace these legacy metrics with centralized `s.metrics`:

| Legacy Field | Centralized Metric | Label Values |
|---|---|---|
| `s.webhookRequestsTotal` | `s.metrics.SignalsReceived` | `source`, `signal_type` |
| `s.webhookErrorsTotal` | `s.metrics.SignalsFailed` | `source`, `error_type` |
| `s.crdCreationTotal` | `s.metrics.CRDsCreated` | `namespace`, `priority` |
| `s.webhookProcessingSeconds` | `s.metrics.ProcessingDuration` | `source`, `stage` |
| N/A (new) | `s.metrics.DuplicateSignals` | `source` |
| N/A (new) | `s.metrics.SignalsProcessed` | `source`, `priority`, `environment` |

### **OUT OF SCOPE** ❌
Keep these Redis-specific metrics as-is:

- `s.redisOperationErrorsTotal` - Redis operation failures
- `s.requestsRejectedTotal` - 503 rejections
- `s.duplicateCRDsPreventedTotal` - Duplicate prevention tracking
- `s.duplicatePreventionActive` - Deduplication health gauge
- `s.consecutive503Responses` - Prometheus retry risk tracking
- `s.duration503Seconds` - 503 period duration
- `s.alertsQueuedEstimate` - Estimated queued alerts
- `s.redisMasterChangesTotal` - Sentinel failovers
- `s.redisFailoverDurationSeconds` - Failover duration

**Rationale**: These are part of DD-GATEWAY-003 (Redis Outage Risk Tracking) and should remain separate.

---

## 🔧 **Implementation Plan**

### **Step 1: Add Nil Check** (5 min)
Add nil check at the start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... rest of function
}
```

### **Step 2: Replace Legacy Metrics** (20 min)

#### **2.1: Signal Reception** (Line 112)
```go
// OLD:
s.webhookRequestsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
}
```

#### **2.2: Signal Processing Errors** (Multiple locations)
```go
// OLD:
s.webhookErrorsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsFailed.WithLabelValues(sourceType, errorType).Inc()
}
```

**Error Types**:
- `"read_error"` - Failed to read request body
- `"parse_error"` - Failed to parse webhook payload
- `"crd_creation_error"` - Failed to create CRD

#### **2.3: Duplicate Detection** (Lines 159-172)
```go
// NEW: Track duplicate detection
if s.metrics != nil {
	s.metrics.DuplicateSignals.WithLabelValues(sourceType).Inc()
}
```

#### **2.4: CRD Creation** (After successful CRD creation)
```go
// OLD:
s.crdCreationTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.CRDsCreated.WithLabelValues(signal.Namespace, priority).Inc()
}
```

#### **2.5: Processing Duration** (End of successful processing)
```go
// OLD:
s.webhookProcessingSeconds.Observe(duration)

// NEW:
if s.metrics != nil {
	s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
}
```

#### **2.6: Successful Processing** (After CRD creation)
```go
// NEW: Track successful signal processing
if s.metrics != nil {
	s.metrics.SignalsProcessed.WithLabelValues(sourceType, priority, environment).Inc()
}
```

### **Step 3: Add Processing Duration Tracking** (10 min)

Add timer at start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	start := time.Now()
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... processing logic ...

	// Track processing duration at end
	if s.metrics != nil {
		duration := time.Since(start).Seconds()
		s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
	}
}
```

### **Step 4: Verify Compilation** (5 min)
```bash
go build -o /dev/null ./pkg/gateway/...
```

### **Step 5: Run Unit Tests** (5 min)
```bash
go test -v ./test/unit/gateway/...
```

---

## ✅ **Success Criteria**

- [ ] All legacy signal processing metrics replaced with `s.metrics.*`
- [ ] Redis-specific metrics remain unchanged (DD-GATEWAY-003)
- [ ] Nil checks prevent panics when metrics disabled
- [ ] Code compiles successfully
- [ ] Unit tests pass (186/187 expected)
- [ ] No new lint errors

---

## 📊 **Metrics Mapping**

| Handler Location | Old Metric | New Metric | Labels |
|---|---|---|---|
| Line 112 | `webhookRequestsTotal` | `SignalsReceived` | `source=Prometheus/K8s`, `signal_type=webhook` |
| Line ~130 | `webhookErrorsTotal` | `SignalsFailed` | `source=Prometheus/K8s`, `error_type=parse_error` |
| Line ~159 | N/A | `DuplicateSignals` | `source=Prometheus/K8s` |
| Line ~250 | `crdCreationTotal` | `CRDsCreated` | `namespace=X`, `priority=P0-P3` |
| Line ~260 | `webhookProcessingSeconds` | `ProcessingDuration` | `source=Prometheus/K8s`, `stage=complete` |
| Line ~260 | N/A | `SignalsProcessed` | `source=Prometheus/K8s`, `priority=P0-P3`, `environment=prod/staging/dev` |

---

## 🚨 **Important Notes**

### **Redis Metrics Are Separate**
The Redis health monitoring metrics (v2.10 - DD-GATEWAY-003) are **intentionally separate** from the centralized metrics:
- They track Redis-specific failure scenarios
- They're part of the Redis outage risk tracking feature
- They're documented in `REDIS_OUTAGE_METRICS.md`
- They should NOT be moved to centralized metrics

### **Nil Safety**
All metrics calls MUST have `if s.metrics != nil` checks to prevent panics when metrics are disabled (e.g., in tests).

### **Label Consistency**
- `source`: "Prometheus AlertManager" or "Kubernetes Event"
- `signal_type`: "webhook"
- `error_type`: "read_error", "parse_error", "crd_creation_error"
- `priority`: "P0", "P1", "P2", "P3"
- `environment`: "production", "staging", "development", "unknown"

---

## 🔗 **Related Files**

- **Handler**: `pkg/gateway/server/handlers.go`
- **Metrics**: `pkg/gateway/metrics/metrics.go`
- **Server**: `pkg/gateway/server/server.go`
- **Redis Metrics Doc**: `docs/services/stateless/gateway-service/REDIS_OUTAGE_METRICS.md`
- **DD-GATEWAY-003**: `docs/decisions/DD-GATEWAY-003-redis-outage-metrics.md`

---

**Status**: 🔧 **READY TO IMPLEMENT**
**Estimated Time**: 45 minutes
**Confidence**: 90% - Clear scope, straightforward implementation

# Day 9 Phase 2.4: Webhook Handler Metrics Integration

**Date**: 2025-10-26
**Status**: 🔧 **PLANNING**
**Estimated Time**: 45 minutes

---

## 🎯 **Objective**

Wire centralized metrics (`s.metrics`) into webhook handler processing flow for signal ingestion tracking.

---

## 📋 **Current State Analysis**

### **Existing Metrics in handlers.go**

The webhook handlers currently use **TWO types of metrics**:

#### **1. Basic Signal Processing Metrics** (Legacy - Individual Fields)
```go
// Lines 112, etc.
s.webhookRequestsTotal.Inc()
s.webhookErrorsTotal.Inc()
s.crdCreationTotal.Inc()
s.webhookProcessingSeconds.Observe(duration)
```

**These SHOULD be replaced with centralized metrics**:
- `s.webhookRequestsTotal` → `s.metrics.SignalsReceived`
- `s.webhookErrorsTotal` → `s.metrics.SignalsFailed`
- `s.crdCreationTotal` → `s.metrics.CRDsCreated`
- `s.webhookProcessingSeconds` → `s.metrics.ProcessingDuration`

#### **2. Redis Health Monitoring Metrics** (v2.10 - DD-GATEWAY-003)
```go
// Lines 148-152, 166-168, 176, 190-191
s.redisOperationErrorsTotal.WithLabelValues("check", "deduplication", "unavailable").Inc()
s.requestsRejectedTotal.WithLabelValues("redis_unavailable", "deduplication").Inc()
s.duplicateCRDsPreventedTotal.Inc()
s.duplicatePreventionActive.Set(1)
s.consecutive503Responses.WithLabelValues(signal.Namespace).Inc()
```

**These should REMAIN as individual fields** (not in centralized metrics):
- Part of Redis health monitoring feature (v2.10)
- Documented in DD-GATEWAY-003
- Specific to Redis failure handling
- NOT part of basic signal processing metrics

---

## 🎯 **Scope for Phase 2.4**

### **IN SCOPE** ✅
Replace these legacy metrics with centralized `s.metrics`:

| Legacy Field | Centralized Metric | Label Values |
|---|---|---|
| `s.webhookRequestsTotal` | `s.metrics.SignalsReceived` | `source`, `signal_type` |
| `s.webhookErrorsTotal` | `s.metrics.SignalsFailed` | `source`, `error_type` |
| `s.crdCreationTotal` | `s.metrics.CRDsCreated` | `namespace`, `priority` |
| `s.webhookProcessingSeconds` | `s.metrics.ProcessingDuration` | `source`, `stage` |
| N/A (new) | `s.metrics.DuplicateSignals` | `source` |
| N/A (new) | `s.metrics.SignalsProcessed` | `source`, `priority`, `environment` |

### **OUT OF SCOPE** ❌
Keep these Redis-specific metrics as-is:

- `s.redisOperationErrorsTotal` - Redis operation failures
- `s.requestsRejectedTotal` - 503 rejections
- `s.duplicateCRDsPreventedTotal` - Duplicate prevention tracking
- `s.duplicatePreventionActive` - Deduplication health gauge
- `s.consecutive503Responses` - Prometheus retry risk tracking
- `s.duration503Seconds` - 503 period duration
- `s.alertsQueuedEstimate` - Estimated queued alerts
- `s.redisMasterChangesTotal` - Sentinel failovers
- `s.redisFailoverDurationSeconds` - Failover duration

**Rationale**: These are part of DD-GATEWAY-003 (Redis Outage Risk Tracking) and should remain separate.

---

## 🔧 **Implementation Plan**

### **Step 1: Add Nil Check** (5 min)
Add nil check at the start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... rest of function
}
```

### **Step 2: Replace Legacy Metrics** (20 min)

#### **2.1: Signal Reception** (Line 112)
```go
// OLD:
s.webhookRequestsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
}
```

#### **2.2: Signal Processing Errors** (Multiple locations)
```go
// OLD:
s.webhookErrorsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsFailed.WithLabelValues(sourceType, errorType).Inc()
}
```

**Error Types**:
- `"read_error"` - Failed to read request body
- `"parse_error"` - Failed to parse webhook payload
- `"crd_creation_error"` - Failed to create CRD

#### **2.3: Duplicate Detection** (Lines 159-172)
```go
// NEW: Track duplicate detection
if s.metrics != nil {
	s.metrics.DuplicateSignals.WithLabelValues(sourceType).Inc()
}
```

#### **2.4: CRD Creation** (After successful CRD creation)
```go
// OLD:
s.crdCreationTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.CRDsCreated.WithLabelValues(signal.Namespace, priority).Inc()
}
```

#### **2.5: Processing Duration** (End of successful processing)
```go
// OLD:
s.webhookProcessingSeconds.Observe(duration)

// NEW:
if s.metrics != nil {
	s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
}
```

#### **2.6: Successful Processing** (After CRD creation)
```go
// NEW: Track successful signal processing
if s.metrics != nil {
	s.metrics.SignalsProcessed.WithLabelValues(sourceType, priority, environment).Inc()
}
```

### **Step 3: Add Processing Duration Tracking** (10 min)

Add timer at start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	start := time.Now()
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... processing logic ...

	// Track processing duration at end
	if s.metrics != nil {
		duration := time.Since(start).Seconds()
		s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
	}
}
```

### **Step 4: Verify Compilation** (5 min)
```bash
go build -o /dev/null ./pkg/gateway/...
```

### **Step 5: Run Unit Tests** (5 min)
```bash
go test -v ./test/unit/gateway/...
```

---

## ✅ **Success Criteria**

- [ ] All legacy signal processing metrics replaced with `s.metrics.*`
- [ ] Redis-specific metrics remain unchanged (DD-GATEWAY-003)
- [ ] Nil checks prevent panics when metrics disabled
- [ ] Code compiles successfully
- [ ] Unit tests pass (186/187 expected)
- [ ] No new lint errors

---

## 📊 **Metrics Mapping**

| Handler Location | Old Metric | New Metric | Labels |
|---|---|---|---|
| Line 112 | `webhookRequestsTotal` | `SignalsReceived` | `source=Prometheus/K8s`, `signal_type=webhook` |
| Line ~130 | `webhookErrorsTotal` | `SignalsFailed` | `source=Prometheus/K8s`, `error_type=parse_error` |
| Line ~159 | N/A | `DuplicateSignals` | `source=Prometheus/K8s` |
| Line ~250 | `crdCreationTotal` | `CRDsCreated` | `namespace=X`, `priority=P0-P3` |
| Line ~260 | `webhookProcessingSeconds` | `ProcessingDuration` | `source=Prometheus/K8s`, `stage=complete` |
| Line ~260 | N/A | `SignalsProcessed` | `source=Prometheus/K8s`, `priority=P0-P3`, `environment=prod/staging/dev` |

---

## 🚨 **Important Notes**

### **Redis Metrics Are Separate**
The Redis health monitoring metrics (v2.10 - DD-GATEWAY-003) are **intentionally separate** from the centralized metrics:
- They track Redis-specific failure scenarios
- They're part of the Redis outage risk tracking feature
- They're documented in `REDIS_OUTAGE_METRICS.md`
- They should NOT be moved to centralized metrics

### **Nil Safety**
All metrics calls MUST have `if s.metrics != nil` checks to prevent panics when metrics are disabled (e.g., in tests).

### **Label Consistency**
- `source`: "Prometheus AlertManager" or "Kubernetes Event"
- `signal_type`: "webhook"
- `error_type`: "read_error", "parse_error", "crd_creation_error"
- `priority`: "P0", "P1", "P2", "P3"
- `environment`: "production", "staging", "development", "unknown"

---

## 🔗 **Related Files**

- **Handler**: `pkg/gateway/server/handlers.go`
- **Metrics**: `pkg/gateway/metrics/metrics.go`
- **Server**: `pkg/gateway/server/server.go`
- **Redis Metrics Doc**: `docs/services/stateless/gateway-service/REDIS_OUTAGE_METRICS.md`
- **DD-GATEWAY-003**: `docs/decisions/DD-GATEWAY-003-redis-outage-metrics.md`

---

**Status**: 🔧 **READY TO IMPLEMENT**
**Estimated Time**: 45 minutes
**Confidence**: 90% - Clear scope, straightforward implementation

# Day 9 Phase 2.4: Webhook Handler Metrics Integration

**Date**: 2025-10-26
**Status**: 🔧 **PLANNING**
**Estimated Time**: 45 minutes

---

## 🎯 **Objective**

Wire centralized metrics (`s.metrics`) into webhook handler processing flow for signal ingestion tracking.

---

## 📋 **Current State Analysis**

### **Existing Metrics in handlers.go**

The webhook handlers currently use **TWO types of metrics**:

#### **1. Basic Signal Processing Metrics** (Legacy - Individual Fields)
```go
// Lines 112, etc.
s.webhookRequestsTotal.Inc()
s.webhookErrorsTotal.Inc()
s.crdCreationTotal.Inc()
s.webhookProcessingSeconds.Observe(duration)
```

**These SHOULD be replaced with centralized metrics**:
- `s.webhookRequestsTotal` → `s.metrics.SignalsReceived`
- `s.webhookErrorsTotal` → `s.metrics.SignalsFailed`
- `s.crdCreationTotal` → `s.metrics.CRDsCreated`
- `s.webhookProcessingSeconds` → `s.metrics.ProcessingDuration`

#### **2. Redis Health Monitoring Metrics** (v2.10 - DD-GATEWAY-003)
```go
// Lines 148-152, 166-168, 176, 190-191
s.redisOperationErrorsTotal.WithLabelValues("check", "deduplication", "unavailable").Inc()
s.requestsRejectedTotal.WithLabelValues("redis_unavailable", "deduplication").Inc()
s.duplicateCRDsPreventedTotal.Inc()
s.duplicatePreventionActive.Set(1)
s.consecutive503Responses.WithLabelValues(signal.Namespace).Inc()
```

**These should REMAIN as individual fields** (not in centralized metrics):
- Part of Redis health monitoring feature (v2.10)
- Documented in DD-GATEWAY-003
- Specific to Redis failure handling
- NOT part of basic signal processing metrics

---

## 🎯 **Scope for Phase 2.4**

### **IN SCOPE** ✅
Replace these legacy metrics with centralized `s.metrics`:

| Legacy Field | Centralized Metric | Label Values |
|---|---|---|
| `s.webhookRequestsTotal` | `s.metrics.SignalsReceived` | `source`, `signal_type` |
| `s.webhookErrorsTotal` | `s.metrics.SignalsFailed` | `source`, `error_type` |
| `s.crdCreationTotal` | `s.metrics.CRDsCreated` | `namespace`, `priority` |
| `s.webhookProcessingSeconds` | `s.metrics.ProcessingDuration` | `source`, `stage` |
| N/A (new) | `s.metrics.DuplicateSignals` | `source` |
| N/A (new) | `s.metrics.SignalsProcessed` | `source`, `priority`, `environment` |

### **OUT OF SCOPE** ❌
Keep these Redis-specific metrics as-is:

- `s.redisOperationErrorsTotal` - Redis operation failures
- `s.requestsRejectedTotal` - 503 rejections
- `s.duplicateCRDsPreventedTotal` - Duplicate prevention tracking
- `s.duplicatePreventionActive` - Deduplication health gauge
- `s.consecutive503Responses` - Prometheus retry risk tracking
- `s.duration503Seconds` - 503 period duration
- `s.alertsQueuedEstimate` - Estimated queued alerts
- `s.redisMasterChangesTotal` - Sentinel failovers
- `s.redisFailoverDurationSeconds` - Failover duration

**Rationale**: These are part of DD-GATEWAY-003 (Redis Outage Risk Tracking) and should remain separate.

---

## 🔧 **Implementation Plan**

### **Step 1: Add Nil Check** (5 min)
Add nil check at the start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... rest of function
}
```

### **Step 2: Replace Legacy Metrics** (20 min)

#### **2.1: Signal Reception** (Line 112)
```go
// OLD:
s.webhookRequestsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
}
```

#### **2.2: Signal Processing Errors** (Multiple locations)
```go
// OLD:
s.webhookErrorsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsFailed.WithLabelValues(sourceType, errorType).Inc()
}
```

**Error Types**:
- `"read_error"` - Failed to read request body
- `"parse_error"` - Failed to parse webhook payload
- `"crd_creation_error"` - Failed to create CRD

#### **2.3: Duplicate Detection** (Lines 159-172)
```go
// NEW: Track duplicate detection
if s.metrics != nil {
	s.metrics.DuplicateSignals.WithLabelValues(sourceType).Inc()
}
```

#### **2.4: CRD Creation** (After successful CRD creation)
```go
// OLD:
s.crdCreationTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.CRDsCreated.WithLabelValues(signal.Namespace, priority).Inc()
}
```

#### **2.5: Processing Duration** (End of successful processing)
```go
// OLD:
s.webhookProcessingSeconds.Observe(duration)

// NEW:
if s.metrics != nil {
	s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
}
```

#### **2.6: Successful Processing** (After CRD creation)
```go
// NEW: Track successful signal processing
if s.metrics != nil {
	s.metrics.SignalsProcessed.WithLabelValues(sourceType, priority, environment).Inc()
}
```

### **Step 3: Add Processing Duration Tracking** (10 min)

Add timer at start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	start := time.Now()
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... processing logic ...

	// Track processing duration at end
	if s.metrics != nil {
		duration := time.Since(start).Seconds()
		s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
	}
}
```

### **Step 4: Verify Compilation** (5 min)
```bash
go build -o /dev/null ./pkg/gateway/...
```

### **Step 5: Run Unit Tests** (5 min)
```bash
go test -v ./test/unit/gateway/...
```

---

## ✅ **Success Criteria**

- [ ] All legacy signal processing metrics replaced with `s.metrics.*`
- [ ] Redis-specific metrics remain unchanged (DD-GATEWAY-003)
- [ ] Nil checks prevent panics when metrics disabled
- [ ] Code compiles successfully
- [ ] Unit tests pass (186/187 expected)
- [ ] No new lint errors

---

## 📊 **Metrics Mapping**

| Handler Location | Old Metric | New Metric | Labels |
|---|---|---|---|
| Line 112 | `webhookRequestsTotal` | `SignalsReceived` | `source=Prometheus/K8s`, `signal_type=webhook` |
| Line ~130 | `webhookErrorsTotal` | `SignalsFailed` | `source=Prometheus/K8s`, `error_type=parse_error` |
| Line ~159 | N/A | `DuplicateSignals` | `source=Prometheus/K8s` |
| Line ~250 | `crdCreationTotal` | `CRDsCreated` | `namespace=X`, `priority=P0-P3` |
| Line ~260 | `webhookProcessingSeconds` | `ProcessingDuration` | `source=Prometheus/K8s`, `stage=complete` |
| Line ~260 | N/A | `SignalsProcessed` | `source=Prometheus/K8s`, `priority=P0-P3`, `environment=prod/staging/dev` |

---

## 🚨 **Important Notes**

### **Redis Metrics Are Separate**
The Redis health monitoring metrics (v2.10 - DD-GATEWAY-003) are **intentionally separate** from the centralized metrics:
- They track Redis-specific failure scenarios
- They're part of the Redis outage risk tracking feature
- They're documented in `REDIS_OUTAGE_METRICS.md`
- They should NOT be moved to centralized metrics

### **Nil Safety**
All metrics calls MUST have `if s.metrics != nil` checks to prevent panics when metrics are disabled (e.g., in tests).

### **Label Consistency**
- `source`: "Prometheus AlertManager" or "Kubernetes Event"
- `signal_type`: "webhook"
- `error_type`: "read_error", "parse_error", "crd_creation_error"
- `priority`: "P0", "P1", "P2", "P3"
- `environment`: "production", "staging", "development", "unknown"

---

## 🔗 **Related Files**

- **Handler**: `pkg/gateway/server/handlers.go`
- **Metrics**: `pkg/gateway/metrics/metrics.go`
- **Server**: `pkg/gateway/server/server.go`
- **Redis Metrics Doc**: `docs/services/stateless/gateway-service/REDIS_OUTAGE_METRICS.md`
- **DD-GATEWAY-003**: `docs/decisions/DD-GATEWAY-003-redis-outage-metrics.md`

---

**Status**: 🔧 **READY TO IMPLEMENT**
**Estimated Time**: 45 minutes
**Confidence**: 90% - Clear scope, straightforward implementation



**Date**: 2025-10-26
**Status**: 🔧 **PLANNING**
**Estimated Time**: 45 minutes

---

## 🎯 **Objective**

Wire centralized metrics (`s.metrics`) into webhook handler processing flow for signal ingestion tracking.

---

## 📋 **Current State Analysis**

### **Existing Metrics in handlers.go**

The webhook handlers currently use **TWO types of metrics**:

#### **1. Basic Signal Processing Metrics** (Legacy - Individual Fields)
```go
// Lines 112, etc.
s.webhookRequestsTotal.Inc()
s.webhookErrorsTotal.Inc()
s.crdCreationTotal.Inc()
s.webhookProcessingSeconds.Observe(duration)
```

**These SHOULD be replaced with centralized metrics**:
- `s.webhookRequestsTotal` → `s.metrics.SignalsReceived`
- `s.webhookErrorsTotal` → `s.metrics.SignalsFailed`
- `s.crdCreationTotal` → `s.metrics.CRDsCreated`
- `s.webhookProcessingSeconds` → `s.metrics.ProcessingDuration`

#### **2. Redis Health Monitoring Metrics** (v2.10 - DD-GATEWAY-003)
```go
// Lines 148-152, 166-168, 176, 190-191
s.redisOperationErrorsTotal.WithLabelValues("check", "deduplication", "unavailable").Inc()
s.requestsRejectedTotal.WithLabelValues("redis_unavailable", "deduplication").Inc()
s.duplicateCRDsPreventedTotal.Inc()
s.duplicatePreventionActive.Set(1)
s.consecutive503Responses.WithLabelValues(signal.Namespace).Inc()
```

**These should REMAIN as individual fields** (not in centralized metrics):
- Part of Redis health monitoring feature (v2.10)
- Documented in DD-GATEWAY-003
- Specific to Redis failure handling
- NOT part of basic signal processing metrics

---

## 🎯 **Scope for Phase 2.4**

### **IN SCOPE** ✅
Replace these legacy metrics with centralized `s.metrics`:

| Legacy Field | Centralized Metric | Label Values |
|---|---|---|
| `s.webhookRequestsTotal` | `s.metrics.SignalsReceived` | `source`, `signal_type` |
| `s.webhookErrorsTotal` | `s.metrics.SignalsFailed` | `source`, `error_type` |
| `s.crdCreationTotal` | `s.metrics.CRDsCreated` | `namespace`, `priority` |
| `s.webhookProcessingSeconds` | `s.metrics.ProcessingDuration` | `source`, `stage` |
| N/A (new) | `s.metrics.DuplicateSignals` | `source` |
| N/A (new) | `s.metrics.SignalsProcessed` | `source`, `priority`, `environment` |

### **OUT OF SCOPE** ❌
Keep these Redis-specific metrics as-is:

- `s.redisOperationErrorsTotal` - Redis operation failures
- `s.requestsRejectedTotal` - 503 rejections
- `s.duplicateCRDsPreventedTotal` - Duplicate prevention tracking
- `s.duplicatePreventionActive` - Deduplication health gauge
- `s.consecutive503Responses` - Prometheus retry risk tracking
- `s.duration503Seconds` - 503 period duration
- `s.alertsQueuedEstimate` - Estimated queued alerts
- `s.redisMasterChangesTotal` - Sentinel failovers
- `s.redisFailoverDurationSeconds` - Failover duration

**Rationale**: These are part of DD-GATEWAY-003 (Redis Outage Risk Tracking) and should remain separate.

---

## 🔧 **Implementation Plan**

### **Step 1: Add Nil Check** (5 min)
Add nil check at the start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... rest of function
}
```

### **Step 2: Replace Legacy Metrics** (20 min)

#### **2.1: Signal Reception** (Line 112)
```go
// OLD:
s.webhookRequestsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
}
```

#### **2.2: Signal Processing Errors** (Multiple locations)
```go
// OLD:
s.webhookErrorsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsFailed.WithLabelValues(sourceType, errorType).Inc()
}
```

**Error Types**:
- `"read_error"` - Failed to read request body
- `"parse_error"` - Failed to parse webhook payload
- `"crd_creation_error"` - Failed to create CRD

#### **2.3: Duplicate Detection** (Lines 159-172)
```go
// NEW: Track duplicate detection
if s.metrics != nil {
	s.metrics.DuplicateSignals.WithLabelValues(sourceType).Inc()
}
```

#### **2.4: CRD Creation** (After successful CRD creation)
```go
// OLD:
s.crdCreationTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.CRDsCreated.WithLabelValues(signal.Namespace, priority).Inc()
}
```

#### **2.5: Processing Duration** (End of successful processing)
```go
// OLD:
s.webhookProcessingSeconds.Observe(duration)

// NEW:
if s.metrics != nil {
	s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
}
```

#### **2.6: Successful Processing** (After CRD creation)
```go
// NEW: Track successful signal processing
if s.metrics != nil {
	s.metrics.SignalsProcessed.WithLabelValues(sourceType, priority, environment).Inc()
}
```

### **Step 3: Add Processing Duration Tracking** (10 min)

Add timer at start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	start := time.Now()
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... processing logic ...

	// Track processing duration at end
	if s.metrics != nil {
		duration := time.Since(start).Seconds()
		s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
	}
}
```

### **Step 4: Verify Compilation** (5 min)
```bash
go build -o /dev/null ./pkg/gateway/...
```

### **Step 5: Run Unit Tests** (5 min)
```bash
go test -v ./test/unit/gateway/...
```

---

## ✅ **Success Criteria**

- [ ] All legacy signal processing metrics replaced with `s.metrics.*`
- [ ] Redis-specific metrics remain unchanged (DD-GATEWAY-003)
- [ ] Nil checks prevent panics when metrics disabled
- [ ] Code compiles successfully
- [ ] Unit tests pass (186/187 expected)
- [ ] No new lint errors

---

## 📊 **Metrics Mapping**

| Handler Location | Old Metric | New Metric | Labels |
|---|---|---|---|
| Line 112 | `webhookRequestsTotal` | `SignalsReceived` | `source=Prometheus/K8s`, `signal_type=webhook` |
| Line ~130 | `webhookErrorsTotal` | `SignalsFailed` | `source=Prometheus/K8s`, `error_type=parse_error` |
| Line ~159 | N/A | `DuplicateSignals` | `source=Prometheus/K8s` |
| Line ~250 | `crdCreationTotal` | `CRDsCreated` | `namespace=X`, `priority=P0-P3` |
| Line ~260 | `webhookProcessingSeconds` | `ProcessingDuration` | `source=Prometheus/K8s`, `stage=complete` |
| Line ~260 | N/A | `SignalsProcessed` | `source=Prometheus/K8s`, `priority=P0-P3`, `environment=prod/staging/dev` |

---

## 🚨 **Important Notes**

### **Redis Metrics Are Separate**
The Redis health monitoring metrics (v2.10 - DD-GATEWAY-003) are **intentionally separate** from the centralized metrics:
- They track Redis-specific failure scenarios
- They're part of the Redis outage risk tracking feature
- They're documented in `REDIS_OUTAGE_METRICS.md`
- They should NOT be moved to centralized metrics

### **Nil Safety**
All metrics calls MUST have `if s.metrics != nil` checks to prevent panics when metrics are disabled (e.g., in tests).

### **Label Consistency**
- `source`: "Prometheus AlertManager" or "Kubernetes Event"
- `signal_type`: "webhook"
- `error_type`: "read_error", "parse_error", "crd_creation_error"
- `priority`: "P0", "P1", "P2", "P3"
- `environment`: "production", "staging", "development", "unknown"

---

## 🔗 **Related Files**

- **Handler**: `pkg/gateway/server/handlers.go`
- **Metrics**: `pkg/gateway/metrics/metrics.go`
- **Server**: `pkg/gateway/server/server.go`
- **Redis Metrics Doc**: `docs/services/stateless/gateway-service/REDIS_OUTAGE_METRICS.md`
- **DD-GATEWAY-003**: `docs/decisions/DD-GATEWAY-003-redis-outage-metrics.md`

---

**Status**: 🔧 **READY TO IMPLEMENT**
**Estimated Time**: 45 minutes
**Confidence**: 90% - Clear scope, straightforward implementation

# Day 9 Phase 2.4: Webhook Handler Metrics Integration

**Date**: 2025-10-26
**Status**: 🔧 **PLANNING**
**Estimated Time**: 45 minutes

---

## 🎯 **Objective**

Wire centralized metrics (`s.metrics`) into webhook handler processing flow for signal ingestion tracking.

---

## 📋 **Current State Analysis**

### **Existing Metrics in handlers.go**

The webhook handlers currently use **TWO types of metrics**:

#### **1. Basic Signal Processing Metrics** (Legacy - Individual Fields)
```go
// Lines 112, etc.
s.webhookRequestsTotal.Inc()
s.webhookErrorsTotal.Inc()
s.crdCreationTotal.Inc()
s.webhookProcessingSeconds.Observe(duration)
```

**These SHOULD be replaced with centralized metrics**:
- `s.webhookRequestsTotal` → `s.metrics.SignalsReceived`
- `s.webhookErrorsTotal` → `s.metrics.SignalsFailed`
- `s.crdCreationTotal` → `s.metrics.CRDsCreated`
- `s.webhookProcessingSeconds` → `s.metrics.ProcessingDuration`

#### **2. Redis Health Monitoring Metrics** (v2.10 - DD-GATEWAY-003)
```go
// Lines 148-152, 166-168, 176, 190-191
s.redisOperationErrorsTotal.WithLabelValues("check", "deduplication", "unavailable").Inc()
s.requestsRejectedTotal.WithLabelValues("redis_unavailable", "deduplication").Inc()
s.duplicateCRDsPreventedTotal.Inc()
s.duplicatePreventionActive.Set(1)
s.consecutive503Responses.WithLabelValues(signal.Namespace).Inc()
```

**These should REMAIN as individual fields** (not in centralized metrics):
- Part of Redis health monitoring feature (v2.10)
- Documented in DD-GATEWAY-003
- Specific to Redis failure handling
- NOT part of basic signal processing metrics

---

## 🎯 **Scope for Phase 2.4**

### **IN SCOPE** ✅
Replace these legacy metrics with centralized `s.metrics`:

| Legacy Field | Centralized Metric | Label Values |
|---|---|---|
| `s.webhookRequestsTotal` | `s.metrics.SignalsReceived` | `source`, `signal_type` |
| `s.webhookErrorsTotal` | `s.metrics.SignalsFailed` | `source`, `error_type` |
| `s.crdCreationTotal` | `s.metrics.CRDsCreated` | `namespace`, `priority` |
| `s.webhookProcessingSeconds` | `s.metrics.ProcessingDuration` | `source`, `stage` |
| N/A (new) | `s.metrics.DuplicateSignals` | `source` |
| N/A (new) | `s.metrics.SignalsProcessed` | `source`, `priority`, `environment` |

### **OUT OF SCOPE** ❌
Keep these Redis-specific metrics as-is:

- `s.redisOperationErrorsTotal` - Redis operation failures
- `s.requestsRejectedTotal` - 503 rejections
- `s.duplicateCRDsPreventedTotal` - Duplicate prevention tracking
- `s.duplicatePreventionActive` - Deduplication health gauge
- `s.consecutive503Responses` - Prometheus retry risk tracking
- `s.duration503Seconds` - 503 period duration
- `s.alertsQueuedEstimate` - Estimated queued alerts
- `s.redisMasterChangesTotal` - Sentinel failovers
- `s.redisFailoverDurationSeconds` - Failover duration

**Rationale**: These are part of DD-GATEWAY-003 (Redis Outage Risk Tracking) and should remain separate.

---

## 🔧 **Implementation Plan**

### **Step 1: Add Nil Check** (5 min)
Add nil check at the start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... rest of function
}
```

### **Step 2: Replace Legacy Metrics** (20 min)

#### **2.1: Signal Reception** (Line 112)
```go
// OLD:
s.webhookRequestsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
}
```

#### **2.2: Signal Processing Errors** (Multiple locations)
```go
// OLD:
s.webhookErrorsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsFailed.WithLabelValues(sourceType, errorType).Inc()
}
```

**Error Types**:
- `"read_error"` - Failed to read request body
- `"parse_error"` - Failed to parse webhook payload
- `"crd_creation_error"` - Failed to create CRD

#### **2.3: Duplicate Detection** (Lines 159-172)
```go
// NEW: Track duplicate detection
if s.metrics != nil {
	s.metrics.DuplicateSignals.WithLabelValues(sourceType).Inc()
}
```

#### **2.4: CRD Creation** (After successful CRD creation)
```go
// OLD:
s.crdCreationTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.CRDsCreated.WithLabelValues(signal.Namespace, priority).Inc()
}
```

#### **2.5: Processing Duration** (End of successful processing)
```go
// OLD:
s.webhookProcessingSeconds.Observe(duration)

// NEW:
if s.metrics != nil {
	s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
}
```

#### **2.6: Successful Processing** (After CRD creation)
```go
// NEW: Track successful signal processing
if s.metrics != nil {
	s.metrics.SignalsProcessed.WithLabelValues(sourceType, priority, environment).Inc()
}
```

### **Step 3: Add Processing Duration Tracking** (10 min)

Add timer at start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	start := time.Now()
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... processing logic ...

	// Track processing duration at end
	if s.metrics != nil {
		duration := time.Since(start).Seconds()
		s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
	}
}
```

### **Step 4: Verify Compilation** (5 min)
```bash
go build -o /dev/null ./pkg/gateway/...
```

### **Step 5: Run Unit Tests** (5 min)
```bash
go test -v ./test/unit/gateway/...
```

---

## ✅ **Success Criteria**

- [ ] All legacy signal processing metrics replaced with `s.metrics.*`
- [ ] Redis-specific metrics remain unchanged (DD-GATEWAY-003)
- [ ] Nil checks prevent panics when metrics disabled
- [ ] Code compiles successfully
- [ ] Unit tests pass (186/187 expected)
- [ ] No new lint errors

---

## 📊 **Metrics Mapping**

| Handler Location | Old Metric | New Metric | Labels |
|---|---|---|---|
| Line 112 | `webhookRequestsTotal` | `SignalsReceived` | `source=Prometheus/K8s`, `signal_type=webhook` |
| Line ~130 | `webhookErrorsTotal` | `SignalsFailed` | `source=Prometheus/K8s`, `error_type=parse_error` |
| Line ~159 | N/A | `DuplicateSignals` | `source=Prometheus/K8s` |
| Line ~250 | `crdCreationTotal` | `CRDsCreated` | `namespace=X`, `priority=P0-P3` |
| Line ~260 | `webhookProcessingSeconds` | `ProcessingDuration` | `source=Prometheus/K8s`, `stage=complete` |
| Line ~260 | N/A | `SignalsProcessed` | `source=Prometheus/K8s`, `priority=P0-P3`, `environment=prod/staging/dev` |

---

## 🚨 **Important Notes**

### **Redis Metrics Are Separate**
The Redis health monitoring metrics (v2.10 - DD-GATEWAY-003) are **intentionally separate** from the centralized metrics:
- They track Redis-specific failure scenarios
- They're part of the Redis outage risk tracking feature
- They're documented in `REDIS_OUTAGE_METRICS.md`
- They should NOT be moved to centralized metrics

### **Nil Safety**
All metrics calls MUST have `if s.metrics != nil` checks to prevent panics when metrics are disabled (e.g., in tests).

### **Label Consistency**
- `source`: "Prometheus AlertManager" or "Kubernetes Event"
- `signal_type`: "webhook"
- `error_type`: "read_error", "parse_error", "crd_creation_error"
- `priority`: "P0", "P1", "P2", "P3"
- `environment`: "production", "staging", "development", "unknown"

---

## 🔗 **Related Files**

- **Handler**: `pkg/gateway/server/handlers.go`
- **Metrics**: `pkg/gateway/metrics/metrics.go`
- **Server**: `pkg/gateway/server/server.go`
- **Redis Metrics Doc**: `docs/services/stateless/gateway-service/REDIS_OUTAGE_METRICS.md`
- **DD-GATEWAY-003**: `docs/decisions/DD-GATEWAY-003-redis-outage-metrics.md`

---

**Status**: 🔧 **READY TO IMPLEMENT**
**Estimated Time**: 45 minutes
**Confidence**: 90% - Clear scope, straightforward implementation

# Day 9 Phase 2.4: Webhook Handler Metrics Integration

**Date**: 2025-10-26
**Status**: 🔧 **PLANNING**
**Estimated Time**: 45 minutes

---

## 🎯 **Objective**

Wire centralized metrics (`s.metrics`) into webhook handler processing flow for signal ingestion tracking.

---

## 📋 **Current State Analysis**

### **Existing Metrics in handlers.go**

The webhook handlers currently use **TWO types of metrics**:

#### **1. Basic Signal Processing Metrics** (Legacy - Individual Fields)
```go
// Lines 112, etc.
s.webhookRequestsTotal.Inc()
s.webhookErrorsTotal.Inc()
s.crdCreationTotal.Inc()
s.webhookProcessingSeconds.Observe(duration)
```

**These SHOULD be replaced with centralized metrics**:
- `s.webhookRequestsTotal` → `s.metrics.SignalsReceived`
- `s.webhookErrorsTotal` → `s.metrics.SignalsFailed`
- `s.crdCreationTotal` → `s.metrics.CRDsCreated`
- `s.webhookProcessingSeconds` → `s.metrics.ProcessingDuration`

#### **2. Redis Health Monitoring Metrics** (v2.10 - DD-GATEWAY-003)
```go
// Lines 148-152, 166-168, 176, 190-191
s.redisOperationErrorsTotal.WithLabelValues("check", "deduplication", "unavailable").Inc()
s.requestsRejectedTotal.WithLabelValues("redis_unavailable", "deduplication").Inc()
s.duplicateCRDsPreventedTotal.Inc()
s.duplicatePreventionActive.Set(1)
s.consecutive503Responses.WithLabelValues(signal.Namespace).Inc()
```

**These should REMAIN as individual fields** (not in centralized metrics):
- Part of Redis health monitoring feature (v2.10)
- Documented in DD-GATEWAY-003
- Specific to Redis failure handling
- NOT part of basic signal processing metrics

---

## 🎯 **Scope for Phase 2.4**

### **IN SCOPE** ✅
Replace these legacy metrics with centralized `s.metrics`:

| Legacy Field | Centralized Metric | Label Values |
|---|---|---|
| `s.webhookRequestsTotal` | `s.metrics.SignalsReceived` | `source`, `signal_type` |
| `s.webhookErrorsTotal` | `s.metrics.SignalsFailed` | `source`, `error_type` |
| `s.crdCreationTotal` | `s.metrics.CRDsCreated` | `namespace`, `priority` |
| `s.webhookProcessingSeconds` | `s.metrics.ProcessingDuration` | `source`, `stage` |
| N/A (new) | `s.metrics.DuplicateSignals` | `source` |
| N/A (new) | `s.metrics.SignalsProcessed` | `source`, `priority`, `environment` |

### **OUT OF SCOPE** ❌
Keep these Redis-specific metrics as-is:

- `s.redisOperationErrorsTotal` - Redis operation failures
- `s.requestsRejectedTotal` - 503 rejections
- `s.duplicateCRDsPreventedTotal` - Duplicate prevention tracking
- `s.duplicatePreventionActive` - Deduplication health gauge
- `s.consecutive503Responses` - Prometheus retry risk tracking
- `s.duration503Seconds` - 503 period duration
- `s.alertsQueuedEstimate` - Estimated queued alerts
- `s.redisMasterChangesTotal` - Sentinel failovers
- `s.redisFailoverDurationSeconds` - Failover duration

**Rationale**: These are part of DD-GATEWAY-003 (Redis Outage Risk Tracking) and should remain separate.

---

## 🔧 **Implementation Plan**

### **Step 1: Add Nil Check** (5 min)
Add nil check at the start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... rest of function
}
```

### **Step 2: Replace Legacy Metrics** (20 min)

#### **2.1: Signal Reception** (Line 112)
```go
// OLD:
s.webhookRequestsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
}
```

#### **2.2: Signal Processing Errors** (Multiple locations)
```go
// OLD:
s.webhookErrorsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsFailed.WithLabelValues(sourceType, errorType).Inc()
}
```

**Error Types**:
- `"read_error"` - Failed to read request body
- `"parse_error"` - Failed to parse webhook payload
- `"crd_creation_error"` - Failed to create CRD

#### **2.3: Duplicate Detection** (Lines 159-172)
```go
// NEW: Track duplicate detection
if s.metrics != nil {
	s.metrics.DuplicateSignals.WithLabelValues(sourceType).Inc()
}
```

#### **2.4: CRD Creation** (After successful CRD creation)
```go
// OLD:
s.crdCreationTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.CRDsCreated.WithLabelValues(signal.Namespace, priority).Inc()
}
```

#### **2.5: Processing Duration** (End of successful processing)
```go
// OLD:
s.webhookProcessingSeconds.Observe(duration)

// NEW:
if s.metrics != nil {
	s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
}
```

#### **2.6: Successful Processing** (After CRD creation)
```go
// NEW: Track successful signal processing
if s.metrics != nil {
	s.metrics.SignalsProcessed.WithLabelValues(sourceType, priority, environment).Inc()
}
```

### **Step 3: Add Processing Duration Tracking** (10 min)

Add timer at start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	start := time.Now()
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... processing logic ...

	// Track processing duration at end
	if s.metrics != nil {
		duration := time.Since(start).Seconds()
		s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
	}
}
```

### **Step 4: Verify Compilation** (5 min)
```bash
go build -o /dev/null ./pkg/gateway/...
```

### **Step 5: Run Unit Tests** (5 min)
```bash
go test -v ./test/unit/gateway/...
```

---

## ✅ **Success Criteria**

- [ ] All legacy signal processing metrics replaced with `s.metrics.*`
- [ ] Redis-specific metrics remain unchanged (DD-GATEWAY-003)
- [ ] Nil checks prevent panics when metrics disabled
- [ ] Code compiles successfully
- [ ] Unit tests pass (186/187 expected)
- [ ] No new lint errors

---

## 📊 **Metrics Mapping**

| Handler Location | Old Metric | New Metric | Labels |
|---|---|---|---|
| Line 112 | `webhookRequestsTotal` | `SignalsReceived` | `source=Prometheus/K8s`, `signal_type=webhook` |
| Line ~130 | `webhookErrorsTotal` | `SignalsFailed` | `source=Prometheus/K8s`, `error_type=parse_error` |
| Line ~159 | N/A | `DuplicateSignals` | `source=Prometheus/K8s` |
| Line ~250 | `crdCreationTotal` | `CRDsCreated` | `namespace=X`, `priority=P0-P3` |
| Line ~260 | `webhookProcessingSeconds` | `ProcessingDuration` | `source=Prometheus/K8s`, `stage=complete` |
| Line ~260 | N/A | `SignalsProcessed` | `source=Prometheus/K8s`, `priority=P0-P3`, `environment=prod/staging/dev` |

---

## 🚨 **Important Notes**

### **Redis Metrics Are Separate**
The Redis health monitoring metrics (v2.10 - DD-GATEWAY-003) are **intentionally separate** from the centralized metrics:
- They track Redis-specific failure scenarios
- They're part of the Redis outage risk tracking feature
- They're documented in `REDIS_OUTAGE_METRICS.md`
- They should NOT be moved to centralized metrics

### **Nil Safety**
All metrics calls MUST have `if s.metrics != nil` checks to prevent panics when metrics are disabled (e.g., in tests).

### **Label Consistency**
- `source`: "Prometheus AlertManager" or "Kubernetes Event"
- `signal_type`: "webhook"
- `error_type`: "read_error", "parse_error", "crd_creation_error"
- `priority`: "P0", "P1", "P2", "P3"
- `environment`: "production", "staging", "development", "unknown"

---

## 🔗 **Related Files**

- **Handler**: `pkg/gateway/server/handlers.go`
- **Metrics**: `pkg/gateway/metrics/metrics.go`
- **Server**: `pkg/gateway/server/server.go`
- **Redis Metrics Doc**: `docs/services/stateless/gateway-service/REDIS_OUTAGE_METRICS.md`
- **DD-GATEWAY-003**: `docs/decisions/DD-GATEWAY-003-redis-outage-metrics.md`

---

**Status**: 🔧 **READY TO IMPLEMENT**
**Estimated Time**: 45 minutes
**Confidence**: 90% - Clear scope, straightforward implementation



**Date**: 2025-10-26
**Status**: 🔧 **PLANNING**
**Estimated Time**: 45 minutes

---

## 🎯 **Objective**

Wire centralized metrics (`s.metrics`) into webhook handler processing flow for signal ingestion tracking.

---

## 📋 **Current State Analysis**

### **Existing Metrics in handlers.go**

The webhook handlers currently use **TWO types of metrics**:

#### **1. Basic Signal Processing Metrics** (Legacy - Individual Fields)
```go
// Lines 112, etc.
s.webhookRequestsTotal.Inc()
s.webhookErrorsTotal.Inc()
s.crdCreationTotal.Inc()
s.webhookProcessingSeconds.Observe(duration)
```

**These SHOULD be replaced with centralized metrics**:
- `s.webhookRequestsTotal` → `s.metrics.SignalsReceived`
- `s.webhookErrorsTotal` → `s.metrics.SignalsFailed`
- `s.crdCreationTotal` → `s.metrics.CRDsCreated`
- `s.webhookProcessingSeconds` → `s.metrics.ProcessingDuration`

#### **2. Redis Health Monitoring Metrics** (v2.10 - DD-GATEWAY-003)
```go
// Lines 148-152, 166-168, 176, 190-191
s.redisOperationErrorsTotal.WithLabelValues("check", "deduplication", "unavailable").Inc()
s.requestsRejectedTotal.WithLabelValues("redis_unavailable", "deduplication").Inc()
s.duplicateCRDsPreventedTotal.Inc()
s.duplicatePreventionActive.Set(1)
s.consecutive503Responses.WithLabelValues(signal.Namespace).Inc()
```

**These should REMAIN as individual fields** (not in centralized metrics):
- Part of Redis health monitoring feature (v2.10)
- Documented in DD-GATEWAY-003
- Specific to Redis failure handling
- NOT part of basic signal processing metrics

---

## 🎯 **Scope for Phase 2.4**

### **IN SCOPE** ✅
Replace these legacy metrics with centralized `s.metrics`:

| Legacy Field | Centralized Metric | Label Values |
|---|---|---|
| `s.webhookRequestsTotal` | `s.metrics.SignalsReceived` | `source`, `signal_type` |
| `s.webhookErrorsTotal` | `s.metrics.SignalsFailed` | `source`, `error_type` |
| `s.crdCreationTotal` | `s.metrics.CRDsCreated` | `namespace`, `priority` |
| `s.webhookProcessingSeconds` | `s.metrics.ProcessingDuration` | `source`, `stage` |
| N/A (new) | `s.metrics.DuplicateSignals` | `source` |
| N/A (new) | `s.metrics.SignalsProcessed` | `source`, `priority`, `environment` |

### **OUT OF SCOPE** ❌
Keep these Redis-specific metrics as-is:

- `s.redisOperationErrorsTotal` - Redis operation failures
- `s.requestsRejectedTotal` - 503 rejections
- `s.duplicateCRDsPreventedTotal` - Duplicate prevention tracking
- `s.duplicatePreventionActive` - Deduplication health gauge
- `s.consecutive503Responses` - Prometheus retry risk tracking
- `s.duration503Seconds` - 503 period duration
- `s.alertsQueuedEstimate` - Estimated queued alerts
- `s.redisMasterChangesTotal` - Sentinel failovers
- `s.redisFailoverDurationSeconds` - Failover duration

**Rationale**: These are part of DD-GATEWAY-003 (Redis Outage Risk Tracking) and should remain separate.

---

## 🔧 **Implementation Plan**

### **Step 1: Add Nil Check** (5 min)
Add nil check at the start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... rest of function
}
```

### **Step 2: Replace Legacy Metrics** (20 min)

#### **2.1: Signal Reception** (Line 112)
```go
// OLD:
s.webhookRequestsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
}
```

#### **2.2: Signal Processing Errors** (Multiple locations)
```go
// OLD:
s.webhookErrorsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsFailed.WithLabelValues(sourceType, errorType).Inc()
}
```

**Error Types**:
- `"read_error"` - Failed to read request body
- `"parse_error"` - Failed to parse webhook payload
- `"crd_creation_error"` - Failed to create CRD

#### **2.3: Duplicate Detection** (Lines 159-172)
```go
// NEW: Track duplicate detection
if s.metrics != nil {
	s.metrics.DuplicateSignals.WithLabelValues(sourceType).Inc()
}
```

#### **2.4: CRD Creation** (After successful CRD creation)
```go
// OLD:
s.crdCreationTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.CRDsCreated.WithLabelValues(signal.Namespace, priority).Inc()
}
```

#### **2.5: Processing Duration** (End of successful processing)
```go
// OLD:
s.webhookProcessingSeconds.Observe(duration)

// NEW:
if s.metrics != nil {
	s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
}
```

#### **2.6: Successful Processing** (After CRD creation)
```go
// NEW: Track successful signal processing
if s.metrics != nil {
	s.metrics.SignalsProcessed.WithLabelValues(sourceType, priority, environment).Inc()
}
```

### **Step 3: Add Processing Duration Tracking** (10 min)

Add timer at start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	start := time.Now()
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... processing logic ...

	// Track processing duration at end
	if s.metrics != nil {
		duration := time.Since(start).Seconds()
		s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
	}
}
```

### **Step 4: Verify Compilation** (5 min)
```bash
go build -o /dev/null ./pkg/gateway/...
```

### **Step 5: Run Unit Tests** (5 min)
```bash
go test -v ./test/unit/gateway/...
```

---

## ✅ **Success Criteria**

- [ ] All legacy signal processing metrics replaced with `s.metrics.*`
- [ ] Redis-specific metrics remain unchanged (DD-GATEWAY-003)
- [ ] Nil checks prevent panics when metrics disabled
- [ ] Code compiles successfully
- [ ] Unit tests pass (186/187 expected)
- [ ] No new lint errors

---

## 📊 **Metrics Mapping**

| Handler Location | Old Metric | New Metric | Labels |
|---|---|---|---|
| Line 112 | `webhookRequestsTotal` | `SignalsReceived` | `source=Prometheus/K8s`, `signal_type=webhook` |
| Line ~130 | `webhookErrorsTotal` | `SignalsFailed` | `source=Prometheus/K8s`, `error_type=parse_error` |
| Line ~159 | N/A | `DuplicateSignals` | `source=Prometheus/K8s` |
| Line ~250 | `crdCreationTotal` | `CRDsCreated` | `namespace=X`, `priority=P0-P3` |
| Line ~260 | `webhookProcessingSeconds` | `ProcessingDuration` | `source=Prometheus/K8s`, `stage=complete` |
| Line ~260 | N/A | `SignalsProcessed` | `source=Prometheus/K8s`, `priority=P0-P3`, `environment=prod/staging/dev` |

---

## 🚨 **Important Notes**

### **Redis Metrics Are Separate**
The Redis health monitoring metrics (v2.10 - DD-GATEWAY-003) are **intentionally separate** from the centralized metrics:
- They track Redis-specific failure scenarios
- They're part of the Redis outage risk tracking feature
- They're documented in `REDIS_OUTAGE_METRICS.md`
- They should NOT be moved to centralized metrics

### **Nil Safety**
All metrics calls MUST have `if s.metrics != nil` checks to prevent panics when metrics are disabled (e.g., in tests).

### **Label Consistency**
- `source`: "Prometheus AlertManager" or "Kubernetes Event"
- `signal_type`: "webhook"
- `error_type`: "read_error", "parse_error", "crd_creation_error"
- `priority`: "P0", "P1", "P2", "P3"
- `environment`: "production", "staging", "development", "unknown"

---

## 🔗 **Related Files**

- **Handler**: `pkg/gateway/server/handlers.go`
- **Metrics**: `pkg/gateway/metrics/metrics.go`
- **Server**: `pkg/gateway/server/server.go`
- **Redis Metrics Doc**: `docs/services/stateless/gateway-service/REDIS_OUTAGE_METRICS.md`
- **DD-GATEWAY-003**: `docs/decisions/DD-GATEWAY-003-redis-outage-metrics.md`

---

**Status**: 🔧 **READY TO IMPLEMENT**
**Estimated Time**: 45 minutes
**Confidence**: 90% - Clear scope, straightforward implementation

# Day 9 Phase 2.4: Webhook Handler Metrics Integration

**Date**: 2025-10-26
**Status**: 🔧 **PLANNING**
**Estimated Time**: 45 minutes

---

## 🎯 **Objective**

Wire centralized metrics (`s.metrics`) into webhook handler processing flow for signal ingestion tracking.

---

## 📋 **Current State Analysis**

### **Existing Metrics in handlers.go**

The webhook handlers currently use **TWO types of metrics**:

#### **1. Basic Signal Processing Metrics** (Legacy - Individual Fields)
```go
// Lines 112, etc.
s.webhookRequestsTotal.Inc()
s.webhookErrorsTotal.Inc()
s.crdCreationTotal.Inc()
s.webhookProcessingSeconds.Observe(duration)
```

**These SHOULD be replaced with centralized metrics**:
- `s.webhookRequestsTotal` → `s.metrics.SignalsReceived`
- `s.webhookErrorsTotal` → `s.metrics.SignalsFailed`
- `s.crdCreationTotal` → `s.metrics.CRDsCreated`
- `s.webhookProcessingSeconds` → `s.metrics.ProcessingDuration`

#### **2. Redis Health Monitoring Metrics** (v2.10 - DD-GATEWAY-003)
```go
// Lines 148-152, 166-168, 176, 190-191
s.redisOperationErrorsTotal.WithLabelValues("check", "deduplication", "unavailable").Inc()
s.requestsRejectedTotal.WithLabelValues("redis_unavailable", "deduplication").Inc()
s.duplicateCRDsPreventedTotal.Inc()
s.duplicatePreventionActive.Set(1)
s.consecutive503Responses.WithLabelValues(signal.Namespace).Inc()
```

**These should REMAIN as individual fields** (not in centralized metrics):
- Part of Redis health monitoring feature (v2.10)
- Documented in DD-GATEWAY-003
- Specific to Redis failure handling
- NOT part of basic signal processing metrics

---

## 🎯 **Scope for Phase 2.4**

### **IN SCOPE** ✅
Replace these legacy metrics with centralized `s.metrics`:

| Legacy Field | Centralized Metric | Label Values |
|---|---|---|
| `s.webhookRequestsTotal` | `s.metrics.SignalsReceived` | `source`, `signal_type` |
| `s.webhookErrorsTotal` | `s.metrics.SignalsFailed` | `source`, `error_type` |
| `s.crdCreationTotal` | `s.metrics.CRDsCreated` | `namespace`, `priority` |
| `s.webhookProcessingSeconds` | `s.metrics.ProcessingDuration` | `source`, `stage` |
| N/A (new) | `s.metrics.DuplicateSignals` | `source` |
| N/A (new) | `s.metrics.SignalsProcessed` | `source`, `priority`, `environment` |

### **OUT OF SCOPE** ❌
Keep these Redis-specific metrics as-is:

- `s.redisOperationErrorsTotal` - Redis operation failures
- `s.requestsRejectedTotal` - 503 rejections
- `s.duplicateCRDsPreventedTotal` - Duplicate prevention tracking
- `s.duplicatePreventionActive` - Deduplication health gauge
- `s.consecutive503Responses` - Prometheus retry risk tracking
- `s.duration503Seconds` - 503 period duration
- `s.alertsQueuedEstimate` - Estimated queued alerts
- `s.redisMasterChangesTotal` - Sentinel failovers
- `s.redisFailoverDurationSeconds` - Failover duration

**Rationale**: These are part of DD-GATEWAY-003 (Redis Outage Risk Tracking) and should remain separate.

---

## 🔧 **Implementation Plan**

### **Step 1: Add Nil Check** (5 min)
Add nil check at the start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... rest of function
}
```

### **Step 2: Replace Legacy Metrics** (20 min)

#### **2.1: Signal Reception** (Line 112)
```go
// OLD:
s.webhookRequestsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
}
```

#### **2.2: Signal Processing Errors** (Multiple locations)
```go
// OLD:
s.webhookErrorsTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.SignalsFailed.WithLabelValues(sourceType, errorType).Inc()
}
```

**Error Types**:
- `"read_error"` - Failed to read request body
- `"parse_error"` - Failed to parse webhook payload
- `"crd_creation_error"` - Failed to create CRD

#### **2.3: Duplicate Detection** (Lines 159-172)
```go
// NEW: Track duplicate detection
if s.metrics != nil {
	s.metrics.DuplicateSignals.WithLabelValues(sourceType).Inc()
}
```

#### **2.4: CRD Creation** (After successful CRD creation)
```go
// OLD:
s.crdCreationTotal.Inc()

// NEW:
if s.metrics != nil {
	s.metrics.CRDsCreated.WithLabelValues(signal.Namespace, priority).Inc()
}
```

#### **2.5: Processing Duration** (End of successful processing)
```go
// OLD:
s.webhookProcessingSeconds.Observe(duration)

// NEW:
if s.metrics != nil {
	s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
}
```

#### **2.6: Successful Processing** (After CRD creation)
```go
// NEW: Track successful signal processing
if s.metrics != nil {
	s.metrics.SignalsProcessed.WithLabelValues(sourceType, priority, environment).Inc()
}
```

### **Step 3: Add Processing Duration Tracking** (10 min)

Add timer at start of `processWebhook`:

```go
func (s *Server) processWebhook(w http.ResponseWriter, r *http.Request, adapterName string, sourceType string) {
	start := time.Now()
	ctx := r.Context()
	requestID := middleware.GetReqID(ctx)

	// Track signal reception (Day 9 Phase 2.4)
	if s.metrics != nil {
		s.metrics.SignalsReceived.WithLabelValues(sourceType, "webhook").Inc()
	}

	// ... processing logic ...

	// Track processing duration at end
	if s.metrics != nil {
		duration := time.Since(start).Seconds()
		s.metrics.ProcessingDuration.WithLabelValues(sourceType, "complete").Observe(duration)
	}
}
```

### **Step 4: Verify Compilation** (5 min)
```bash
go build -o /dev/null ./pkg/gateway/...
```

### **Step 5: Run Unit Tests** (5 min)
```bash
go test -v ./test/unit/gateway/...
```

---

## ✅ **Success Criteria**

- [ ] All legacy signal processing metrics replaced with `s.metrics.*`
- [ ] Redis-specific metrics remain unchanged (DD-GATEWAY-003)
- [ ] Nil checks prevent panics when metrics disabled
- [ ] Code compiles successfully
- [ ] Unit tests pass (186/187 expected)
- [ ] No new lint errors

---

## 📊 **Metrics Mapping**

| Handler Location | Old Metric | New Metric | Labels |
|---|---|---|---|
| Line 112 | `webhookRequestsTotal` | `SignalsReceived` | `source=Prometheus/K8s`, `signal_type=webhook` |
| Line ~130 | `webhookErrorsTotal` | `SignalsFailed` | `source=Prometheus/K8s`, `error_type=parse_error` |
| Line ~159 | N/A | `DuplicateSignals` | `source=Prometheus/K8s` |
| Line ~250 | `crdCreationTotal` | `CRDsCreated` | `namespace=X`, `priority=P0-P3` |
| Line ~260 | `webhookProcessingSeconds` | `ProcessingDuration` | `source=Prometheus/K8s`, `stage=complete` |
| Line ~260 | N/A | `SignalsProcessed` | `source=Prometheus/K8s`, `priority=P0-P3`, `environment=prod/staging/dev` |

---

## 🚨 **Important Notes**

### **Redis Metrics Are Separate**
The Redis health monitoring metrics (v2.10 - DD-GATEWAY-003) are **intentionally separate** from the centralized metrics:
- They track Redis-specific failure scenarios
- They're part of the Redis outage risk tracking feature
- They're documented in `REDIS_OUTAGE_METRICS.md`
- They should NOT be moved to centralized metrics

### **Nil Safety**
All metrics calls MUST have `if s.metrics != nil` checks to prevent panics when metrics are disabled (e.g., in tests).

### **Label Consistency**
- `source`: "Prometheus AlertManager" or "Kubernetes Event"
- `signal_type`: "webhook"
- `error_type`: "read_error", "parse_error", "crd_creation_error"
- `priority`: "P0", "P1", "P2", "P3"
- `environment`: "production", "staging", "development", "unknown"

---

## 🔗 **Related Files**

- **Handler**: `pkg/gateway/server/handlers.go`
- **Metrics**: `pkg/gateway/metrics/metrics.go`
- **Server**: `pkg/gateway/server/server.go`
- **Redis Metrics Doc**: `docs/services/stateless/gateway-service/REDIS_OUTAGE_METRICS.md`
- **DD-GATEWAY-003**: `docs/decisions/DD-GATEWAY-003-redis-outage-metrics.md`

---

**Status**: 🔧 **READY TO IMPLEMENT**
**Estimated Time**: 45 minutes
**Confidence**: 90% - Clear scope, straightforward implementation




