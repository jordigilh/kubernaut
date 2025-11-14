# RCA Toolset Integration Status

**Date**: November 14, 2025  
**Status**: 🔧 FIXES APPLIED - READY FOR TESTING

## Changes Made

### 1. Toolset Configuration ✅
**File**: `holmesgpt-api/src/extensions/recovery.py`

- Updated `_get_holmes_config()` to accept `app_config` parameter
- Added default toolsets configuration:
  ```python
  toolsets_config = {
      "kubernetes": {"enabled": True},
      "prometheus": {"enabled": True}
  }
  ```
- Toolsets are now passed to HolmesGPT SDK Config via `toolsets` parameter

### 2. Priority/Severity Mapping Fix ✅
**File**: `holmesgpt-api/src/extensions/recovery.py` - `_get_playbook_recommendations()`

**Problem**: Test used `"priority": "P1"` but code expected `"priority": "high"`

**Fix**: Added detection for P-format priorities:
```python
# Check if priority is already in P-format (P0, P1, P2, P3)
if isinstance(priority, str) and priority.upper().startswith("P") and priority[1:].isdigit():
    severity = priority.upper()
else:
    # Map text priority to severity
    severity_map = {"critical": "P0", "high": "P1", "medium": "P2", "low": "P3"}
    severity = severity_map.get(priority.lower(), "P2")
```

### 3. Debug Logging Enabled ✅
**File**: `deploy/llm-validation/deploy-mcp-anthropic.yaml`

- Changed `log_level: "INFO"` to `log_level: "DEBUG"`
- Added debug logging before SDK call to trace:
  - Prompt length
  - Playbook count
  - Toolsets enabled
  - Prompt preview

### 4. App Config Propagation ✅
**File**: `holmesgpt-api/src/extensions/recovery.py`

- Updated `analyze_recovery()` to accept `app_config` parameter
- Updated `recovery_analyze_endpoint()` to pass `app_config` from `router.config`
- Removed duplicate exception handling code

### 5. ConfigMap Toolsets ✅
**File**: `deploy/llm-validation/deploy-mcp-anthropic.yaml`

Added toolsets configuration to ConfigMap:
```yaml
toolsets:
  kubernetes:
    enabled: true
  prometheus:
    enabled: true
```

## Issues Fixed

### Issue 1: Toolsets Not Enabled
**Symptom**: LLM complained "NO ENABLED KUBERNETES TOOLSET"  
**Root Cause**: `_get_holmes_config()` wasn't receiving `app_config` with toolsets  
**Fix**: Pass `app_config` through the call chain

### Issue 2: Playbooks Not Found
**Symptom**: Mock MCP Server returned 0 playbooks  
**Root Cause**: Priority "P1" was being mapped to severity "P2"  
**Fix**: Handle both P-format and text format priorities

### Issue 3: No Tool Calls
**Symptom**: `"tool_calls": 0` in response metadata  
**Root Cause**: Toolsets weren't enabled in HolmesGPT SDK Config  
**Fix**: Pass toolsets configuration to SDK

## Testing Required

### Test 1: Verify Toolsets Are Enabled
```bash
# Check logs for toolsets configuration
kubectl logs -n kubernaut-system -l app=holmesgpt-api --tail=100 | grep "toolsets"
```

**Expected**: Should see `toolsets={'kubernetes': {'enabled': True}, 'prometheus': {'enabled': True}}`

### Test 2: Verify Playbooks Are Found
```bash
# Send test request
curl -X POST http://localhost:8080/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d @test/llm-validation/test-e2e-with-rca.json
```

**Expected**: 
- Mock MCP Server logs should show: `Response: 2 playbooks returned`
- API response should include playbook recommendations in the prompt

### Test 3: Verify LLM Uses Tools
```bash
# Check for tool calls in response
kubectl logs -n kubernaut-system -l app=holmesgpt-api --tail=200 | grep "Tool Calls"
```

**Expected**: `Tool Calls: N` where N > 0 (LLM called Kubernetes/Prometheus tools)

