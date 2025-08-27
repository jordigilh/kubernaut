# Granite 3.1 Model Performance Comparison

**Test Date**: August 26, 2025  
**Test Suite**: prometheus-alerts-slm Integration Tests  
**Environment**: Local Ollama deployment

## Executive Summary

This document compares the performance and accuracy of two IBM Granite 3.1 models for Kubernetes alert analysis and automated remediation:

- **granite3.1-dense:8b** - Larger, more accurate model
- **granite3.1-moe:1b** - Smaller, faster model with MoE architecture

## Key Findings

| Metric | Dense 8B | MoE 1B | Difference |
|--------|----------|---------|------------|
| **Success Rate** | 100% (18/18) | 77.8% (14/18) | -22.2% |
| **Total Runtime** | 100.14s | 18.62s | **5.4x faster** |
| **Avg Response Time** | 4.78s | 0.85s | **5.6x faster** |
| **Max Response Time** | 7.87s | 1.34s | **5.9x faster** |
| **Model Size** | 5.0GB | 1.4GB | **3.6x smaller** |

## Performance Analysis

### Speed Comparison by Test Category

| Test Category | Dense 8B | MoE 1B | Speedup |
|---------------|----------|---------|---------|
| **Model Availability** | 3.59s | 0.52s | **6.9x** |
| **Alert Analysis** | 68.32s | 12.43s | **5.5x** |
| **Resource Exhaustion** | 14.12s | 2.29s | **6.2x** |
| **Workflow Resilience** | 13.58s | 3.36s | **4.0x** |

### Individual Test Performance

| Test Scenario | Dense 8B | MoE 1B | Speedup | Accuracy |
|---------------|----------|---------|---------|----------|
| DeploymentFailureRollback | 4.56s | 1.03s | 4.4x | ✅ Both correct |
| StorageSpaceExhaustion | 5.95s | 1.15s | 5.2x | ✅ Both correct |
| NodeMaintenanceRequired | 5.41s | 1.08s | 5.0x | ❌ MoE incorrect |
| SecurityThreatDetected | 6.07s | 1.22s | 5.0x | ❌ MoE incorrect |
| ComplexTroubleshooting | 6.10s | 1.03s | 5.9x | ❌ MoE incorrect |
| HighMemoryUsage | 6.75s | 0.94s | 7.2x | ✅ Both correct |
| PodCrashLooping | 6.65s | 1.00s | 6.7x | ✅ Both correct |
| CPUThrottling | 6.08s | 1.19s | 5.1x | ✅ Both correct |
| NetworkConnectivityIssue | 5.89s | 1.09s | 5.4x | ❌ MoE incorrect |
| LowSeverityDiskSpace | 4.92s | 1.04s | 4.7x | ❌ MoE incorrect |

## Accuracy Analysis

### Action Distribution Comparison

| Action Type | Dense 8B | MoE 1B | Notes |
|-------------|----------|---------|-------|
| `scale_deployment` | 5 | 10 | MoE overuses scaling |
| `restart_pod` | 4 | 0 | MoE missing restart logic |
| `expand_pvc` | 3 | 3 | Both handle storage well |
| `rollback_deployment` | 1 | 1 | Both understand rollbacks |
| `drain_node` | 1 | 0 | MoE missing maintenance |
| `quarantine_pod` | 1 | 0 | MoE missing security |

### Failed Scenarios Analysis (MoE 1B)

#### 1. Security Threat Detection
- **Expected**: `quarantine_pod` (isolate compromised pod)
- **Got**: `scale_deployment` (spread potential threat)
- **Risk**: **Critical security vulnerability** - could propagate malicious activity

#### 2. Node Maintenance Required  
- **Expected**: `drain_node` (safe pod eviction)
- **Got**: `scale_deployment` (ignore maintenance needs)
- **Risk**: **Operational failure** - node could fail during maintenance

#### 3. Complex Troubleshooting
- **Expected**: `collect_diagnostics` (investigate root cause)
- **Got**: `scale_deployment` (mask the problem)
- **Risk**: **Problem escalation** - underlying issues remain unresolved

