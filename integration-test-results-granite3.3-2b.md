# Integration Test Results - Granite 3.3 2B Model

**Test Date**: August 27, 2025  
**Model**: granite3.3:2b  
**Ollama Endpoint**: http://localhost:11434  

## Summary

- **Total Tests**: 49 integration tests (extended test suite)
- **Status**: ⚠️ **Mixed results** (42 passed, 7 failed - 85.7% success rate)
- **Total Runtime**: 107.34s
- **Average Response Time**: 2.19s (per response)
- **Max Response Time**: 3.36s

## Detailed Test Duration Breakdown

### Core Connectivity Tests
- `TestOllamaConnectivity`: 0.00s (instant - fake client health check)
- `TestModelAvailability`: 2.28s (SLM model validation) ⚡ **Fast validation**

### Alert Analysis Scenarios (30.31s total) ⚡ **2.2x faster than 8B**
- `DeploymentFailureRollback`: 4.03s ✅ - rollback_deployment action
- `StorageSpaceExhaustion`: 3.04s ✅ - expand_pvc action  
- `NodeMaintenanceRequired`: 2.17s ✅ - drain_node action
- `SecurityThreatDetected`: 2.87s ✅ - quarantine_pod action
- `ComplexTroubleshooting`: 2.63s ❌ - restart_pod (expected collect_diagnostics)
- `HighMemoryUsage`: 3.03s ✅ - scale_deployment action
- `PodCrashLooping`: 2.44s ✅ - restart_pod action
- `CPUThrottling`: 2.49s ✅ - scale_deployment action
- `DeploymentReplicasMismatch`: 2.14s ✅ - scale_deployment action
- `LowSeverityDiskSpace`: 2.54s ✅ - expand_pvc action
- `NetworkConnectivityIssue`: 2.62s ❌ - scale_deployment (expected restart_pod)
- `TestConnectivity`: 2.33s ✅ - scale_deployment action

### End-to-End Action Execution (13.50s total) ⚡ **Excellent performance**
- `DeploymentFailureRollback_EndToEnd`: 3.36s ✅ - Full K8s execution
- `StorageSpaceExhaustion_EndToEnd`: 2.57s ✅ - PVC expansion workflow
- `NodeMaintenanceRequired_EndToEnd`: 2.26s ✅ - Node drain execution
- `SecurityThreatDetected_EndToEnd`: 2.69s ✅ - Pod quarantine workflow
- `HighMemoryUsage_EndToEnd`: 2.62s ✅ - Scaling execution

### Security & RBAC Tests (13.56s total)
- `Scale_Permission_Denied`: 3.35s ❌ - Incorrect action (expected error handling)
- `Restart_Permission_Denied`: 2.32s ✅ - Proper fallback
- `NotifyOnly_Always_Allowed`: 2.27s ✅ - Notification workflow
- `SecurityIncident_privilege_escalation`: 3.13s ❌ - Insufficient security response
- `SecurityIncident_data_exfiltration`: 2.51s ✅ - Appropriate quarantine

### Resource Exhaustion Tests (6.01s total) ⚡ **2.4x faster than 8B**
- `ResourceExhaustion_inode_exhaustion`: 3.18s ✅ - expand_pvc action
- `ResourceExhaustion_network_bandwidth`: 2.83s ✅ - scale_deployment action

### Workflow Resilience Tests (6.79s total) ⚡ **2.0x faster than 8B**
- `Basic_Execution`: 2.48s ✅ - rollback_deployment with K8s execution
- `Health_Check`: 2.30s ✅ - rollback_deployment with health validation
- `Resource_Consistency`: 2.02s ✅ - rollback_deployment with consistency check

## Failed Test Analysis

### 1. ComplexTroubleshooting (Expected: collect_diagnostics, Got: restart_pod)
- **Issue**: Model prioritizes immediate remediation over investigation
- **Reasoning**: "The alert indicates service degradation with performance issues. Restarting the affected pods should help resolve temporary issues"
- **Impact**: Masks underlying problems, prevents root cause analysis

### 2. NetworkConnectivityIssue (Expected: restart_pod, Got: scale_deployment)
- **Issue**: Model treats connectivity as capacity problem
- **Reasoning**: "Network connectivity issues could be due to high traffic. Scaling up deployment replicas might help distribute the load"
- **Impact**: Ineffective solution for network problems

### 3. Scale_Permission_Denied (Expected: notify_only, Got: scale_deployment)
- **Issue**: Model doesn't handle RBAC permission failures appropriately
- **Reasoning**: Attempts scaling despite permission constraints
- **Impact**: Would fail in production with proper RBAC controls

### 4. SecurityIncident_privilege_escalation (Expected: quarantine_pod, Got: restart_pod)
- **Issue**: Insufficient security response to privilege escalation
- **Reasoning**: "The alert indicates privilege escalation attempt. Restarting the pods should resolve the immediate security threat"
- **Impact**: Critical security gap - doesn't isolate compromised workloads

## Action Distribution