### Test 4: Verify RCA Before Remediation
```bash
# Check LLM response for investigation steps
kubectl logs -n kubernaut-system -l app=holmesgpt-api --tail=300 | grep -A20 "RAW LLM RESPONSE"
```

**Expected**: LLM should:
1. Use Kubernetes tools to check pod status
2. Use Prometheus tools to check metrics
3. Perform RCA based on actual cluster data
4. Provide remediation based on RCA findings

## Mock MCP Server Playbooks

The Mock MCP Server has 2 playbooks for OOMKill + P1:

### Playbook 1: General OOMKill (confidence: 0.9)
- **Signal**: OOMKill
- **Severity**: P1
- **Component**: * (any)
- **Steps**: Check logs, review limits, analyze usage, increase limits, optimize

### Playbook 2: Cost-Management OOMKill (confidence: 0.95)
- **Signal**: OOMKill
- **Severity**: P1
- **Component**: * (any)
- **Business Category**: cost-management
- **Steps**: Cost-aware remediation with budget constraints

## Expected End-to-End Flow

```
1. User sends recovery request with:
   - error: "OOMKill"
   - priority: "P1"
   - business_category: "cost-management"

2. HolmesGPT API:
   a. Calls Mock MCP Server → Gets 2 playbooks
   b. Creates prompt with playbooks + RCA instructions
   c. Initializes HolmesGPT SDK with toolsets enabled
   d. SDK calls LLM with tools available

3. LLM (Claude Haiku):
   a. Sees available tools: Kubernetes, Prometheus, MCP
   b. Calls Kubernetes tools to check pod status
   c. Calls Prometheus tools to check memory metrics
   d. Performs RCA: "Pod exceeded 512Mi limit"
   e. Reviews playbooks for OOMKill + P1
   f. Recommends: Increase memory limit to 1Gi (from playbook)

4. Response includes:
   - RCA findings
   - Playbook-based remediation
   - tool_calls > 0
   - High confidence
```

## Files Modified

1. `holmesgpt-api/src/extensions/recovery.py`
   - `_get_holmes_config()` - Accept app_config, enable toolsets
   - `_get_playbook_recommendations()` - Fix priority mapping
   - `analyze_recovery()` - Accept app_config parameter
   - `recovery_analyze_endpoint()` - Pass app_config
   - Added debug logging

2. `deploy/llm-validation/deploy-mcp-anthropic.yaml`
   - Changed log_level to DEBUG
   - Added toolsets configuration

3. `test/llm-validation/test-e2e-with-rca.json` (NEW)
   - Comprehensive test case with P1 priority

## Next Steps

1. **Rebuild and Deploy** (if code changes require it)
2. **Run Test Suite** - Execute all 4 tests above
3. **Verify Logs** - Check debug logs for toolset usage
4. **Monitor Tool Calls** - Ensure LLM is calling Kubernetes/Prometheus
5. **Validate RCA** - Confirm LLM performs investigation before remediation

## Success Criteria

- ✅ Toolsets appear in config logs
- ✅ Mock MCP Server returns 2 playbooks
- ✅ LLM makes tool calls (tool_calls > 0)
- ✅ Response includes RCA findings
- ✅ Remediation aligns with playbook recommendations
- ✅ Confidence score reflects actual investigation

## Known Limitations

1. **In-Cluster Access Required**: Kubernetes and Prometheus tools only work when HolmesGPT API runs inside the cluster
2. **RBAC Permissions**: ServiceAccount needs proper permissions (already configured)
3. **Prometheus URL**: Must be accessible from pod (configured for OpenShift)
4. **Mock Data**: Mock MCP Server uses static playbook data

## Rollback Plan

If issues occur:
```bash
# Revert ConfigMap
kubectl apply -f deploy/llm-validation/deploy-mcp-anthropic.yaml.backup

# Restart pods
kubectl rollout restart deployment/holmesgpt-api -n kubernaut-system
```

## Contact

For questions or issues, check:
- HolmesGPT API logs: `kubectl logs -n kubernaut-system -l app=holmesgpt-api`
- Mock MCP Server logs: `kubectl logs -n kubernaut-system -l app=mock-mcp-server`
- Pod status: `kubectl get pods -n kubernaut-system`

