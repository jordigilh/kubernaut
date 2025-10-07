# Business Requirements: HolmesGPT-API Validation & Resilience

**Category**: Service Reliability & Validation
**BR Range**: BR-HAPI-186 to BR-HAPI-191
**Version**: 1.0
**Last Updated**: 2025-01-15
**Status**: APPROVED

---

## Overview

This document defines business requirements for HolmesGPT-API service startup validation and runtime resilience. These requirements ensure production reliability through fail-fast validation and automatic recovery mechanisms.

**Key Capabilities**:
- Fail-fast startup validation prevents runtime failures
- Comprehensive error messages reduce troubleshooting time
- Development mode enables local development
- Runtime failure tracking enables self-healing
- Automatic ConfigMap reload handles configuration drift
- Graceful reload preserves active investigations

---

## BR-HAPI-186: Fail-Fast Startup Validation

**Priority**: P0 - CRITICAL
**Category**: Service Reliability
**Related Services**: HolmesGPT-API (port 8090)

### Requirement

The service MUST validate ALL enabled toolsets at startup before accepting requests.

**Validation Scope**:

| Toolset | Validation Checks |
|---------|------------------|
| **kubernetes** | 1. K8s API server connectivity<br>2. RBAC: `list pods` permission<br>3. RBAC: `read pod logs` permission<br>4. RBAC: `list events` permission |
| **prometheus** | 1. HTTP connectivity to endpoint<br>2. Query API validation (`up` query)<br>3. Response format validation |
| **grafana** | 1. HTTP connectivity to endpoint<br>2. `/api/health` endpoint check |

**Exit Behavior**:
- ✅ **All enabled toolsets validate** → Service starts and accepts requests
- ❌ **Any enabled toolset fails validation** → Service exits with detailed error message and non-zero exit code

**Validation Timeout**:
- kubernetes toolset: 10 seconds max (K8s API call timeout)
- prometheus toolset: 5 seconds max (HTTP request timeout)
- grafana toolset: 5 seconds max (HTTP request timeout)
- Total startup validation: 30 seconds max

**Edge Cases**:
- If toolset is NOT enabled in ConfigMap → Service MUST skip validation for that toolset
- If ConfigMap missing → Service uses default toolsets (`kubernetes`, `prometheus`) and validates them
- If grafana endpoint not configured → Service MUST skip grafana validation (optional toolset)

### Acceptance Criteria

**AC1**: Service validates kubernetes toolset RBAC permissions at startup
- Checks: `list pods`, `read pod logs`, `list events`
- Uses ServiceAccount credentials from in-cluster config
- Fails with actionable RBAC error if permissions missing

**AC2**: Service validates prometheus HTTP connectivity at startup
- HTTP GET to `{PROMETHEUS_URL}/api/v1/query?query=up`
- Timeout: 5 seconds
- Validates response JSON format: `{"status": "success"}`

**AC3**: Service validates grafana HTTP connectivity at startup (if endpoint configured)
- HTTP GET to `{GRAFANA_URL}/api/health`
- Timeout: 5 seconds
- Skips validation if `GRAFANA_URL` not set (optional toolset)

**AC4**: Service exits with non-zero exit code if ANY enabled toolset validation fails
- Exit code: 1
- Logs detailed error message before exit
- Does NOT start FastAPI server

**AC5**: Service logs validation progress for each toolset
- Log level: INFO
- Format: `"toolset_validated", toolset=<name>`
- Final log: `"all_toolsets_validated", count=<N>`

**AC6**: Service starts FastAPI server ONLY after ALL validations pass
- FastAPI lifespan startup completes
- `/health` endpoint becomes available
- Service ready to accept investigation requests

### Rationale

**Problem**: Toolset unavailability causes runtime failures during investigations, leading to:
- User-facing errors with unclear root cause
- Wasted investigation time and LLM API costs
- Difficult troubleshooting (is it RBAC? connectivity? configuration?)

**Solution**: Fail-fast validation at startup ensures:
- Immediate feedback on configuration issues
- Clear error messages with troubleshooting guidance
- Prevents runtime cascading failures
- Reduces mean-time-to-resolution (MTTR) for deployment issues

**Business Impact**:
- ⬇️ Reduces deployment failures by 80% (configuration issues caught early)
- ⬇️ Reduces MTTR from 30 minutes to 5 minutes (immediate error feedback)
- ⬆️ Increases service reliability to 99.9% uptime

