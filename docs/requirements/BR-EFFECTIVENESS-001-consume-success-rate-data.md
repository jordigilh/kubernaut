# BR-EFFECTIVENESS-001: Consume Success Rate Data

> **ARCHIVED** (February 2026)
>
> This BR describes a Context API polling design that has been **superseded** by DD-017 v2.0.
> DD-017 v2.0 defines the authoritative EM Level 1 architecture: a Kubernetes controller
> watching RemediationRequest CRDs, performing automated assessment, and emitting structured
> audit events to DataStorage. The Context API polling and trend table design described here
> is NOT being implemented.
>
> **Authoritative source**: `docs/architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md` (v2.0)
> **V1.0 BRs**: BR-INS-001, BR-INS-002, BR-INS-005 (from DD-017 v2.0)

**Business Requirement ID**: BR-EFFECTIVENESS-001
**Category**: Effectiveness Monitor Service
**Priority**: P1
**Target Version**: V1
**Status**: ⚠️ ARCHIVED — Superseded by DD-017 v2.0
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 introduces multi-dimensional success tracking across incident types and playbooks. The Effectiveness Monitor Service must consume historical success rate data from Context API to calculate trends, identify degrading playbooks, and trigger learning feedback loops for continuous AI improvement.

**Current Limitations**:
- ❌ No service consuming success rate data for trend analysis
- ❌ Cannot detect when playbook effectiveness degrades over time
- ❌ No continuous learning feedback to AI Service
- ❌ Manual analysis required to identify playbook optimization opportunities
- ❌ No automated detection of successful vs failing remediation patterns

**Impact**:
- Cannot implement ADR-033 continuous learning loops
- Playbook degradation goes undetected (e.g., 89% → 60% success rate)
- AI cannot adapt to changing infrastructure conditions
- Missing foundation for automated playbook improvement

---

## 🎯 **Business Objective**

**Enable Effectiveness Monitor to query Context API for incident-type and playbook success rates, store historical trend data, and provide effectiveness dashboards for continuous remediation improvement.**

### **Success Criteria**
1. ✅ Effectiveness Monitor polls Context API every 5 minutes for success rates
2. ✅ Stores historical trend data in internal PostgreSQL database
3. ✅ Tracks success rate changes over time (7d, 30d, 90d windows)
4. ✅ Provides REST API to query effectiveness trends
5. ✅ Dashboard displays incident-type and playbook effectiveness charts
6. ✅ Historical data retained for 180 days (6 months)
7. ✅ Data collection latency <10 seconds (5-minute polling + processing)

---

## 📊 **Use Cases**

### **Use Case 1: Detect Playbook Effectiveness Degradation**

**Scenario**: `pod-oom-recovery v1.2` had 89% success rate last week but dropped to 60% this week due to infrastructure changes.

**Current Flow** (Without BR-EFFECTIVENESS-001):
```
1. Playbook success rate degrades from 89% to 60%
2. No monitoring of success rate trends
3. ❌ Degradation goes undetected
4. ❌ AI continues selecting degraded playbook
5. ❌ Poor remediation outcomes for days/weeks
6. ❌ Manual investigation required after user complaints
```

**Desired Flow with BR-EFFECTIVENESS-001**:
```
1. Effectiveness Monitor polls Context API every 5 minutes:
   GET /incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.2
2. Effectiveness Monitor stores trend data:
   Week 1: 89% success (90 executions)
   Week 2: 60% success (50 executions)  ← 29% drop detected
3. Effectiveness Monitor calculates trend: "declining"
4. Effectiveness Monitor stores trend analysis:
   {
     "playbook_id": "pod-oom-recovery",
     "playbook_version": "v1.2",
     "trend": "declining",
     "current_success_rate": 0.60,
     "previous_success_rate": 0.89,
     "change_percent": -32.6,
     "threshold_exceeded": true  // >10% drop
   }
5. ✅ Dashboard shows degradation alert
6. ✅ Team investigates root cause (infrastructure change)
7. ✅ Team creates v1.3 to adapt to new conditions
8. ✅ Automated detection vs manual discovery
```

---

### **Use Case 2: Historical Trend Analysis**

**Scenario**: Team wants to analyze `container-restart-playbook` effectiveness over last 90 days.

**Current Flow**:
```
1. Team wants 90-day trend data
2. No historical data stored
3. ❌ Cannot query past success rates (Data Storage only has raw execution data)
4. ❌ Manual data export and analysis required
```

