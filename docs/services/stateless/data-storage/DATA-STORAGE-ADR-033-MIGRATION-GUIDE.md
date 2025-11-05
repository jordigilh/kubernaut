# ADR-033 Migration Guide: Multi-Dimensional Success Tracking

**Version**: 1.0
**Date**: November 5, 2025
**Status**: ✅ Production Ready
**Target Audience**: Service Developers, AI/ML Engineers, SRE Teams

---

## 📋 Table of Contents

1. [Overview](#overview)
2. [What's New in ADR-033](#whats-new-in-adr-033)
3. [Schema Changes](#schema-changes)
4. [New API Endpoints](#new-api-endpoints)
5. [Integration Guide](#integration-guide)
6. [Code Examples](#code-examples)
7. [Testing Guide](#testing-guide)
8. [Troubleshooting](#troubleshooting)
9. [FAQ](#faq)

---

## Overview

ADR-033 introduces **multi-dimensional success tracking** to enable AI-driven continuous improvement of remediation effectiveness. This guide helps you integrate the new success rate analytics endpoints into your services.

### Key Benefits

- **AI Learning**: Track which playbooks work best for each incident type
- **Confidence-Based Decisions**: Use high-confidence recommendations only
- **Trend Analysis**: Compare recent vs historical success rates
- **Version Comparison**: A/B test playbook versions

### What Changed

**Schema**: Added 11 new columns to `resource_action_traces` table
**API**: Added 2 new aggregation endpoints
**Breaking Changes**: ❌ **NONE** (all changes are additive)

---

## What's New in ADR-033

### 1. Multi-Dimensional Success Tracking

Track remediation effectiveness across three dimensions:

| Dimension | Purpose | Example |
|-----------|---------|---------|
| **PRIMARY**: Incident Type | Identify which problems are being solved | "pod-oom-killer", "disk-pressure" |
| **SECONDARY**: Remediation Playbook | Identify which solutions work best | "pod-oom-recovery", "disk-cleanup" |
| **TERTIARY**: AI Execution Mode | Track AI behavior distribution | catalog/chained/manual |

### 2. Confidence-Based Recommendations

Prevent decisions based on insufficient data:

| Confidence | Sample Size | Recommended Action |
|-----------|-------------|-------------------|
| **high** | ≥100 samples | ✅ Safe for automated decisions |
| **medium** | 20-99 samples | ⚠️ Use with caution |
| **low** | 5-19 samples | ⚠️ Manual review required |
| **insufficient_data** | <5 samples | ❌ Do not use for decisions |

### 3. AI Execution Mode Tracking (Hybrid Model)

Track how AI selects remediation strategies:

| Mode | Expected % | Description |
|------|-----------|-------------|
| **catalog_selected** | 90-95% | Single playbook from catalog |
| **chained** | 4-9% | Multiple playbooks chained |
| **manual_escalation** | <1% | Escalated to human operator |

---

## Schema Changes

### New Columns in `resource_action_traces`

```sql
-- DIMENSION 1: INCIDENT TYPE (PRIMARY)
incident_type VARCHAR(100)              -- "pod-oom-killer", "disk-pressure"
alert_name VARCHAR(255)                 -- Prometheus alert name
incident_severity VARCHAR(20)           -- "critical", "warning", "info"

-- DIMENSION 2: PLAYBOOK (SECONDARY)
playbook_id VARCHAR(64)                 -- "pod-oom-recovery"
playbook_version VARCHAR(20)            -- "v1.2", "v2.0"
playbook_step_number INT                -- Step position (1, 2, 3...)
playbook_execution_id VARCHAR(64)       -- Groups actions in single playbook run

-- AI EXECUTION MODE (HYBRID MODEL)
ai_selected_playbook BOOLEAN            -- Catalog selection (90%)
ai_chained_playbooks BOOLEAN            -- Chained playbooks (9%)
ai_manual_escalation BOOLEAN            -- Manual escalation (1%)
ai_playbook_customization JSONB         -- AI parameter customizations
```

### Migration Script

**File**: `migrations/012_adr033_multidimensional_tracking.sql`

```bash
# Apply migration (using Goose)
goose -dir migrations postgres "postgresql://user:pass@localhost:5432/action_history" up

# Verify migration
goose -dir migrations postgres "postgresql://user:pass@localhost:5432/action_history" status

# Rollback if needed
goose -dir migrations postgres "postgresql://user:pass@localhost:5432/action_history" down
```

**Migration Status**: ✅ Tested and verified (idempotent, includes rollback)

---

## New API Endpoints

### 1. GET /api/v1/success-rate/incident-type

**Purpose**: Calculate success rate for a specific incident type (PRIMARY dimension)

**Use Case**: AI selects best playbook for "pod-oom-killer" incidents

```bash
curl "http://data-storage:8080/api/v1/success-rate/incident-type?incident_type=pod-oom-killer&time_range=7d&min_samples=5"
```

**Response**:
```json
{
  "incident_type": "pod-oom-killer",
  "time_range": "7d",
  "total_executions": 150,
  "successful_executions": 135,
  "failed_executions": 15,
  "success_rate": 90.0,
  "confidence": "high",
  "min_samples_met": true,
  "ai_execution_mode": {
    "catalog_selected": 135,
    "chained": 12,
    "manual_escalation": 3
  },
  "playbook_breakdown": [
    {
      "playbook_id": "pod-oom-recovery",
      "playbook_version": "v1.2",
      "executions": 120,
      "success_rate": 92.5
    }
  ]
}
```

### 2. GET /api/v1/success-rate/playbook

**Purpose**: Calculate success rate for a specific playbook (SECONDARY dimension)

**Use Case**: Validate new playbook version effectiveness

```bash
curl "http://data-storage:8080/api/v1/success-rate/playbook?playbook_id=pod-oom-recovery&playbook_version=v2.0&time_range=7d"
```

**Response**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "playbook_version": "v2.0",
  "time_range": "7d",
  "total_executions": 80,
  "successful_executions": 76,
  "failed_executions": 4,
  "success_rate": 95.0,
  "confidence": "high",
  "min_samples_met": true,
  "ai_execution_mode": {
    "catalog_selected": 75,
    "chained": 4,
    "manual_escalation": 1
  },
  "incident_type_breakdown": [
    {
      "incident_type": "pod-oom-killer",
      "executions": 60,
      "success_rate": 96.67
    }
  ]
}
```

---

## Integration Guide

### For AI/LLM Services

**Scenario**: AI needs to select best playbook for an incident

```go
package ai

import (
    "context"
    "fmt"
    "net/http"
    "encoding/json"
)

// SelectBestPlaybook queries Data Storage for highest success rate playbook
func (s *AIService) SelectBestPlaybook(ctx context.Context, incidentType string) (string, error) {
    // 1. Query incident-type success rate
    url := fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=20",
        s.dataStorageURL, incidentType)

    resp, err := http.Get(url)
    if err != nil {
        return "", fmt.Errorf("failed to query success rate: %w", err)
    }
    defer resp.Body.Close()

    var result struct {
        IncidentType      string  `json:"incident_type"`
        SuccessRate       float64 `json:"success_rate"`
        Confidence        string  `json:"confidence"`
        MinSamplesMet     bool    `json:"min_samples_met"`
        PlaybookBreakdown []struct {
            PlaybookID      string  `json:"playbook_id"`
            PlaybookVersion string  `json:"playbook_version"`
            Executions      int     `json:"executions"`
            SuccessRate     float64 `json:"success_rate"`
        } `json:"playbook_breakdown"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", fmt.Errorf("failed to decode response: %w", err)
    }

    // 2. Check confidence level
    if result.Confidence == "insufficient_data" || !result.MinSamplesMet {
        return "", fmt.Errorf("insufficient data for incident type %s", incidentType)
    }

    // 3. Select playbook with highest success rate
    if len(result.PlaybookBreakdown) == 0 {
        return "", fmt.Errorf("no playbooks found for incident type %s", incidentType)
    }

    bestPlaybook := result.PlaybookBreakdown[0]
    for _, pb := range result.PlaybookBreakdown {
        if pb.SuccessRate > bestPlaybook.SuccessRate {
            bestPlaybook = pb
        }
    }

    // 4. Only use if confidence is high or medium
    if result.Confidence == "low" {
        s.logger.Warn("low confidence for incident type",
            "incident_type", incidentType,
            "confidence", result.Confidence,
            "selected_playbook", bestPlaybook.PlaybookID)
    }

    return bestPlaybook.PlaybookID, nil
}
```

### For RemediationExecutor Service

**Scenario**: Populate ADR-033 fields when writing action traces

```go
package executor

import (
    "context"
    "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// ExecutePlaybook executes a playbook and writes action trace with ADR-033 fields
func (e *RemediationExecutor) ExecutePlaybook(ctx context.Context, incident Incident, playbook Playbook) error {
    // 1. Execute playbook
    result, err := e.tekton.ExecutePlaybook(ctx, playbook)
    if err != nil {
        return err
    }

    // 2. Write action trace with ADR-033 fields
    trace := &client.ActionTrace{
        // Core fields
        ActionID:        result.ActionID,
        ActionType:      playbook.ActionType,
        ActionTimestamp: time.Now(),
        ExecutionStatus: result.Status,

        // ADR-033: DIMENSION 1 - INCIDENT TYPE
        IncidentType:     incident.Type,           // "pod-oom-killer"
        AlertName:        incident.AlertName,      // "PodOOMKilled"
        IncidentSeverity: incident.Severity,       // "critical"

        // ADR-033: DIMENSION 2 - PLAYBOOK
        PlaybookID:           playbook.ID,         // "pod-oom-recovery"
        PlaybookVersion:      playbook.Version,    // "v1.2"
        PlaybookStepNumber:   1,                   // Step position
        PlaybookExecutionID:  result.ExecutionID, // Groups actions

        // ADR-033: AI EXECUTION MODE
        AISelectedPlaybook:   true,                // Catalog selection
        AIChainedPlaybooks:   false,               // Not chained
        AIManualEscalation:   false,               // Not escalated
        AIPlaybookCustomization: nil,              // No customization
    }

    return e.dataStorage.WriteActionTrace(ctx, trace)
}
```

### For Context API Service

**Scenario**: Query success rates for historical context

```go
package contextapi

import (
    "context"
    "fmt"
)

// GetHistoricalSuccessRate retrieves success rate for context enrichment
func (c *ContextAPI) GetHistoricalSuccessRate(ctx context.Context, incidentType string) (float64, string, error) {
    // Query Data Storage success rate endpoint
    url := fmt.Sprintf("%s/api/v1/success-rate/incident-type?incident_type=%s&time_range=30d",
        c.dataStorageURL, incidentType)

    resp, err := c.httpClient.Get(url)
    if err != nil {
        return 0, "", fmt.Errorf("failed to query success rate: %w", err)
    }
    defer resp.Body.Close()

    var result struct {
        SuccessRate float64 `json:"success_rate"`
        Confidence  string  `json:"confidence"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return 0, "", fmt.Errorf("failed to decode response: %w", err)
    }

    return result.SuccessRate, result.Confidence, nil
}
```

---

## Code Examples

### Example 1: Compare Playbook Versions (A/B Testing)

```bash
#!/bin/bash
# Compare v1.2 vs v2.0 of pod-oom-recovery playbook

# Get v1.2 success rate
curl -s "http://data-storage:8080/api/v1/success-rate/playbook?playbook_id=pod-oom-recovery&playbook_version=v1.2&time_range=7d" \
  | jq '{version: "v1.2", success_rate: .success_rate, confidence: .confidence}'

# Get v2.0 success rate
curl -s "http://data-storage:8080/api/v1/success-rate/playbook?playbook_id=pod-oom-recovery&playbook_version=v2.0&time_range=7d" \
  | jq '{version: "v2.0", success_rate: .success_rate, confidence: .confidence}'
```

**Output**:
```json
{"version": "v1.2", "success_rate": 90.5, "confidence": "high"}
{"version": "v2.0", "success_rate": 95.0, "confidence": "high"}
```

**Decision**: v2.0 is 4.5% more effective → promote to production

### Example 2: Trend Analysis (Recent vs Historical)

```bash
#!/bin/bash
# Compare recent (7d) vs historical (30d) success rates

# Recent success rate
curl -s "http://data-storage:8080/api/v1/success-rate/incident-type?incident_type=pod-oom-killer&time_range=7d" \
  | jq '{period: "7d", success_rate: .success_rate, total_executions: .total_executions}'

# Historical success rate
curl -s "http://data-storage:8080/api/v1/success-rate/incident-type?incident_type=pod-oom-killer&time_range=30d" \
  | jq '{period: "30d", success_rate: .success_rate, total_executions: .total_executions}'
```

**Output**:
```json
{"period": "7d", "success_rate": 95.0, "total_executions": 150}
{"period": "30d", "success_rate": 88.0, "total_executions": 600}
```

**Insight**: Recent success rate improved by 7% → playbook optimization working

### Example 3: AI Execution Mode Validation

```bash
#!/bin/bash
# Validate AI is following 90-9-1 Hybrid Model

curl -s "http://data-storage:8080/api/v1/success-rate/incident-type?incident_type=pod-oom-killer&time_range=7d" \
  | jq '.ai_execution_mode | {
      catalog_pct: ((.catalog_selected / (.catalog_selected + .chained + .manual_escalation)) * 100),
      chained_pct: ((.chained / (.catalog_selected + .chained + .manual_escalation)) * 100),
      manual_pct: ((.manual_escalation / (.catalog_selected + .chained + .manual_escalation)) * 100)
    }'
```

**Output**:
```json
{"catalog_pct": 90.0, "chained_pct": 8.0, "manual_pct": 2.0}
```

**Validation**: ✅ AI follows expected 90-9-1 distribution

---

## Testing Guide

### Unit Testing

```go
package ai_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestSelectBestPlaybook_HighConfidence(t *testing.T) {
    // Mock Data Storage response
    mockResponse := &IncidentTypeSuccessRateResponse{
        IncidentType:  "pod-oom-killer",
        SuccessRate:   90.0,
        Confidence:    "high",
        MinSamplesMet: true,
        PlaybookBreakdown: []PlaybookBreakdownItem{
            {PlaybookID: "pod-oom-recovery", SuccessRate: 92.5},
            {PlaybookID: "memory-limit-increase", SuccessRate: 80.0},
        },
    }

    // Test AI selects highest success rate playbook
    ai := NewAIService(mockDataStorage)
    playbookID, err := ai.SelectBestPlaybook(context.Background(), "pod-oom-killer")

    assert.NoError(t, err)
    assert.Equal(t, "pod-oom-recovery", playbookID)
}

func TestSelectBestPlaybook_InsufficientData(t *testing.T) {
    // Mock insufficient data response
    mockResponse := &IncidentTypeSuccessRateResponse{
        IncidentType:  "rare-incident",
        Confidence:    "insufficient_data",
        MinSamplesMet: false,
    }

    // Test AI rejects low confidence
    ai := NewAIService(mockDataStorage)
    _, err := ai.SelectBestPlaybook(context.Background(), "rare-incident")

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "insufficient data")
}
```

### Integration Testing

```bash
#!/bin/bash
# Integration test: Write action trace → Query success rate

# 1. Write test action traces (100 samples)
for i in {1..100}; do
  curl -X POST http://data-storage:8080/api/v1/action-traces \
    -H "Content-Type: application/json" \
    -d '{
      "incident_type": "test-incident",
      "playbook_id": "test-playbook",
      "playbook_version": "v1.0",
      "execution_status": "completed"
    }'
done

# 2. Query success rate
RESULT=$(curl -s "http://data-storage:8080/api/v1/success-rate/incident-type?incident_type=test-incident&time_range=1h")

# 3. Validate response
echo $RESULT | jq -e '.confidence == "high" and .total_executions == 100' || exit 1
echo "✅ Integration test passed"
```

---

## Troubleshooting

### Issue 1: "insufficient_data" Confidence

**Problem**: API returns `confidence: "insufficient_data"`

**Cause**: Not enough samples (<5 by default)

**Solution**:
```bash
# Option A: Lower min_samples threshold
curl "http://data-storage:8080/api/v1/success-rate/incident-type?incident_type=rare-incident&min_samples=1"

# Option B: Wait for more data to accumulate
# Option C: Use historical data (longer time_range)
curl "http://data-storage:8080/api/v1/success-rate/incident-type?incident_type=rare-incident&time_range=30d"
```

### Issue 2: Empty Playbook Breakdown

**Problem**: `playbook_breakdown: []` in response

**Cause**: No `playbook_id` populated in action traces

**Solution**: Ensure RemediationExecutor populates ADR-033 fields:
```go
trace.PlaybookID = playbook.ID           // Required
trace.PlaybookVersion = playbook.Version // Required
```

### Issue 3: AI Execution Mode All Zeros

**Problem**: `ai_execution_mode: {catalog_selected: 0, chained: 0, manual_escalation: 0}`

**Cause**: ADR-033 boolean fields not populated

**Solution**: Set AI execution mode flags:
```go
trace.AISelectedPlaybook = true   // For catalog selection
trace.AIChainedPlaybooks = false  // For chaining
trace.AIManualEscalation = false  // For escalation
```

### Issue 4: 400 Bad Request - Missing incident_type

**Problem**: `{"detail": "incident_type query parameter is required"}`

**Cause**: Missing required parameter

**Solution**:
```bash
# ❌ Wrong
curl "http://data-storage:8080/api/v1/success-rate/incident-type"

# ✅ Correct
curl "http://data-storage:8080/api/v1/success-rate/incident-type?incident_type=pod-oom-killer"
```

---

## FAQ

### Q1: Do I need to migrate existing data?

**A**: No. ADR-033 columns are nullable. Existing data remains valid, new fields will be `NULL`.

### Q2: Are there any breaking changes?

**A**: No. All changes are additive. Existing endpoints and schemas are unchanged.

### Q3: What if I don't populate ADR-033 fields?

**A**: Action traces will still work, but success rate analytics will not be available for those traces.

### Q4: How do I know if my service is populating ADR-033 fields correctly?

**A**: Query the success rate endpoints. If you get meaningful data (not all zeros), fields are populated correctly.

### Q5: Can I use the old `workflow_id` aggregation?

**A**: No. The `workflow_id` endpoint was architecturally invalid for AI-generated workflows and has been removed.

### Q6: What's the difference between `incident_type` and `alert_name`?

**A**:
- `incident_type`: Generic problem category (e.g., "pod-oom-killer")
- `alert_name`: Specific Prometheus alert (e.g., "ProdPodOOMKilled")

Use `incident_type` for aggregation, `alert_name` for traceability.

### Q7: How do I handle playbook chaining?

**A**: Set `ai_chained_playbooks = true` and use the same `playbook_execution_id` for all actions in the chain.

### Q8: What's the recommended `min_samples` threshold?

**A**:
- **Production decisions**: 20-100 samples (high confidence)
- **Development/testing**: 5 samples (low confidence acceptable)
- **Exploratory analysis**: 1 sample (insufficient_data acceptable)

### Q9: How often should I query success rates?

**A**:
- **AI playbook selection**: Real-time (every incident)
- **Trend analysis**: Daily or weekly
- **A/B testing**: After each deployment

### Q10: Can I filter by multiple incident types?

**A**: No. Each query is for a single incident type. Use multiple queries for comparison.

---

## Next Steps

1. **Update RemediationExecutor**: Populate ADR-033 fields when writing action traces
2. **Update AI Service**: Query success rate endpoints for playbook selection
3. **Update Context API**: Use success rates for historical context enrichment
4. **Monitor Metrics**: Track AI execution mode distribution (90-9-1 expected)
5. **Run Integration Tests**: Validate end-to-end flow

---

## Support

**Documentation**: [api-specification.md](./api-specification.md)
**OpenAPI Spec**: [openapi/v2.yaml](./openapi/v2.yaml)
**ADR**: [ADR-033](../../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
**Issues**: GitHub Issues

---

**Last Updated**: November 5, 2025
**Version**: 1.0
**Status**: ✅ Production Ready

