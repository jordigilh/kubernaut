# AI Analysis Service - Event Data Schema

**Version**: 1.0
**Service**: aianalysis
**Purpose**: LLM analysis lifecycle, RCA, workflow selection, token tracking

---

## Schema Structure

```json
{
  "version": "1.0",
  "service": "aianalysis",
  "event_type": "analysis.started|analysis.completed|analysis.failed",
  "timestamp": "2025-11-18T10:00:00Z",
  "data": {
    "ai_analysis": {
      "analysis_id": "analysis-2025-11-18-001",
      "llm_provider": "anthropic",
      "llm_model": "claude-haiku-4-5-20251001",
      "prompt_tokens": 2500,
      "completion_tokens": 750,
      "total_tokens": 3250,
      "duration_ms": 4200,
      "rca_signal_type": "OOMKilled",
      "rca_severity": "critical",
      "confidence": 0.95,
      "workflow_id": "workflow-increase-memory",
      "tools_invoked": [
        "kubernetes/describe_pod",
        "kubernetes/get_logs",
        "workflow/search_catalog"
      ],
      "error_code": "LLM_TIMEOUT"
    }
  }
}
```

---

## Field Definitions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `analysis_id` | string | Yes | Unique analysis identifier |
| `llm_provider` | string | No | LLM provider (`anthropic`, `openai`) |
| `llm_model` | string | No | Model identifier |
| `prompt_tokens` | integer | Yes | Input tokens |
| `completion_tokens` | integer | Yes | Output tokens |
| `total_tokens` | integer | Yes | Total tokens (auto-calculated) |
| `duration_ms` | integer | No | Analysis duration |
| `rca_signal_type` | string | No | Root cause signal type |
| `rca_severity` | string | No | Root cause severity |
| `confidence` | float | No | Confidence score (0.0-1.0) |
| `workflow_id` | string | No | Selected workflow |
| `tools_invoked` | array | No | MCP tools used |
| `error_code` | string | No | Error code if failed |

---

## Go Builder Usage

```go
eventData, err := audit.NewAIAnalysisEvent("analysis.completed").
    WithAnalysisID("analysis-2025-001").
    WithLLM("anthropic", "claude-haiku-4-5").
    WithTokenUsage(2500, 750).
    WithDuration(4200).
    WithRCA("OOMKilled", "critical", 0.95).
    WithWorkflow("workflow-increase-memory").
    WithToolsInvoked([]string{
        "kubernetes/describe_pod",
        "workflow/search_catalog",
    }).
    Build()
```

---

## Query Examples

### Find high-confidence analyses

```sql
SELECT event_id, event_timestamp,
       event_data->'data'->'ai_analysis'->>'rca_signal_type' AS rca,
       (event_data->'data'->'ai_analysis'->>'confidence')::float AS confidence
FROM audit_events
WHERE event_data->>'service' = 'aianalysis'
AND (event_data->'data'->'ai_analysis'->>'confidence')::float > 0.9
ORDER BY event_timestamp DESC;
```

### Track token usage by model

```sql
SELECT
    event_data->'data'->'ai_analysis'->>'llm_model' AS model,
    SUM((event_data->'data'->'ai_analysis'->>'total_tokens')::integer) AS total_tokens,
    AVG((event_data->'data'->'ai_analysis'->>'duration_ms')::integer) AS avg_duration_ms
FROM audit_events
WHERE event_data->>'service' = 'aianalysis'
AND event_type = 'ai.analysis.completed'
GROUP BY model;
```

---

## Business Requirements

| BR ID | Requirement | Fields Used |
|-------|-------------|-------------|
| BR-STORAGE-033-007 | AI event structure | All ai_analysis fields |
| BR-STORAGE-033-008 | LLM metrics tracking | `llm_provider`, `llm_model`, tokens, `duration_ms` |
| BR-STORAGE-033-009 | RCA/workflow metadata | `rca_signal_type`, `confidence`, `workflow_id`, `tools_invoked` |