**Desired Flow with BR-EFFECTIVENESS-001**:
```
1. Team queries Effectiveness Monitor API:
   GET /api/v1/effectiveness/trends/playbook?playbook_id=container-restart-playbook&playbook_version=v1.0&time_range=90d
2. Effectiveness Monitor returns historical trend:
   {
     "playbook_id": "container-restart-playbook",
     "playbook_version": "v1.0",
     "time_range": "90d",
     "data_points": [
       {"date": "2025-08-05", "success_rate": 0.85, "executions": 100},
       {"date": "2025-08-12", "success_rate": 0.87, "executions": 110},
       ...
       {"date": "2025-11-05", "success_rate": 0.92, "executions": 150}
     ],
     "trend": "improving",
     "change_percent": +8.2
   }
3. ✅ Dashboard renders 90-day trend chart
4. ✅ Team validates: Playbook improving over time
5. ✅ Data-driven playbook promotion decision
```

---

### **Use Case 3: Cross-Playbook Comparison**

**Scenario**: Team wants to compare effectiveness of 3 playbooks for `pod-oom-killer` incident type.

**Current Flow**:
```
1. Team wants to compare 3 playbooks
2. No cross-playbook comparison available
3. ❌ Manual queries to Data Storage for each playbook
4. ❌ Time-consuming comparison
```

**Desired Flow with BR-EFFECTIVENESS-001**:
```
1. Team queries Effectiveness Monitor:
   GET /api/v1/effectiveness/compare/playbooks?incident_type=pod-oom-killer&time_range=30d
2. Effectiveness Monitor returns comparison:
   {
     "incident_type": "pod-oom-killer",
     "time_range": "30d",
     "playbooks": [
       {
         "playbook_id": "pod-oom-recovery",
         "playbook_version": "v1.2",
         "success_rate": 0.89,
         "executions": 200,
         "trend": "stable"
       },
       {
         "playbook_id": "pod-oom-recovery",
         "playbook_version": "v1.1",
         "success_rate": 0.40,
         "executions": 10,
         "trend": "stable"
       },
       {
         "playbook_id": "pod-oom-vertical-scaling",
         "playbook_version": "v1.0",
         "success_rate": 0.75,
         "executions": 50,
         "trend": "improving"
       }
     ]
   }
3. ✅ Dashboard renders playbook comparison chart
4. ✅ Team identifies: v1.2 is best (89%), v1.1 should be deprecated (40%)
5. ✅ Data-driven playbook selection and deprecation decisions
```

---

## 🔧 **Functional Requirements**

### **FR-EFFECTIVENESS-001-01: Context API Polling**

**Requirement**: Effectiveness Monitor SHALL poll Context API every 5 minutes to collect success rate data.

**Implementation Example**:
```go
package effectivenessmonitor

// DataCollector polls Context API for success rate data
type DataCollector struct {
    contextAPIClient ContextAPIClient
    repository       EffectivenessRepository
    logger           *zap.Logger
}

// StartPolling initiates 5-minute polling of Context API
func (dc *DataCollector) StartPolling(ctx context.Context) error {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := dc.collectSuccessRates(ctx); err != nil {
                dc.logger.Error("failed to collect success rates", zap.Error(err))
                // Continue polling (don't exit on error)
            }
        }
    }
}

// collectSuccessRates queries Context API and stores trend data
func (dc *DataCollector) collectSuccessRates(ctx context.Context) error {
    // Step 1: Query all incident types
    incidentTypes, err := dc.getActiveIncidentTypes(ctx)
    if err != nil {
        return fmt.Errorf("failed to get incident types: %w", err)
    }

    // Step 2: Query success rate for each incident type
    for _, incidentType := range incidentTypes {
        successRate, err := dc.contextAPIClient.GetSuccessRateByIncidentType(ctx, incidentType, "7d")
        if err != nil {
            dc.logger.Warn("failed to get success rate",
                zap.String("incident_type", incidentType),
                zap.Error(err))
            continue
        }

        // Step 3: Store trend data
        if err := dc.repository.StoreSuccessRateTrend(ctx, incidentType, successRate); err != nil {
            dc.logger.Error("failed to store trend data",
                zap.String("incident_type", incidentType),
                zap.Error(err))
        }
    }

    // Step 4: Query all active playbooks
    playbooks, err := dc.getActivePlaybooks(ctx)
    if err != nil {
        return fmt.Errorf("failed to get playbooks: %w", err)
    }

    // Step 5: Query success rate for each playbook
    for _, playbook := range playbooks {
        successRate, err := dc.contextAPIClient.GetSuccessRateByPlaybook(ctx, playbook.PlaybookID, playbook.Version, "7d")
        if err != nil {
            dc.logger.Warn("failed to get playbook success rate",
                zap.String("playbook_id", playbook.PlaybookID),
                zap.String("version", playbook.Version),
                zap.Error(err))
            continue
        }

        // Step 6: Store playbook trend data
        if err := dc.repository.StorePlaybookSuccessRateTrend(ctx, playbook.PlaybookID, playbook.Version, successRate); err != nil {
            dc.logger.Error("failed to store playbook trend",
                zap.String("playbook_id", playbook.PlaybookID),
                zap.Error(err))
        }
    }

    dc.logger.Info("success rate collection complete",
        zap.Int("incident_types", len(incidentTypes)),
        zap.Int("playbooks", len(playbooks)))

    return nil
}
```