---

## BR-HAPI-187: Startup Validation Error Messages

**Priority**: P1 - HIGH
**Category**: Operational Excellence
**Related Services**: HolmesGPT-API (port 8090)

### Requirement

The service MUST provide actionable error messages for toolset validation failures.

**Error Message Structure**:

```
Service startup failed: <N> toolset(s) unavailable:
  - Toolset '<name>' validation failed: <specific_failure_reason>
    <troubleshooting_guidance>
    <documentation_reference>

Set HOLMESGPT_DEV_MODE=true to skip validation (development only)
```

**Required Components**:
1. **Toolset name**: Exact toolset that failed (`kubernetes`, `prometheus`, `grafana`)
2. **Failure reason**: Specific error (RBAC denied, connection timeout, HTTP 404, etc.)
3. **Troubleshooting guidance**: Actionable steps to resolve issue
4. **Documentation reference**: Link to relevant documentation section

**Toolset-Specific Guidance**:

| Toolset | Error Type | Troubleshooting Guidance |
|---------|-----------|-------------------------|
| **kubernetes** | RBAC denied | "Ensure ServiceAccount has ClusterRole 'holmesgpt-api-kubernetes-toolset'. See RBAC section in 08-holmesgpt-api.md" |
| **kubernetes** | API unreachable | "Check KUBERNETES_IN_CLUSTER setting and API server connectivity" |
| **prometheus** | Connection failed | "Check Prometheus service is running and PROMETHEUS_URL is correct: {url}" |
| **prometheus** | HTTP 404 | "Prometheus endpoint not found. Verify PROMETHEUS_URL: {url}" |
| **grafana** | Connection failed | "Check Grafana service is running and GRAFANA_URL is correct: {url}" |

### Acceptance Criteria

**AC1**: Error message includes toolset name in format `"Toolset '{name}' validation failed"`
- Example: `"Toolset 'kubernetes' validation failed"`
- Uses exact toolset name from ConfigMap

**AC2**: Error message includes specific failure reason
- RBAC errors: Include permission name (`list pods`, `read logs`)
- HTTP errors: Include status code and URL
- Timeout errors: Include timeout duration

**AC3**: kubernetes toolset errors include ServiceAccount and ClusterRole names
- ServiceAccount: `holmesgpt-api`
- ClusterRole: `holmesgpt-api-kubernetes-toolset`
- Namespace: `kubernaut-system`

**AC4**: prometheus/grafana errors include endpoint URL
- Full URL from environment variable
- Example: `"Connection failed to http://prometheus:9090"`

**AC5**: Error message includes documentation reference
- Format: `"See {section} in {file}"`
- Example: `"See RBAC section in 08-holmesgpt-api.md"`

**AC6**: Error message suggests dev mode override
- Footer: `"Set HOLMESGPT_DEV_MODE=true to skip validation (development only)"`
- Only shown in non-production environments (optional)

### JSON Structured Logging Format

```json
{
  "level": "error",
  "message": "Service startup failed",
  "failed_toolsets": 2,
  "validation_errors": [
    {
      "toolset": "kubernetes",
      "failure_reason": "RBAC permission denied for 'list pods'",
      "troubleshooting": "Ensure ServiceAccount has ClusterRole 'holmesgpt-api-kubernetes-toolset'",
      "documentation": "See RBAC section in 08-holmesgpt-api.md",
      "timestamp": "2025-01-15T10:30:00Z"
    },
    {
      "toolset": "prometheus",
      "failure_reason": "Connection failed to http://prometheus:9090",
      "troubleshooting": "Check Prometheus service is running and network connectivity",
      "documentation": "See Configuration section in 08-holmesgpt-api.md",
      "timestamp": "2025-01-15T10:30:01Z"
    }
  ],
  "dev_mode_override": "Set HOLMESGPT_DEV_MODE=true to skip validation (development only)"
}
```

### Rationale

**Problem**: Generic error messages increase troubleshooting time:
- "Toolset validation failed" → No actionable information
- Users waste time guessing: RBAC? Connectivity? Configuration?
- Support tickets increase operational burden

**Solution**: Actionable error messages with specific guidance reduce MTTR:
- Immediate identification of root cause
- Step-by-step troubleshooting guidance
- Documentation links for detailed resolution

