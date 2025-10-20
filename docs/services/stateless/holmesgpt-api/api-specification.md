# HolmesGPT API - REST API Specification

**Version**: v1.1
**Last Updated**: October 16, 2025
**Base URL**: `http://holmesgpt-api.kubernaut-system:8080`
**Authentication**: Bearer Token (Kubernetes ServiceAccount)
**Prompt Format**: Self-Documenting JSON (DD-HOLMESGPT-009)

**IMPORTANT UPDATE (October 16, 2025)**: All investigation requests now use **Self-Documenting JSON format** for LLM prompts. This achieves:
- ✅ **60% token reduction** (~730 → ~180 tokens)
- ✅ **$1,980/year cost savings** ($165/month)
- ✅ **150ms latency improvement** per investigation
- ✅ **98% parsing accuracy maintained**
- ✅ **Single format only** (pre-production system, no backward compatibility needed)

**Decision Document**: `docs/architecture/decisions/DD-HOLMESGPT-009-Ultra-Compact-JSON-Format.md`

**Note**: This is a streamlined version. See [ORIGINAL_MONOLITHIC.md](./ORIGINAL_MONOLITHIC.md) for complete 2,100+ line specification.

---

## Table of Contents

1. [API Overview](#api-overview)
2. [Investigation API](#investigation-api)
3. [Health & Metrics](#health--metrics)
4. [Error Responses](#error-responses)

---

## API Overview

### Base URL
```
http://holmesgpt-api.kubernaut-system:8080
```

### API Version
All endpoints are prefixed with `/api/v1/`

### Content Type
```
Content-Type: application/json
```

---

## Investigation API

### Create Investigation

**Purpose**: Perform AI-powered investigation using HolmesGPT SDK with support for structured action format

**Business Requirements**: BR-HAPI-001 to BR-HAPI-005, BR-LLM-021, BR-LLM-026

#### Request

```
POST /api/v1/investigate
```

#### Request Body (Self-Documenting JSON - Current)

**Format**: DD-HOLMESGPT-009
**Token Count**: ~180 tokens (vs ~730 for verbose)

The `context` field now accepts ultra-compact JSON for maximum token efficiency:

```json
{
  "context": {
    "i":"mem-api-srv-abc123","p":"P0","e":"prod","s":"api-server",
    "sf":{"dt":60,"a":0,"ok":["scale","restart","rollback","mem_inc"],"no":["del_*"]},
    "dp":[{"s":"api-gw","i":"c"},{"s":"db-proxy","i":"h"}],"dc":"h","ui":"c",
    "al":{"n":"HighMemoryUsage","ns":"prod","pod":"api-server-abc123","mem":"3.8/4.0"},
    "k8":{"d":"api-server","r":3,"node":"node-5","mem_lim":"4Gi","mem_req":"2Gi"},
    "mn":{"ra":2,"cpu":"s","mem":"u","lat":"s","err":"s"},
    "sc":{"w":"15m","d":"dtl","h":1},
    "t":"Analyze HighMemoryUsage. Generate 2-3 recs with deps for parallel exec. Respect 60s downtime."
  },
  "llmProvider": "openai",
  "llmModel": "gpt-4",
  "toolsets": ["kubernetes", "prometheus"],
  "maxTokens": 2000,
  "temperature": 0.7,
  "responseFormat": "v2-structured",
  "enableValidation": true
}
```

**Legend** (one-time overhead):
```
i=inv_id, p=priority, e=env, s=service, sf=safety, dt=downtime, a=approval(0/1),
ok=allowed, no=blocked, dp=dependencies, dc=data_crit, ui=usr_impact,
al=alert, k8=kubernetes, mn=monitoring, sc=scope, t=task,
c=critical, h=high, m=medium, l=low, s=stable, u=up, d=down, dtl=detailed
```

**Fields**:
- `context` (object, required): Investigation context in ultra-compact JSON format
- `llmProvider` (string, required): LLM provider (`"openai"`, `"anthropic"`, etc.)
- `llmModel` (string, required): Model name (e.g., `"gpt-4"`, `"claude-3-opus"`)
- `toolsets` (array, optional): Available toolsets for investigation
- `maxTokens` (integer, optional): Maximum tokens for response (default: 2000)
- `temperature` (float, optional): LLM temperature 0.0-1.0 (default: 0.7)
- `responseFormat` (string, optional): Response format version (`"v2-structured"` default)
- `enableValidation` (boolean, optional): Enable schema validation (default: `true`)

**Legacy Verbose Format** (Deprecated):
```json
{
  "context": {
    "namespace": "production",
    "podName": "api-server-abc123",
    "alertName": "HighMemoryUsage",
    "timeRange": "15m"
  },
  "llmProvider": "openai",
  "llmModel": "gpt-4",
  "toolsets": ["kubernetes", "prometheus"],
  "maxTokens": 2000,
  "temperature": 0.7,
  "responseFormat": "v2-structured",
  "enableValidation": true
}
```

#### Response (200 OK) - Legacy Format

```json
{
  "investigationId": "inv-xyz789",
  "rootCause": "Memory leak in cache layer causing unbounded growth",
  "confidence": 0.85,
  "recommendation": "Increase memory limits to 4Gi and implement cache eviction policy with 1000-item limit",
  "analysis": "Detailed analysis text...",
  "toolsUsed": ["kubernetes", "prometheus"],
  "tokensUsed": 1250,
  "durationSeconds": 5.2,
  "timestamp": "2025-10-06T10:00:17Z"
}
```

#### Response (200 OK) - Structured Format (v2-structured)

```json
{
  "investigationId": "inv-xyz789",
  "status": "completed",
  "structuredActions": [
    {
      "actionType": "restart_pod",
      "parameters": {
        "namespace": "production",
        "resourceType": "pod",
        "resourceName": "api-server-abc123",
        "reason": "high_memory_usage"
      },
      "priority": "high",
      "confidence": 0.9,
      "reasoning": {
        "primaryReason": "Memory leak detected in cache layer causing unbounded growth",
        "riskAssessment": "low",
        "businessImpact": "Brief service interruption (10-15 seconds) with automatic recovery"
      },
      "monitoring": {
        "successCriteria": [
          "memory_below_80_percent",
          "pod_running",
          "no_crash_loops"
        ],
        "validationInterval": "30s"
      }
    },
    {
      "actionType": "increase_resources",
      "parameters": {
        "namespace": "production",
        "resourceType": "deployment",
        "resourceName": "api-server",
        "memory": "4Gi",
        "reason": "prevent_future_oom"
      },
      "priority": "medium",
      "confidence": 0.85,
      "reasoning": {
        "primaryReason": "Current memory limits insufficient for workload with cache",
        "riskAssessment": "low",
        "businessImpact": "Rolling update with zero downtime"
      },
      "monitoring": {
        "successCriteria": [
          "deployment_updated",
          "no_pod_restarts"
        ],
        "validationInterval": "1m"
      }
    }
  ],
  "metadata": {
    "generatedAt": "2025-10-06T10:00:17Z",
    "modelVersion": "holmesgpt-v1.0",
    "formatVersion": "v2-structured",
    "tokensUsed": 1250,
    "durationSeconds": 5.2
  }
}
```

---

### Structured Action Format Specification

**Business Requirements**: BR-LLM-021 to BR-LLM-025 (Structured Response Generation)

#### Valid Action Types

**Source of Truth**: `docs/design/CANONICAL_ACTION_TYPES.md`

The structured response MUST use one of the following **27 canonical predefined action types**:

**Core Actions** (P0 - High Frequency) - **5 actions**:
- `scale_deployment` - Scale deployment replicas
- `restart_pod` - Restart specific pod
- `increase_resources` - Increase CPU/memory limits
- `rollback_deployment` - Rollback to previous version
- `expand_pvc` - Expand PersistentVolumeClaim

**Infrastructure Actions** (P1 - Medium Frequency) - **6 actions**:
- `drain_node` - Drain node for maintenance
- `cordon_node` - Mark node unschedulable
- `uncordon_node` - Mark node schedulable
- `taint_node` - Apply node taints to control pod scheduling
- `untaint_node` - Remove node taints to allow pod scheduling
- `quarantine_pod` - Isolate problematic pod

**Storage & Persistence** (P2) - **3 actions**:
- `cleanup_storage` - Clean up old data
- `backup_data` - Create data backup
- `compact_storage` - Compact storage

**Application Lifecycle** (P1) - **3 actions**:
- `update_hpa` - Update HorizontalPodAutoscaler
- `restart_daemonset` - Restart DaemonSet pods
- `scale_statefulset` - Scale StatefulSet replicas

**Security & Compliance** (P2) - **3 actions**:
- `rotate_secrets` - Rotate Kubernetes secrets
- `audit_logs` - Collect audit logs
- `update_network_policy` - Update NetworkPolicy

**Network & Connectivity** (P2) - **2 actions**:
- `restart_network` - Restart network components
- `reset_service_mesh` - Reset service mesh

**Database & Stateful** (P2) - **2 actions**:
- `failover_database` - Trigger database failover
- `repair_database` - Repair database corruption

**Monitoring & Observability** (P2) - **3 actions**:
- `enable_debug_mode` - Enable debug logging
- `create_heap_dump` - Create JVM heap dump
- `collect_diagnostics` - Collect diagnostic data

**Resource Management** (P1) - **2 actions**:
- `optimize_resources` - Optimize resource allocation
- `migrate_workload` - Migrate workload to another node/cluster

**Fallback** (P3) - **1 action**:
- `notify_only` - No automated action, notify only (used when no automated action is appropriate)

#### Structured Action Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["investigationId", "status", "structuredActions"],
  "properties": {
    "investigationId": {
      "type": "string",
      "description": "Unique investigation identifier",
      "pattern": "^inv-[a-zA-Z0-9]+$"
    },
    "status": {
      "type": "string",
      "enum": ["completed", "partial", "failed"],
      "description": "Investigation completion status"
    },
    "structuredActions": {
      "type": "array",
      "description": "List of structured remediation actions",
      "minItems": 1,
      "items": {
        "type": "object",
        "required": ["actionType", "parameters", "priority", "confidence", "reasoning"],
        "properties": {
          "actionType": {
            "type": "string",
            "enum": [
              "scale_deployment", "restart_pod", "increase_resources",
              "rollback_deployment", "expand_pvc", "drain_node",
              "cordon_node", "uncordon_node", "taint_node", "untaint_node",
              "quarantine_pod", "cleanup_storage", "backup_data", "compact_storage",
              "update_hpa", "restart_daemonset", "scale_statefulset",
              "rotate_secrets", "audit_logs", "update_network_policy",
              "restart_network", "reset_service_mesh", "failover_database",
              "repair_database", "enable_debug_mode", "create_heap_dump",
              "collect_diagnostics", "optimize_resources", "migrate_workload",
              "notify_only"
            ],
            "description": "Predefined action type from Kubernaut action registry (29 canonical actions)"
          },
          "parameters": {
            "type": "object",
            "required": ["namespace"],
            "properties": {
              "namespace": {
                "type": "string",
                "description": "Kubernetes namespace"
              },
              "resourceType": {
                "type": "string",
                "enum": ["pod", "deployment", "statefulset", "daemonset", "node", "pvc", "service", "hpa"],
                "description": "Type of Kubernetes resource"
              },
              "resourceName": {
                "type": "string",
                "description": "Name of the resource to act upon"
              }
            },
            "additionalProperties": true
          },
          "priority": {
            "type": "string",
            "enum": ["critical", "high", "medium", "low"],
            "description": "Action execution priority"
          },
          "confidence": {
            "type": "number",
            "minimum": 0.0,
            "maximum": 1.0,
            "description": "Confidence score for this recommendation"
          },
          "reasoning": {
            "type": "object",
            "required": ["primaryReason", "riskAssessment"],
            "properties": {
              "primaryReason": {
                "type": "string",
                "description": "Root cause analysis and justification"
              },
              "riskAssessment": {
                "type": "string",
                "enum": ["low", "medium", "high"],
                "description": "Risk level of executing this action"
              },
              "businessImpact": {
                "type": "string",
                "description": "Expected impact on business operations"
              }
            }
          },
          "monitoring": {
            "type": "object",
            "properties": {
              "successCriteria": {
                "type": "array",
                "items": {
                  "type": "string"
                },
                "description": "Conditions that indicate successful action completion"
              },
              "validationInterval": {
                "type": "string",
                "pattern": "^[0-9]+(s|m|h)$",
                "description": "How often to validate success (e.g., '30s', '1m')"
              }
            }
          }
        }
      }
    },
    "metadata": {
      "type": "object",
      "required": ["generatedAt", "formatVersion"],
      "properties": {
        "generatedAt": {
          "type": "string",
          "format": "date-time",
          "description": "ISO 8601 timestamp"
        },
        "modelVersion": {
          "type": "string",
          "description": "HolmesGPT model version used"
        },
        "formatVersion": {
          "type": "string",
          "enum": ["v2-structured"],
          "description": "Response format version"
        },
        "tokensUsed": {
          "type": "integer",
          "minimum": 0,
          "description": "LLM tokens consumed"
        },
        "durationSeconds": {
          "type": "number",
          "minimum": 0,
          "description": "Investigation duration in seconds"
        }
      }
    }
  }
}
```

#### Configuration Requirements

**Environment Variables**:

```bash
# Structured response configuration
HOLMESGPT_STRUCTURED_FORMAT_ENABLED=true
HOLMESGPT_DEFAULT_RESPONSE_FORMAT=v2-structured
HOLMESGPT_ENABLE_SCHEMA_VALIDATION=true
HOLMESGPT_ENABLE_FUZZY_MATCHING=true
HOLMESGPT_FALLBACK_TO_LEGACY=true

# Structured action toolset
HOLMESGPT_STRUCTURED_TOOLSET_PATH=/config/structured-actions-toolset.yaml
HOLMESGPT_VALID_ACTIONS_PATH=/config/valid-actions.json
```

**Configuration File** (`config/structured-actions.yaml`):

```yaml
structuredActions:
  enabled: true
  defaultFormat: v2-structured

  validation:
    enabled: true
    strictMode: false  # Allow fuzzy matching when true=strict
    fuzzyMatchingThreshold: 0.8

  fallback:
    enableLegacyFallback: true
    legacyFallbackTimeout: 5s

  actionRegistry:
    source: /config/valid-actions.json
    refreshInterval: 5m
```

#### Backward Compatibility Requirements

**BR-LLM-023**: MUST handle both structured and legacy formats

**Implementation Requirements**:

1. **Request Compatibility**:
   - If `responseFormat` is not specified, default to `v2-structured`
   - Support `legacy` format for backward compatibility during transition
   - Validate `responseFormat` value against supported formats

2. **Response Parsing**:
   - Implement parser that handles both formats
   - Detect format automatically if response format is ambiguous
   - Convert legacy format to structured format internally for unified processing

3. **Error Handling**:
   - If structured parsing fails, attempt legacy format parsing
   - If both fail, return structured error response with `notify_only` action
   - Log format detection and parsing attempts for debugging

4. **Validation Modes**:
   - **Strict Mode** (`strictMode: true`): Reject invalid action types
   - **Fuzzy Mode** (`strictMode: false`): Attempt fuzzy matching to closest valid action
   - **Fallback Mode**: Use `notify_only` if no match found

#### Fuzzy Matching Algorithm

**BR-LLM-024**: MUST extract structured data with intelligent fallback

**Configuration Parameters**:

```yaml
fuzzy_matching:
  enabled: true                    # Enable/disable fuzzy matching
  threshold: 0.8                   # Similarity threshold (0.0-1.0)
  fallback_action: "notify_only"  # Action to use when no match found
  log_matches: true                # Log all fuzzy matches for monitoring
  max_suggestions: 3               # Maximum number of suggestions to return
```

**Environment Variables**:
```bash
FUZZY_MATCHING_ENABLED=true
FUZZY_MATCHING_THRESHOLD=0.8
FUZZY_MATCHING_FALLBACK=notify_only
FUZZY_MATCHING_LOG_MATCHES=true
```

**Implementation**:

```python
def fuzzy_match_action(
    action_type: str,
    valid_actions: List[str],
    threshold: float = None,  # Now configurable, not hardcoded
    fallback_action: str = None
) -> Optional[str]:
    """
    Fuzzy match unknown action to predefined action.

    Args:
        action_type: Action type from LLM response
        valid_actions: List of valid action types
        threshold: Minimum similarity threshold (from config)
        fallback_action: Action to use when no match found (from config)

    Returns:
        Matched action type or fallback action
    """
    from difflib import get_close_matches

    # Use configuration values or defaults
    threshold = threshold or app.config.get('FUZZY_MATCHING_THRESHOLD', 0.8)
    fallback_action = fallback_action or app.config.get('FUZZY_MATCHING_FALLBACK', 'notify_only')

    matches = get_close_matches(action_type, valid_actions, n=1, cutoff=threshold)
    if matches:
        matched = matches[0]
        logger.info(
            f"Fuzzy matched '{action_type}' -> '{matched}'",
            extra={
                "original_action": action_type,
                "matched_action": matched,
                "threshold": threshold,
                "match_score": calculate_similarity(action_type, matched)
            }
        )

        # Emit metric for monitoring
        fuzzy_match_success_counter.labels(
            original=action_type,
            matched=matched
        ).inc()

        return matched

    logger.warning(
        f"No fuzzy match for '{action_type}', using fallback '{fallback_action}'",
        extra={
            "original_action": action_type,
            "fallback_action": fallback_action,
            "threshold": threshold
        }
    )

    # Emit metric for monitoring
    fuzzy_match_fallback_counter.labels(
        original=action_type,
        fallback=fallback_action
    ).inc()

    return fallback_action

def calculate_similarity(a: str, b: str) -> float:
    """Calculate similarity score between two strings."""
    from difflib import SequenceMatcher
    return SequenceMatcher(None, a, b).ratio()
```

**Monitoring Metrics**:

```python
from prometheus_client import Counter

fuzzy_match_success_counter = Counter(
    'holmesgpt_fuzzy_match_success_total',
    'Total successful fuzzy matches',
    ['original', 'matched']
)

fuzzy_match_fallback_counter = Counter(
    'holmesgpt_fuzzy_match_fallback_total',
    'Total fuzzy match fallbacks',
    ['original', 'fallback']
)
```

---

### Failure Handling & Rollback Strategy

**Business Requirements**: BR-LLM-023 (Handle both structured and legacy formats)

**Purpose**: Define resilient failure handling and automatic rollback when structured format encounters repeated failures.

#### Circuit Breaker Pattern

**Configuration**:

```yaml
circuit_breaker:
  enabled: true
  failure_threshold: 5              # Number of consecutive failures before opening circuit
  success_threshold: 3              # Number of consecutive successes before closing circuit
  timeout: 300                      # Timeout in seconds before attempting half-open
  half_open_max_requests: 3         # Max requests allowed in half-open state

structured_format:
  failure_tracking_window: "5m"     # Time window for tracking failures
  failure_rate_threshold: 0.20      # 20% failure rate triggers circuit breaker
  auto_disable_threshold: 10        # Auto-disable after N consecutive failures
  cooldown_period: "15m"            # Cooldown before re-enabling
```

**Environment Variables**:
```bash
CIRCUIT_BREAKER_ENABLED=true
STRUCTURED_FORMAT_FAILURE_THRESHOLD=5
STRUCTURED_FORMAT_FAILURE_RATE_THRESHOLD=0.20
STRUCTURED_FORMAT_AUTO_DISABLE_THRESHOLD=10
STRUCTURED_FORMAT_COOLDOWN_PERIOD=15m
```

#### Failure Scenarios & Responses

| Scenario | Condition | Response | Fallback | Alert |
|----------|-----------|----------|----------|-------|
| **Parsing Failure** | Cannot parse JSON response | Attempt legacy format | Return notify_only | Warning |
| **Schema Validation Failure** | Response doesn't match schema | Attempt to fix/coerce types | Return notify_only | Warning |
| **Invalid Action Types** | Unknown action types in response | Apply fuzzy matching | Use fallback action | Info |
| **Repeated Failures (5+)** | 5 consecutive failures | Open circuit breaker | Force legacy format | Critical |
| **High Failure Rate (>20%)** | >20% failures in 5m window | Open circuit breaker | Force legacy format | Critical |
| **Total Failure (10+)** | 10 consecutive failures | Auto-disable structured format | Legacy only | Critical |

#### Implementation

**File**: `app/services/circuit_breaker.py`

```python
from enum import Enum
from datetime import datetime, timedelta
from typing import Optional
import logging

logger = logging.getLogger(__name__)

class CircuitState(Enum):
    CLOSED = "closed"        # Normal operation
    OPEN = "open"            # Failures detected, using fallback
    HALF_OPEN = "half_open"  # Testing if service recovered

class StructuredFormatCircuitBreaker:
    """Circuit breaker for structured format failures."""

    def __init__(self, config):
        self.failure_threshold = config.get('STRUCTURED_FORMAT_FAILURE_THRESHOLD', 5)
        self.success_threshold = config.get('STRUCTURED_FORMAT_SUCCESS_THRESHOLD', 3)
        self.timeout = config.get('STRUCTURED_FORMAT_TIMEOUT', 300)
        self.auto_disable_threshold = config.get('STRUCTURED_FORMAT_AUTO_DISABLE_THRESHOLD', 10)
        self.cooldown_period = config.get('STRUCTURED_FORMAT_COOLDOWN_PERIOD', 900)  # 15m

        self.state = CircuitState.CLOSED
        self.consecutive_failures = 0
        self.consecutive_successes = 0
        self.total_consecutive_failures = 0
        self.last_failure_time = None
        self.opened_at = None
        self.auto_disabled = False
        self.disabled_at = None

    def call(self, func, *args, **kwargs):
        """Execute function with circuit breaker protection."""

        # Check if auto-disabled
        if self.auto_disabled:
            if self._should_retry_after_disable():
                logger.info("Cooldown period elapsed, re-enabling structured format")
                self._reset()
            else:
                logger.warning("Structured format auto-disabled, using legacy format")
                raise CircuitBreakerOpen("Structured format auto-disabled")

        # Check circuit state
        if self.state == CircuitState.OPEN:
            if self._should_attempt_reset():
                logger.info("Circuit breaker attempting reset (half-open)")
                self.state = CircuitState.HALF_OPEN
            else:
                logger.warning(f"Circuit breaker OPEN, using fallback (failures: {self.consecutive_failures})")
                raise CircuitBreakerOpen("Too many structured format failures")

        try:
            result = func(*args, **kwargs)
            self._on_success()
            return result
        except Exception as e:
            self._on_failure()
            raise

    def _on_success(self):
        """Record successful call."""
        self.consecutive_failures = 0
        self.total_consecutive_failures = 0
        self.consecutive_successes += 1
        self.last_failure_time = None

        # Close circuit after enough successes
        if self.state == CircuitState.HALF_OPEN:
            if self.consecutive_successes >= self.success_threshold:
                logger.info(f"Circuit breaker CLOSED after {self.consecutive_successes} successes")
                self.state = CircuitState.CLOSED
                self.consecutive_successes = 0
                self.opened_at = None

    def _on_failure(self):
        """Record failed call."""
        self.consecutive_failures += 1
        self.total_consecutive_failures += 1
        self.consecutive_successes = 0
        self.last_failure_time = datetime.now()

        # Open circuit after threshold
        if self.consecutive_failures >= self.failure_threshold:
            if self.state != CircuitState.OPEN:
                logger.error(f"Circuit breaker OPEN after {self.consecutive_failures} consecutive failures")
                self.state = CircuitState.OPEN
                self.opened_at = datetime.now()

                # Send critical alert
                send_alert(
                    severity="critical",
                    message=f"Structured format circuit breaker OPEN: {self.consecutive_failures} consecutive failures",
                    labels={"component": "holmesgpt-api", "circuit": "structured_format"}
                )

        # Auto-disable after too many failures
        if self.total_consecutive_failures >= self.auto_disable_threshold:
            logger.critical(f"Auto-disabling structured format after {self.total_consecutive_failures} total failures")
            self.auto_disabled = True
            self.disabled_at = datetime.now()

            # Send critical alert
            send_alert(
                severity="critical",
                message=f"Structured format AUTO-DISABLED after {self.total_consecutive_failures} failures",
                labels={"component": "holmesgpt-api", "action": "auto_disabled"}
            )

    def _should_attempt_reset(self) -> bool:
        """Check if enough time has passed to attempt reset."""
        if not self.opened_at:
            return False
        return (datetime.now() - self.opened_at).total_seconds() >= self.timeout

    def _should_retry_after_disable(self) -> bool:
        """Check if cooldown period has elapsed."""
        if not self.disabled_at:
            return True
        return (datetime.now() - self.disabled_at).total_seconds() >= self.cooldown_period

    def _reset(self):
        """Reset circuit breaker state."""
        self.state = CircuitState.CLOSED
        self.consecutive_failures = 0
        self.total_consecutive_failures = 0
        self.consecutive_successes = 0
        self.opened_at = None
        self.auto_disabled = False
        self.disabled_at = None
        logger.info("Circuit breaker RESET")

class CircuitBreakerOpen(Exception):
    """Exception raised when circuit breaker is open."""
    pass
```

#### Failure Handling Flow

```python
@investigation_bp.route('/api/v1/investigate', methods=['POST'])
@validate_token
def investigate_alert(request: InvestigateRequest):
    """Investigation with automatic failure handling and rollback."""

    # Determine which format to use
    use_structured = (
        request.response_format == "v2-structured" and
        app.config.get('STRUCTURED_FORMAT_ENABLED', True) and
        not circuit_breaker.auto_disabled
    )

    if use_structured:
        try:
            # Attempt structured format with circuit breaker protection
            result = circuit_breaker.call(
                investigate_with_structured_format,
                request
            )
            return result

        except CircuitBreakerOpen as e:
            logger.warning(f"Circuit breaker open, falling back to legacy: {e}")

            # Fallback to legacy format
            if app.config.get('FALLBACK_TO_LEGACY', True):
                logger.info("Using legacy format as fallback")
                return investigate_with_legacy_format(request)
            else:
                # Return safe fallback response
                return create_fallback_response(request, str(e))

        except Exception as e:
            logger.error(f"Structured format failed: {e}", exc_info=True)
            circuit_breaker._on_failure()

            # Fallback to legacy format
            if app.config.get('FALLBACK_TO_LEGACY', True):
                logger.info("Falling back to legacy format after error")
                return investigate_with_legacy_format(request)
            else:
                return create_fallback_response(request, str(e))

    else:
        # Use legacy format
        logger.info("Using legacy format (structured disabled or not requested)")
        return investigate_with_legacy_format(request)

def create_fallback_response(request, error_message):
    """Create safe fallback response when all formats fail."""
    return {
        "investigation_id": f"fallback-{int(time.time())}",
        "status": "partial",
        "structured_actions": [
            {
                "action_type": "notify_only",
                "parameters": {
                    "namespace": request.namespace,
                    "message": f"Investigation failed: {error_message}. Manual review required.",
                },
                "priority": "high",
                "confidence": 0.5,
                "reasoning": {
                    "primary_reason": "Automated investigation unavailable",
                    "risk_assessment": "low",
                    "business_impact": "Manual intervention needed"
                }
            }
        ],
        "metadata": {
            "generated_at": datetime.utcnow().isoformat(),
            "format_version": "fallback",
            "error": error_message
        }
    }
```

#### Monitoring & Alerts

**Prometheus Metrics**:

```python
from prometheus_client import Counter, Gauge, Histogram

# Circuit breaker state
circuit_breaker_state_gauge = Gauge(
    'holmesgpt_circuit_breaker_state',
    'Circuit breaker state (0=closed, 1=open, 2=half-open)',
    ['format']
)

# Failure tracking
structured_format_failures_counter = Counter(
    'holmesgpt_structured_format_failures_total',
    'Total structured format failures',
    ['error_type']
)

structured_format_fallback_counter = Counter(
    'holmesgpt_structured_format_fallback_total',
    'Total fallbacks to legacy format'
)

# Auto-disable tracking
structured_format_auto_disabled_gauge = Gauge(
    'holmesgpt_structured_format_auto_disabled',
    'Whether structured format is auto-disabled (0=enabled, 1=disabled)'
)
```

**AlertManager Rules**:

```yaml
groups:
- name: holmesgpt-structured-format
  rules:
  - alert: StructuredFormatCircuitBreakerOpen
    expr: holmesgpt_circuit_breaker_state{format="structured"} == 1
    for: 5m
    labels:
      severity: critical
      component: holmesgpt-api
    annotations:
      summary: "Structured format circuit breaker is OPEN"
      description: "Circuit breaker has been open for 5 minutes due to repeated failures"

  - alert: StructuredFormatAutoDisabled
    expr: holmesgpt_structured_format_auto_disabled == 1
    labels:
      severity: critical
      component: holmesgpt-api
    annotations:
      summary: "Structured format has been AUTO-DISABLED"
      description: "Too many consecutive failures, structured format disabled"

  - alert: StructuredFormatHighFailureRate
    expr: rate(holmesgpt_structured_format_failures_total[5m]) > 0.2
    for: 5m
    labels:
      severity: warning
      component: holmesgpt-api
    annotations:
      summary: "High structured format failure rate"
      description: "Failure rate >20% over 5 minutes"
```

#### Manual Recovery Procedures

**Force Enable**:
```bash
# Via API
curl -X POST http://holmesgpt-api:8090/admin/circuit-breaker/reset \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Via environment variable
kubectl set env deployment/holmesgpt-api \
  STRUCTURED_FORMAT_FORCE_ENABLE=true \
  -n kubernaut-system
```

**Check Status**:
```bash
curl http://holmesgpt-api:8090/admin/circuit-breaker/status
```

---

#### Testing Requirements

**Unit Tests**:
- ✅ Schema validation for all 27 action types
- ✅ Fuzzy matching with various similarity thresholds
- ✅ Legacy format conversion to structured format
- ✅ Error handling for malformed responses
- ✅ Circuit breaker state transitions
- ✅ Auto-disable and recovery logic

**Integration Tests**:
- ✅ End-to-end investigation with structured response
- ✅ Backward compatibility with legacy clients
- ✅ Feature flag controlled rollout
- ✅ Performance comparison: structured vs legacy
- ✅ Circuit breaker failure scenarios
- ✅ Automatic fallback to legacy format

**Test Coverage Target**: >90%

---

#### Python Implementation

```python
# app/routes/investigation.py
from flask import Blueprint, request, jsonify
from holmes import Holmes
from app.models import InvestigationRequest, InvestigationResponse
from app.utils import validate_token, get_correlation_id

investigation_bp = Blueprint('investigation', __name__)

@investigation_bp.route('/api/v1/investigate', methods=['POST'])
@validate_token
def investigate():
    """
    Perform HolmesGPT investigation.
    """
    correlation_id = get_correlation_id(request)

    # Parse request
    req_data = request.get_json()
    req = InvestigationRequest(**req_data)

    # Validate toolsets
    available_toolsets = app.config.get('AVAILABLE_TOOLSETS', [])
    for toolset in req.toolsets:
        if toolset not in available_toolsets:
            return jsonify({"error": f"Toolset '{toolset}' not available"}), 400

    # Initialize Holmes
    holmes = Holmes(
        llm_provider=req.llmProvider,
        llm_model=req.llmModel,
        toolsets=req.toolsets
    )

    # Execute investigation
    try:
        result = holmes.investigate(
            context=req.context,
            max_tokens=req.maxTokens,
            temperature=req.temperature
        )

        # Format response
        response = InvestigationResponse(
            investigationId=result['id'],
            rootCause=result['root_cause'],
            confidence=result['confidence'],
            recommendation=result['recommendation'],
            analysis=result['analysis'],
            toolsUsed=result['tools_used'],
            tokensUsed=result['tokens_used'],
            durationSeconds=result['duration']
        )

        return jsonify(response.dict()), 200

    except Exception as e:
        app.logger.error(f"Investigation failed: {e}", extra={
            'correlation_id': correlation_id
        })
        return jsonify({"error": "Investigation failed"}), 500
```

---

## Health & Metrics

### Health Check

```
GET /health
```

**Response**: 200 OK if healthy

```json
{
  "status": "healthy",
  "timestamp": "2025-10-06T10:15:30Z",
  "dependencies": {
    "llm": "healthy",
    "kubernetes": "healthy",
    "prometheus": "healthy"
  },
  "toolsets": ["kubernetes", "prometheus"]
}
```

### Readiness Check

```
GET /ready
```

**Response**: 200 OK if ready to serve traffic

### Metrics

```
GET /metrics
```

**Format**: Prometheus text format
**Authentication**: Required (TokenReviewer)

**Key Metrics**:
- `holmesgpt_investigations_total{llm_provider="openai"}` - Total investigations
- `holmesgpt_investigation_duration_seconds` - Investigation latency histogram
- `holmesgpt_llm_tokens_used_total` - Total LLM tokens consumed
- `holmesgpt_toolset_executions_total{toolset="kubernetes"}` - Toolset usage
- `holmesgpt_errors_total{type="llm_error"}` - Error counts

---

## Error Responses

### Standard Error Format

```json
{
  "error": {
    "code": "INVESTIGATION_FAILED",
    "message": "LLM provider returned error",
    "details": {
      "provider": "openai",
      "reason": "Rate limit exceeded"
    }
  },
  "timestamp": "2025-10-06T10:15:30Z",
  "path": "/api/v1/investigate",
  "correlationId": "req-2025-10-06-abc123"
}
```

### HTTP Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| 200 | Success | Investigation completed |
| 400 | Bad Request | Invalid toolset name |
| 401 | Unauthorized | Invalid token |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | LLM provider error |
| 503 | Service Unavailable | Toolset unavailable |

---

### Common Error Codes

| Error Code | Description | Resolution |
|------------|-------------|------------|
| `VALIDATION_ERROR` | Request validation failed | Check required fields |
| `TOOLSET_UNAVAILABLE` | Requested toolset not configured | Verify ConfigMap |
| `LLM_ERROR` | LLM provider error | Check API key, retry |
| `KUBERNETES_ERROR` | Kubernetes API error | Check RBAC permissions |
| `PROMETHEUS_ERROR` | Prometheus query error | Verify Prometheus endpoint |

---

## Complete Specification

For the complete 2,100+ line specification including:
- Detailed toolset configuration
- ConfigMap examples
- RBAC complete setup
- LLM provider details
- Token optimization strategies
- Business requirements (BR-HOLMES-001 to BR-HOLMES-180)
- Deployment configurations
- Error handling strategies

**See**: [ORIGINAL_MONOLITHIC.md](./ORIGINAL_MONOLITHIC.md)

---

**Document Status**: ✅ Complete (Streamlined)
**Original**: See [ORIGINAL_MONOLITHIC.md](./ORIGINAL_MONOLITHIC.md) for full specification
**Last Updated**: October 6, 2025
**Version**: 1.0