**Acceptance Criteria**:
- ✅ Polls Context API every 5 minutes (not 1 minute to reduce load)
- ✅ Continues polling on errors (resilient to transient failures)
- ✅ Queries both incident-type and playbook success rates
- ✅ Logs collection success/failure with counts
- ✅ Context cancellation stops polling gracefully

---

### **FR-EFFECTIVENESS-001-02: Historical Trend Storage**

**Requirement**: Effectiveness Monitor SHALL store historical trend data in PostgreSQL with 180-day retention.

**Database Schema**:
```sql
-- Incident-type success rate trends
CREATE TABLE incident_type_effectiveness_trends (
    id SERIAL PRIMARY KEY,
    incident_type VARCHAR(100) NOT NULL,
    collected_at TIMESTAMP NOT NULL DEFAULT NOW(),
    time_range VARCHAR(10) NOT NULL,  -- "7d", "30d", "90d"
    success_rate FLOAT NOT NULL,
    total_executions INT NOT NULL,
    successful_executions INT NOT NULL,
    failed_executions INT NOT NULL,
    confidence VARCHAR(20) NOT NULL,

    INDEX idx_incident_type (incident_type, collected_at DESC)
);

-- Playbook success rate trends
CREATE TABLE playbook_effectiveness_trends (
    id SERIAL PRIMARY KEY,
    playbook_id VARCHAR(64) NOT NULL,
    playbook_version VARCHAR(20) NOT NULL,
    collected_at TIMESTAMP NOT NULL DEFAULT NOW(),
    time_range VARCHAR(10) NOT NULL,
    success_rate FLOAT NOT NULL,
    total_executions INT NOT NULL,
    successful_executions INT NOT NULL,
    failed_executions INT NOT NULL,
    confidence VARCHAR(20) NOT NULL,

    INDEX idx_playbook (playbook_id, playbook_version, collected_at DESC)
);

-- Retention policy: Delete data older than 180 days
CREATE OR REPLACE FUNCTION cleanup_old_trends() RETURNS void AS $$
BEGIN
    DELETE FROM incident_type_effectiveness_trends WHERE collected_at < NOW() - INTERVAL '180 days';
    DELETE FROM playbook_effectiveness_trends WHERE collected_at < NOW() - INTERVAL '180 days';
END;
$$ LANGUAGE plpgsql;
```

**Acceptance Criteria**:
- ✅ Stores timestamp for each data collection
- ✅ Retains 180 days of historical data
- ✅ Automatic cleanup of data older than 180 days (daily job)
- ✅ Indexes on incident_type and playbook_id for fast queries
- ✅ Success rate stored as FLOAT (0.0-1.0)

---

### **FR-EFFECTIVENESS-001-03: Trend Query API**

**Requirement**: Effectiveness Monitor SHALL provide REST API to query historical trends.

**API Specification**:
```http
GET /api/v1/effectiveness/trends/incident-type

Query Parameters:
- incident_type (string, required): Incident type (e.g., "pod-oom-killer")
- time_range (string, optional, default: "30d"): Historical window (7d, 30d, 90d)
- granularity (string, optional, default: "daily"): Data point granularity (hourly, daily, weekly)

Response (200 OK):
{
  "incident_type": "pod-oom-killer",
  "time_range": "30d",
  "granularity": "daily",
  "data_points": [
    {
      "date": "2025-10-05",
      "success_rate": 0.85,
      "total_executions": 100,
      "trend_direction": "stable"
    },
    {
      "date": "2025-10-12",
      "success_rate": 0.87,
      "total_executions": 110,
      "trend_direction": "improving"
    },
    ...
    {
      "date": "2025-11-05",
      "success_rate": 0.89,
      "total_executions": 120,
      "trend_direction": "improving"
    }
  ],
  "overall_trend": "improving",
  "change_percent": +4.7
}

---

GET /api/v1/effectiveness/trends/playbook

Query Parameters:
- playbook_id (string, required): Playbook identifier
- playbook_version (string, required): Playbook version
- time_range (string, optional, default: "30d")
- granularity (string, optional, default: "daily")

Response (200 OK):
{
  "playbook_id": "pod-oom-recovery",
  "playbook_version": "v1.2",
  "time_range": "30d",
  "granularity": "daily",
  "data_points": [...],
  "overall_trend": "stable",
  "change_percent": +1.2
}
```