**Business Impact**:
- ⬇️ Reduces MTTR from 30 minutes to 5 minutes (70% improvement)
- ⬇️ Reduces support tickets by 60% (self-service troubleshooting)
- ⬆️ Improves developer satisfaction (clear feedback)

---

## BR-HAPI-188: Development Mode Override

**Priority**: P1 - HIGH
**Category**: Developer Productivity
**Related Services**: HolmesGPT-API (port 8090)

### Requirement

The service MUST support a development mode override (`HOLMESGPT_DEV_MODE`) to skip startup validation.

**Configuration**:
- **Environment Variable**: `HOLMESGPT_DEV_MODE`
- **Default**: `false` (production behavior)
- **Valid Values**: `true`, `True`, `TRUE`, `1` (enable dev mode), all others treated as `false`

**Behavior**:

| Mode | Validation | Service Startup | Logging |
|------|-----------|-----------------|---------|
| **Production** (`false`) | ✅ Full validation | Fails if toolsets unavailable | Standard validation logs |
| **Development** (`true`) | ⚠️ Skipped | Proceeds regardless of toolsets | Warning: "toolset validation skipped" |

**Security Consideration**:
- Dev mode is UNSAFE for production (bypasses critical safety checks)
- Deployment manifests MUST explicitly set `HOLMESGPT_DEV_MODE=false`
- Kubernetes admission controller SHOULD reject dev mode in production namespaces

### Acceptance Criteria

**AC1**: Service reads `HOLMESGPT_DEV_MODE` environment variable at startup
- Read during `ToolsetConfigService.__init__()`
- Parse before validation logic execution

**AC2**: Default value is `false` if environment variable not set
- Missing env var → Production behavior
- Empty string → Production behavior

**AC3**: Values `"true"`, `"True"`, `"TRUE"`, `"1"` enable dev mode
- Case-insensitive string comparison
- String "1" treated as true (common convention)

**AC4**: All other values treated as `false`
- `"false"`, `"False"`, `"FALSE"`, `"0"` → Production behavior
- Invalid values (e.g., `"yes"`, `"enabled"`) → Production behavior (safe default)

**AC5**: When dev mode enabled, service logs warning
- Log level: WARNING
- Message: `"dev_mode_enabled_skipping_validation", warning="HOLMESGPT_DEV_MODE=true - toolset validation skipped"`
- Logged before skipping validation

**AC6**: When dev mode enabled, service proceeds to FastAPI startup without validation
- Skip `_validate_all_toolsets_at_startup()` call
- Continue to lifespan startup
- FastAPI server starts regardless of toolset availability

### Observability

**Health Endpoint**:
```json
{
  "status": "healthy",
  "dev_mode_enabled": true,
  "warning": "Development mode active - toolset validation skipped"
}
```

**Prometheus Metrics**:
```python
holmesgpt_dev_mode_enabled = Gauge(
    'holmesgpt_dev_mode_enabled',
    'Whether development mode is enabled (1=enabled, 0=disabled)'
)
```

**Use Case**: Alerting on accidental production dev mode usage:
```yaml
# Prometheus alert rule
- alert: HolmesGPTDevModeInProduction
  expr: holmesgpt_dev_mode_enabled{namespace="kubernaut-system"} == 1
  for: 5m
  severity: critical
  annotations:
    summary: "HolmesGPT-API running in dev mode in production"
    description: "HOLMESGPT_DEV_MODE=true in namespace {{ $labels.namespace }}"
```

### Rationale

**Problem**: Local development requires full infrastructure:
- Kubernetes cluster with RBAC configured
- Prometheus instance with data
- Grafana instance (optional)
- Developers cannot iterate quickly without infrastructure

**Solution**: Dev mode enables local development:
- Skip infrastructure validation
- Use mock toolset responses
- Rapid iteration and testing

**Business Impact**:
- ⬆️ Increases developer velocity by 50% (faster iteration)
- ⬇️ Reduces local development setup time from 2 hours to 10 minutes
- ⬆️ Enables unit testing without infrastructure dependencies

---

## BR-HAPI-189: Runtime Toolset Failure Tracking

**Priority**: P1 - HIGH
**Category**: Runtime Resilience
**Related Services**: HolmesGPT-API (port 8090)

### Requirement

The service MUST track toolset failures during investigations to enable automatic recovery.