✅ **Well-balanced action usage across most scenarios**
- `scale_deployment`: 15 actions (load balancing and resource scaling)
- `restart_pod`: 8 actions (service recovery and basic remediation)
- `expand_pvc`: 6 actions (storage expansion)
- `rollback_deployment`: 4 actions (deployment recovery)
- `quarantine_pod`: 3 actions (security response)
- `drain_node`: 3 actions (maintenance operations)
- `notify_only`: 2 actions (permission-constrained scenarios)

## Performance vs Accuracy Analysis

### Performance Advantages ⚡
- **Total Runtime**: 107.34s vs 268.7s (8B baseline) = **2.5x faster**
- **Average Response Time**: 2.19s vs 4.78s (8B) = **2.2x faster**
- **Max Response Time**: 3.36s vs 7.87s (8B) = **2.3x faster**
- **Model Size**: 1.5GB vs 5.0GB = **3.3x smaller**

### Accuracy Assessment ⚠️
- **Success Rate**: 85.7% vs 100% (8B) = **14.3% degradation**
- **Security Handling**: Mixed - good for data exfiltration, poor for privilege escalation
- **Complex Scenarios**: Struggles with diagnostic collection and RBAC handling
- **Basic Operations**: Excellent performance on standard alerts

## Quality Assessment vs Other Models

| Metric | Granite 8B | Granite 3.3 2B | Granite 2B | Gemma 2B |
|--------|------------|-----------------|------------|-----------|
| **Success Rate** | 100% | **85.7%** | 94.4% | 85.7% |
| **Speed vs 8B** | 1.0x | **2.2x** | 2.5x | 2.0x |
| **Model Size** | 5.0GB | **1.5GB** | 1.6GB | 1.7GB |
| **Security Handling** | ✅ Perfect | ⚠️ Mixed | ✅ Perfect | ⚠️ Mixed |
| **Action Diversity** | ✅ Complete | ✅ Complete | ✅ Complete | ✅ Good |
| **Complex Scenarios** | ✅ Excellent | ⚠️ Basic | ✅ Excellent | ⚠️ Basic |

## Model Characteristics

### Strengths ⭐
- ✅ **Fast Response Time**: 2.2x faster than 8B baseline
- ✅ **Resource Efficient**: 3.3x smaller than 8B model
- ✅ **Good Action Variety**: Uses most available action types appropriately
- ✅ **Basic Operations**: Strong performance on standard scaling/recovery scenarios
- ✅ **End-to-End Execution**: Excellent integration with Kubernetes workflows
- ✅ **Storage Intelligence**: Perfect handling of PVC expansion scenarios

### Weaknesses ❌
- ⚠️ **Security Gaps**: Inconsistent security threat response (privilege escalation)
- ⚠️ **Diagnostic Limitations**: Prefers quick fixes over investigation
- ⚠️ **RBAC Awareness**: Poor handling of permission-constrained scenarios
- ⚠️ **Network Logic**: Conflates connectivity with capacity issues

## Recommendations

### When to Use Granite 3.3 2B ⚠️ **Conditional Production**

#### Suitable Scenarios
- **Basic scaling and resource management** - Excellent performance
- **Storage management** - Perfect PVC expansion handling
- **Standard deployment operations** - Reliable rollback and scaling
- **Non-security-critical environments** - Acceptable for basic operations

#### Avoid For
- **Security-sensitive environments** - Mixed security response quality
- **Complex troubleshooting scenarios** - Prefers quick fixes over investigation
- **RBAC-enforced clusters** - Poor permission constraint handling
- **Mission-critical systems** - 14.3% accuracy degradation vs baseline

### Performance Positioning

The **Granite 3.3 2B model offers a reasonable balance** for standard operations:

1. **vs Granite 8B**: 2.2x faster, 3.3x smaller, but 14.3% accuracy loss
2. **vs Granite 2B**: Similar speed, smaller size, but lower accuracy (85.7% vs 94.4%)
3. **vs Gemma 2B**: Similar accuracy (85.7%), slightly faster, smaller size

## Deployment Recommendation

### **Conditional Recommendation: Limited Production Use** ⚠️

**Use Cases:**
- High-volume, non-critical alert processing
- Development/staging environment operations
- Basic scaling and resource management scenarios
- Storage management operations

**Avoid For:**
- Security incident response
- Complex troubleshooting scenarios
- RBAC-enforced production clusters
- Mission-critical application management

### Multi-Modal Routing Potential

**Good candidate for routing layer:**
- **Route basic operations** (scaling, storage) to Granite 3.3 2B for speed
- **Escalate security/complex scenarios** to Granite 8B for accuracy
- **Use for initial triage** before routing to specialized models

## Conclusion

The **granite3.3:2b model represents a reasonable compromise** for speed-focused environments willing to accept accuracy trade-offs:

- ⚡ **Good Performance**: 2.2x faster than 8B with reasonable accuracy (85.7%)
- 🔧 **Basic Operations Excellence**: Strong performance on standard K8s operations
- ⚠️ **Security & Complexity Gaps**: Insufficient for security-critical or complex scenarios
- 💾 **Resource Efficient**: Smallest model tested (1.5GB)

**Best suited for multi-modal routing systems** where basic operations can be handled quickly by this model, with complex/security scenarios escalated to more capable models.

---

*Generated by prometheus-alerts-slm integration test suite*