#### 4. Network Connectivity Issues
- **Expected**: `restart_pod` (reset network state)
- **Got**: `scale_deployment` (won't fix connectivity)
- **Risk**: **Ineffective remediation** - problem persists across all replicas

#### 5. Low Severity Disk Space
- **Expected**: `notify_only` or `expand_pvc` (appropriate response)
- **Got**: `scale_deployment` (creates more disk pressure)
- **Risk**: **Problem amplification** - makes storage situation worse

## Resource Utilization

### Model Characteristics

| Aspect | Dense 8B | MoE 1B |
|--------|----------|---------|
| **Architecture** | Dense transformer | Mixture of Experts |
| **Parameters** | ~8 billion | ~1 billion active |
| **Memory Usage** | 5.0GB | 1.4GB |
| **Inference Speed** | Slower, thorough | Faster, specialized |
| **Context Window** | Larger effective | Smaller effective |

### Token Usage Analysis

Both models show similar token consumption patterns:
- **Input tokens**: 517-624 tokens per request
- **Output tokens**: 78-148 tokens per response
- **Total tokens**: 599-766 tokens per interaction

The speed difference comes from model architecture, not token efficiency.

## Use Case Recommendations

### When to Use Dense 8B ✅

#### Production Environments
- **Security-sensitive clusters** - Requires accurate threat detection
- **Critical infrastructure** - Cannot afford incorrect actions
- **Complex operational scenarios** - Needs nuanced decision making
- **Compliance requirements** - Requires audit-ready decisions

#### Specific Scenarios
- Security incident response
- Node maintenance operations
- Complex troubleshooting workflows
- Multi-step remediation procedures
- Scenarios requiring specialized actions (drain, quarantine, diagnostics)

### When to Use MoE 1B ⚡

#### Performance-Critical Environments
- **High-frequency alerting** - Sub-second response requirements
- **Resource-constrained clusters** - Limited memory/CPU available
- **Development/testing** - Fast feedback loops needed
- **Simple operational scenarios** - Basic scaling and resource management

#### Specific Scenarios
- High memory/CPU usage alerts (scaling decisions)
- Pod crash loops (scaling/restart decisions)
- Deployment replica mismatches
- Basic storage expansion
- Initial alert triage

### Hybrid Architecture Recommendation 🏗️

For optimal production deployment, consider a two-tier approach:

```
Alert → Fast Triage (MoE 1B) → Complex Analysis (Dense 8B) → Action
         └─ Simple scenarios      └─ Complex/Security scenarios
         └─ <2s response          └─ High accuracy required
```

#### Implementation Strategy
1. **Primary Filter**: MoE 1B for initial analysis (0.8s avg)
2. **Confidence Threshold**: If confidence < 0.9, escalate to Dense 8B
3. **Scenario Detection**: Route security/maintenance alerts to Dense 8B
4. **Action Validation**: Cross-check critical actions with Dense 8B

## Technical Implications

### Memory and Compute Requirements

| Deployment Type | Model | Memory | CPU | Response Time |
|----------------|-------|---------|-----|---------------|
| **High Performance** | Dense 8B | 8GB+ | 4+ cores | 4-8s |
| **Fast Response** | MoE 1B | 2GB+ | 2+ cores | 0.8-1.3s |
| **Hybrid** | Both | 10GB+ | 6+ cores | 0.8-8s |

### Reliability Considerations

#### Dense 8B Strengths
- ✅ **High Accuracy**: 100% success rate
- ✅ **Contextual Understanding**: Handles edge cases well
- ✅ **Security Awareness**: Proper threat response
- ✅ **Operational Sophistication**: Understands maintenance workflows

#### MoE 1B Limitations
- ❌ **Action Bias**: Overreliance on scaling
- ❌ **Security Gaps**: Missing critical security responses
- ❌ **Context Sensitivity**: Struggles with specialized scenarios
- ❌ **Operational Scope**: Limited understanding of complex procedures

## Conclusion and Recommendations

### For Production Kubernetes Environments

1. **Primary Recommendation**: Use **Dense 8B** for production workloads
   - **Reasoning**: 100% accuracy is critical for automated remediation
   - **Risk Mitigation**: Prevents security and operational failures

2. **Performance Optimization**: Implement intelligent routing
   - Route simple scenarios to MoE 1B for speed
   - Route complex/security scenarios to Dense 8B for accuracy
   - Use confidence scoring for model selection

3. **Development/Testing**: Use **MoE 1B** for non-critical environments
   - **Reasoning**: Speed enables rapid development cycles
   - **Risk Acceptance**: Lower accuracy acceptable in test scenarios

### Future Optimization Opportunities

1. **Model Fine-tuning**: Train MoE 1B with Kubernetes-specific scenarios
2. **Ensemble Methods**: Combine both models for optimal decision making
3. **Prompt Engineering**: Improve MoE 1B accuracy through better prompts
4. **Confidence Calibration**: Better threshold tuning for model selection

The choice between models represents a classic speed vs. accuracy tradeoff. For production Kubernetes environments where incorrect actions can cause outages or security breaches, the Dense 8B model's superior accuracy outweighs the MoE 1B model's performance advantages.

---

*Analysis based on prometheus-alerts-slm integration test results*