**Tracking Mechanism**:
- In-memory failure counter dictionary: `{toolset_name: failure_count}`
- Increment counter on toolset-specific investigation errors
- Reset counter to 0 on successful toolset usage
- Maintain separate counters per toolset (kubernetes, prometheus, grafana)

**Toolset Error Classification**:

| Toolset | Error Detection Criteria |
|---------|------------------------|
| **kubernetes** | • Exception message contains: `kubernetes`, `rbac`, `forbidden`, `unauthorized`<br>• HTTP 403 from Kubernetes API<br>• Connection refused to API server |
| **prometheus** | • Exception message contains: `prometheus`, `promql`<br>• HTTP 4xx/5xx from Prometheus endpoint<br>• Connection refused to Prometheus endpoint |
| **grafana** | • Exception message contains: `grafana`<br>• HTTP 4xx/5xx from Grafana endpoint<br>• Connection refused to Grafana endpoint |

### Acceptance Criteria

**AC1**: Service maintains in-memory failure counter dictionary
- Data structure: `Dict[str, int]` mapping toolset name to failure count
- Initialized at service startup
- Persists until service restart (no disk persistence required)

**AC2**: On investigation error, service increments counter for detected toolset
- Error detection uses classification criteria above
- Single investigation can increment multiple toolsets if multiple errors detected
- Unknown errors (no toolset detected) do NOT increment any counter

**AC3**: On investigation success, service resets counters for all toolsets used
- HolmesGPT SDK returns `result.toolsets_used` list
- Reset counter to 0 for each toolset in list
- Prevents false positives from intermittent failures

**AC4**: Service logs failure count increment at WARN level
- Log format: `"toolset_failure_recorded", toolset=<name>, failure_count=<N>, max_failures=<threshold>`
- Includes current count and configured threshold for context

**AC5**: Failure counters persist until service restart
- No persistence to disk/database
- Reset to 0 on service restart
- Rationale: Service restart is itself a recovery mechanism

### Edge Cases

**Multiple Toolsets Fail in Single Investigation**:
- Increment counter for ALL detected toolsets
- Example: kubernetes RBAC error + prometheus connection error → Increment both

**Unknown Error (No Toolset Detected)**:
- Log error at ERROR level
- Do NOT increment any counter
- Example: Generic timeout with no toolset context

**Timeout Errors**:
- Classify by which toolset was queried (inspect HolmesGPT SDK context)
- If SDK doesn't provide context, use last toolset called before timeout
- Fallback: Log as unknown error if toolset cannot be determined

### Implementation Pattern

```python
# In HolmesGPTService.investigate()
try:
    result = await holmes_client.investigate(...)

    # Success: Reset failure counters
    if toolset_config_service and result.toolsets_used:
        for toolset_name in result.toolsets_used:
            toolset_config_service.record_toolset_success(toolset_name)

    return result

except Exception as e:
    # Toolset failure: Increment counter
    error_msg = str(e).lower()

    if 'kubernetes' in error_msg or 'rbac' in error_msg or 'forbidden' in error_msg:
        toolset_config_service.record_toolset_failure('kubernetes')
    elif 'prometheus' in error_msg or 'promql' in error_msg:
        toolset_config_service.record_toolset_failure('prometheus')
    elif 'grafana' in error_msg:
        toolset_config_service.record_toolset_failure('grafana')
    else:
        logger.error("investigation_error_unknown_toolset", error=str(e))

    raise
```

### Rationale

**Problem**: Transient toolset failures are invisible:
- kubernetes RBAC changes (ServiceAccount rotated)
- prometheus endpoint migration (service IP changed)
- Network partitions (temporary connectivity loss)
- Service can't self-heal because it doesn't know failures are occurring

**Solution**: Failure tracking enables automatic detection and recovery:
- Detect persistent failures (vs. intermittent)
- Trigger automatic recovery (ConfigMap reload)
- Provide visibility into toolset health

**Business Impact**:
- ⬆️ Increases service availability from 99% to 99.9% (self-healing)
- ⬇️ Reduces manual intervention by 80% (automatic recovery)
- ⬇️ Reduces investigation failure rate from 5% to 0.5%

---

## BR-HAPI-190: Auto-Reload ConfigMap on Persistent Failures

**Priority**: P1 - HIGH
**Category**: Runtime Resilience
**Related Services**: HolmesGPT-API (port 8090)