**Acceptance Criteria**:
- ✅ Returns 200 OK for valid queries
- ✅ Returns 400 Bad Request for missing required parameters
- ✅ Data points sorted by date ASC (oldest first)
- ✅ Calculates overall_trend (improving/stable/declining)
- ✅ Calculates change_percent (first vs last data point)

---

## 📈 **Non-Functional Requirements**

### **NFR-EFFECTIVENESS-001-01: Performance**

- ✅ Data collection completes within 60 seconds (all incident types + playbooks)
- ✅ Trend query response time <200ms
- ✅ Database storage <1GB for 180 days of data

### **NFR-EFFECTIVENESS-001-02: Reliability**

- ✅ Polling continues on Context API failures (resilient to transient errors)
- ✅ Database connection pooling (10 connections)
- ✅ Retry logic for Context API queries (3 retries with exponential backoff)

### **NFR-EFFECTIVENESS-001-03: Scalability**

- ✅ Support 100+ incident types
- ✅ Support 500+ active playbooks
- ✅ Handle 50 concurrent trend queries

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Defines continuous learning feedback loops
- ✅ BR-INTEGRATION-008: Context API incident-type success rate endpoint
- ✅ BR-INTEGRATION-009: Context API playbook success rate endpoint
- ✅ PostgreSQL: For trend data storage

### **Downstream Impacts**
- ✅ BR-EFFECTIVENESS-002: Uses historical trend data to calculate playbook effectiveness
- ✅ Operations Dashboard: Queries trend API for effectiveness charts

---

## 🚀 **Implementation Phases**

### **Phase 1: Database Schema** (Day 14 - 3 hours)
- Create PostgreSQL tables (incident_type_effectiveness_trends, playbook_effectiveness_trends)
- Add indexes and retention policy
- Migration script

### **Phase 2: Context API Client** (Day 14 - 3 hours)
- Implement Context API client for success rate queries
- Add retry logic and timeout handling
- Unit tests

### **Phase 3: Data Collection** (Day 15 - 4 hours)
- Implement `DataCollector` with 5-minute polling
- Implement `StoreSuccessRateTrend()` repository method
- Integration tests with real Context API

### **Phase 4: Trend Query API** (Day 15 - 4 hours)
- Implement `GET /trends/incident-type` endpoint
- Implement `GET /trends/playbook` endpoint
- Add trend calculation logic (improving/stable/declining)

### **Phase 5: Monitoring** (Day 16 - 2 hours)
- Add Prometheus metrics: `effectiveness_monitor_collections_total`, `effectiveness_monitor_collection_duration_seconds`
- Add alerting for collection failures (>5% failure rate)

**Total Estimated Effort**: 16 hours (2 days)

---

## 📊 **Success Metrics**

### **Data Collection Success Rate**
- **Target**: 95%+ of collections succeed
- **Measure**: `effectiveness_monitor_collections_total{status="success"}` / total

### **Trend Query Usage**
- **Target**: 100+ queries per day from dashboard
- **Measure**: Track API request count

### **Data Retention Compliance**
- **Target**: 100% of data retained for 180 days, 0% beyond 180 days
- **Measure**: Query oldest and newest data points daily

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Real-Time Streaming Instead of Polling**

**Approach**: Context API streams success rate updates to Effectiveness Monitor

**Rejected Because**:
- ❌ Over-engineering: 5-minute polling is sufficient for trend analysis
- ❌ Increased complexity (streaming infrastructure)
- ❌ No significant benefit for 5-minute granularity

---

### **Alternative 2: Store Data in Time-Series Database**

**Approach**: Use InfluxDB or Prometheus for trend data storage

**Rejected Because**:
- ❌ Additional infrastructure dependency
- ❌ PostgreSQL sufficient for 180-day retention
- ❌ Team expertise in PostgreSQL

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P1 priority (enables continuous learning feedback loops)
**Rationale**: Foundation for ADR-033 continuous improvement and automated playbook optimization
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-INTEGRATION-008: Context API incident-type success rate endpoint
- BR-INTEGRATION-009: Context API playbook success rate endpoint
- BR-EFFECTIVENESS-002: Calculate playbook effectiveness trends
- BR-EFFECTIVENESS-003: Trigger learning feedback loops

### **Related Documents**
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-037: BR Template Standard](../architecture/decisions/ADR-037-business-requirement-template-standard.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