### Requirement

The service MUST automatically reload ConfigMap after N consecutive toolset failures.

**Configuration**:
- **Environment Variable**: `TOOLSET_MAX_FAILURES_BEFORE_RELOAD`
- **Default**: `3` consecutive failures
- **Valid Range**: 1-10 (configurable for different environments)

**Consecutive Failure Definition**:
- **Consecutive**: N failures in a row WITHOUT successful investigation
- If toolset succeeds after M failures (M < N) → Reset counter to 0
- Failures tracked independently per toolset (do NOT combine across toolsets)

**Example**:
```
kubernetes fails 2 times → counter: 2
kubernetes succeeds 1 time → counter: 0 (reset)
kubernetes fails 3 times → counter: 3 → Trigger reload
```

**Reload Behavior**:

| Scenario | Action |
|----------|--------|
| **Failure count reaches threshold** | Force ConfigMap file re-check (call `_check_file_changes()`) |
| **ConfigMap file mtime unchanged** | Log "ConfigMap unchanged, toolset still unavailable" |
| **ConfigMap file mtime changed** | Parse new config, reinitialize HolmesGPT SDK gracefully |
| **New config ALSO fails validation** | Log error, do NOT reset failure counter (prevents retry loop) |
| **New config validates successfully** | Reset failure counter to 0, apply new toolsets |

### Acceptance Criteria

**AC1**: Service reads `TOOLSET_MAX_FAILURES_BEFORE_RELOAD` environment variable at startup
- Parse during `ToolsetConfigService.__init__()`
- Validate value is integer in range 1-10
- Default to 3 if invalid or missing

**AC2**: Default threshold is 3 if environment variable not set
- Missing env var → Use default 3
- Empty string → Use default 3

**AC3**: When toolset failure count reaches threshold, service forces ConfigMap re-check
- Call `_check_file_changes()` method (existing polling logic)
- Bypass normal polling interval (immediate check)
- Log trigger at ERROR level

**AC4**: ConfigMap re-check uses existing `_check_file_changes()` method
- Reuse polling infrastructure (no duplicate logic)
- Check file mtime and content checksum
- Parse YAML if changed

**AC5**: After ConfigMap reload attempt, service resets failure counter to 0
- Reset counter for toolset that triggered reload
- Only reset if new config validates successfully
- Do NOT reset if new config also fails (prevents infinite retry loop)

**AC6**: Service logs reload trigger at ERROR level
- Format: `"toolset_max_failures_reached_reloading_config", toolset=<name>, failures=<N>`
- Includes toolset name and failure count for troubleshooting

### ConfigMap Reload Behavior Details

**Reload Does NOT Interrupt Active Investigations**:
- Reload queued via `pending_reload` flag (graceful reload pattern)
- Active investigations continue with old toolset configuration
- New investigations after reload applied use new toolset configuration

**If ConfigMap File Unchanged**:
- Log: `"ConfigMap unchanged, toolset still unavailable"`
- Do NOT reset failure counter (configuration hasn't changed)
- Service continues with failures (no recovery possible without config change)

**If ConfigMap File Changed**:
- Parse new YAML configuration
- Validate new toolsets (same logic as startup validation if not in dev mode)
- If validation passes → Reinitialize HolmesGPT SDK with new toolsets
- If validation fails → Log error, do NOT apply new config, do NOT reset counter

**If New Config Also Fails Validation**:
- Log: `"ConfigMap reload failed validation", toolset=<name>, error=<reason>`
- Do NOT reset failure counter (prevents infinite reload loop)
- Service continues tracking failures (will trigger reload again after N more failures)

### Rationale

**Problem**: Configuration drift causes persistent failures:
- Kubernetes RBAC policy changed (ServiceAccount permissions revoked)
- Prometheus service migrated (endpoint URL changed)
- Toolset disabled in ConfigMap (admin manually edited)
- Manual intervention required to recover (increases MTTR)

**Solution**: Auto-reload ConfigMap detects configuration changes:
- Automatically checks for updated configuration
- Applies new toolsets if configuration fixed
- Self-heals without manual intervention

**Business Impact**:
- ⬇️ Reduces MTTR from 30 minutes to 3 minutes (automatic detection + reload)
- ⬆️ Increases service availability from 99.5% to 99.9% (self-healing)
- ⬇️ Reduces operational burden by 80% (no manual intervention)

---

## BR-HAPI-191: Graceful Toolset Reload

**Priority**: P1 - HIGH
**Category**: User Experience
**Related Services**: HolmesGPT-API (port 8090)

### Requirement

The service MUST preserve active investigation sessions during ConfigMap reload to prevent investigation disruption.

**Session Management**:
- Track active investigation session IDs
- Queue toolset reload if sessions active (`pending_reload` flag)
- Apply reload when last session completes
- All new investigations after reload applied use updated toolsets

**Session Lifecycle**:
```
Investigation Start → register_session(session_id)
Investigation Complete/Error → unregister_session(session_id)
If last session completes AND pending_reload=true → Apply reload
```

### Acceptance Criteria

**AC1**: Service registers investigation session ID when `/investigate` endpoint called
- Generate UUID for each investigation
- Register session before calling HolmesGPT SDK
- Format: `toolset_service.register_session(session_id)`

**AC2**: Service unregisters session ID when investigation completes
- Call `unregister_session()` in try/finally block (guarantees cleanup)
- Unregister on both success and error cases
- Format: `toolset_service.unregister_session(session_id)`

**AC3**: If ConfigMap reload triggered while sessions active, set `pending_reload` flag
- Check `_has_active_sessions()` before applying reload
- If sessions active → Set `pending_reload=True`, store new toolsets
- Log: `"toolset_reload_queued", active_sessions=<N>`

**AC4**: When last active session completes, apply pending reload immediately
- Check in `unregister_session()`: if `len(active_sessions) == 0 and pending_reload`
- Call `_reload_toolsets()` with pending toolsets
- Log: `"last_session_completed_applying_pending_reload"`

**AC5**: New investigations after pending reload use OLD toolsets until reload applied
- Reload does NOT apply mid-investigation
- New investigations started during pending reload use OLD toolsets
- Rationale: Consistency - all investigations complete with original toolsets

**AC6**: Service logs session count when queuing reload
- Format: `"toolset_reload_queued", active_sessions=<N>`
- Provides visibility into why reload is delayed

### Session Timeout Handling

**Problem**: Stale sessions block reload indefinitely
- Investigation hangs (network timeout, LLM API failure)
- Session never unregistered
- Reload queued forever

**Solution**: Force-unregister stale sessions after timeout

**AC7**: Service force-unregisters sessions after 10 minutes
- Background thread checks session timestamps every 60 seconds
- If session registered >10 minutes ago → Force unregister
- Log warning: `"stale_session_force_unregistered", session_id=<id>, age=<duration>`

**Implementation**:
```python
def register_session(self, session_id: str):
    self.active_sessions[session_id] = time.time()  # Store timestamp

def _cleanup_stale_sessions(self):
    """Background thread: cleanup stale sessions"""
    current_time = time.time()
    stale_threshold = 600  # 10 minutes

    for session_id, start_time in list(self.active_sessions.items()):
        age = current_time - start_time
        if age > stale_threshold:
            logger.warning("stale_session_force_unregistered",
                          session_id=session_id, age=age)
            del self.active_sessions[session_id]

    # Check if pending reload can now be applied
    if self.pending_reload and not self._has_active_sessions():
        self._reload_toolsets(self.pending_toolsets)
```

### Rationale

**Problem**: Non-graceful reload disrupts investigations:
- Toolsets changed mid-investigation → Inconsistent data sources
- Investigation fails with cryptic error
- User has to retry investigation (poor UX)

**Solution**: Graceful reload preserves investigation integrity:
- Active investigations complete with original toolsets
- New investigations use updated toolsets
- Seamless user experience

**Business Impact**:
- ⬆️ Increases user satisfaction (no investigation disruption)
- ⬇️ Reduces investigation failure rate from 5% to 0.5% (during reload)
- ⬆️ Increases perceived service reliability (seamless updates)

---

## Summary

### BR Coverage Matrix

| BR | Priority | Category | Implementation Effort | Business Impact |
|----|----------|----------|---------------------|----------------|
| **BR-HAPI-186** | P0 | Service Reliability | Medium (1-2 days) | CRITICAL - Prevents runtime failures |
| **BR-HAPI-187** | P1 | Operational Excellence | Low (4-6 hours) | HIGH - Reduces troubleshooting time |
| **BR-HAPI-188** | P1 | Developer Productivity | Low (2-4 hours) | MEDIUM - Enables local development |
| **BR-HAPI-189** | P1 | Runtime Resilience | Medium (1 day) | HIGH - Enables self-healing |
| **BR-HAPI-190** | P1 | Runtime Resilience | Medium (1 day) | HIGH - Automatic recovery |
| **BR-HAPI-191** | P1 | User Experience | Low (4-6 hours) | MEDIUM - Prevents disruption |

### Implementation Phases

**Phase 1 (P0 - Week 1)**:
- BR-HAPI-186: Fail-Fast Startup Validation
- Estimated: 2 days
- Deliverable: Service validates all toolsets at startup, exits if any fail

**Phase 2 (P1 - Week 2)**:
- BR-HAPI-187: Startup Validation Error Messages
- BR-HAPI-188: Development Mode Override
- BR-HAPI-189: Runtime Toolset Failure Tracking
- BR-HAPI-191: Graceful Toolset Reload
- Estimated: 3 days
- Deliverable: Production-ready error handling + dev mode + session tracking

**Phase 3 (P1 - Week 3)**:
- BR-HAPI-190: Auto-Reload ConfigMap on Persistent Failures
- Estimated: 1 day
- Deliverable: Complete self-healing system

**Total Estimated Effort**: 6 days (1.5 weeks)

### Testing Requirements

**Unit Tests (70%+ coverage)**:
- BR-HAPI-186: Mock kubernetes client, httpx for validation (pytest)
- BR-HAPI-187: Validate error message format and content
- BR-HAPI-188: Test environment variable parsing and dev mode logic
- BR-HAPI-189: Test failure counter increment/reset logic
- BR-HAPI-190: Test ConfigMap reload trigger conditions
- BR-HAPI-191: Test session tracking and graceful reload queuing

**Integration Tests (20% coverage)**:
- BR-HAPI-186: Real kubernetes API calls, real Prometheus/Grafana endpoints
- BR-HAPI-189/190: Simulate toolset failures, verify auto-reload
- BR-HAPI-191: Test graceful reload with concurrent active sessions

**E2E Tests (10% coverage)**:
- Complete investigation workflow with fail-fast validation
- Runtime resilience: Persistent failures → Auto-reload → Recovery
- Graceful reload: Active investigation preservation during ConfigMap update

### Metrics & Observability

**Service Reliability Metrics**:
```python
holmesgpt_startup_validation_duration_seconds = Histogram(
    'holmesgpt_startup_validation_duration_seconds',
    'Duration of startup validation by toolset',
    ['toolset']
)

holmesgpt_startup_validation_failures_total = Counter(
    'holmesgpt_startup_validation_failures_total',
    'Total startup validation failures',
    ['toolset', 'failure_reason']
)

holmesgpt_toolset_failure_count = Gauge(
    'holmesgpt_toolset_failure_count',
    'Current failure count per toolset',
    ['toolset']
)

holmesgpt_configmap_reload_total = Counter(
    'holmesgpt_configmap_reload_total',
    'Total ConfigMap reload attempts',
    ['trigger', 'result']  # trigger: failure_threshold|periodic, result: success|failed
)

holmesgpt_active_investigation_sessions = Gauge(
    'holmesgpt_active_investigation_sessions',
    'Number of active investigation sessions'
)

holmesgpt_dev_mode_enabled = Gauge(
    'holmesgpt_dev_mode_enabled',
    'Whether development mode is enabled (1=enabled, 0=disabled)'
)
```

### Documentation Updates

**Files to Update**:
1. `docs/services/stateless/08-holmesgpt-api.md` - Add BR references in relevant sections
2. `docs/requirements/README.md` - Add BR-HAPI-186 to BR-HAPI-191 to BR index
3. `docker/holmesgpt-api/README.md` - Document HOLMESGPT_DEV_MODE and TOOLSET_MAX_FAILURES_BEFORE_RELOAD

---

## Approval

**Status**: APPROVED
**Date**: 2025-01-15
**Approved By**: Architecture Review
**Confidence**: 96% (Very High)

**Next Steps**:
1. ✅ Create implementation tasks in project tracker
2. ✅ Update `08-holmesgpt-api.md` with BR references
3. ✅ Create testing strategy document
4. ✅ Begin Phase 1 implementation (BR-HAPI-186)

