# Integration Test Results - Qwen2 1.5B

**Model**: `qwen2:1.5b` (Alibaba Cloud, 1.5B parameters)  
**Test Date**: August 27, 2025  
**Total Test Duration**: 70.84 seconds (~1.2 minutes)  
**Comparison Baseline**: Granite 3.1 Dense 8B (100% accuracy, 4.78s avg)

## 📊 **Performance Summary**

### Overall Results
- **Total Tests**: 49 tests executed
- **Passed Tests**: 35 tests 
- **Failed Tests**: 14 tests
- **Accuracy**: 71.4% (35/49 passed)
- **Average Response Time**: ~1.45 seconds (70.84s / 49 tests)

### Performance Grade
- **Accuracy**: C- (71.4% - below minimum threshold)
- **Speed**: A+ (1.45s - 3.3x faster than baseline!)
- **Overall**: C+ (Outstanding speed, but insufficient accuracy)

## ⚠️ **Failed Tests Analysis**

### Test Failures (14 total):
1. **TestAlertAnalysisScenarios/StorageSpaceExhaustion**: Used `increase_resources` instead of `expand_pvc`
2. **TestAlertAnalysisScenarios/LowSeverityDiskSpace**: Action selection error
3. **TestAlertAnalysisScenarios/ComplexTroubleshooting**: Complex reasoning failure
4. **TestApplicationLifecycleScenarios**: Multiple application lifecycle failures
5. **TestCascadingFailureResponse**: Both cascading failure scenarios failed
6. **TestConcurrentEndToEndExecution**: Concurrent execution handling
7. **TestEndToEndActionExecution/NodeMaintenanceRequired_EndToEnd**: Node management
8. **TestEndToEndIntegration/NodeMaintenanceRequired_EndToEnd**: Node integration
9. **TestSecurityIncidentHandling**: Both security incidents failed

### Root Cause Analysis:
- **Limited Reasoning Capacity**: 1.5B parameters insufficient for complex scenarios
- **Action Selection Confusion**: Consistent misunderstanding of action categories
- **Context Processing**: Struggles with nuanced alert context
- **Security Handling**: Poor performance on security-specific scenarios
- **Concurrency Issues**: Problems with concurrent request handling

## 🎯 **Successful Scenarios**

### Strong Performance Areas:
- **Exceptional Speed**: 1.45s average (3.3x faster than baseline!)
- **Basic Operations**: Good performance on simple alert scenarios
- **Memory Efficiency**: Only 934MB model size
- **Standard Workflows**: Decent performance on routine operations

### Sample Successful Results:
```
✅ DeploymentFailureRollback: 1.95s (action=rollback_deployment, confidence=0.92)
✅ HighMemoryUsage: 1.14s (action=scale_deployment, confidence=0.92)
✅ SecurityThreatDetected: 0.98s (action=quarantine_pod, confidence=0.92)
✅ Basic execution workflows: <1.5s average
```

## ⚡ **Speed Analysis - Record Performance**

### Response Time Distribution:
- **Ultra-Fast** (<1.5s): ~85% of tests
- **Fast** (1.5-3s): ~15% of tests
- **Moderate** (>3s): <5% of tests
- **Average**: 1.45s (fastest model tested!)

### Speed Comparison:
- **Qwen2 1.5B**: 1.45s average  
- **Gemma 2B**: 2.40s average
- **Granite 8B**: 4.78s average
- **Phi-3 Mini**: 5.48s average
- **Performance**: **70% faster than baseline** ⚡⚡

## 🔄 **Action Distribution Analysis**

### Actions Successfully Used:
- `rollback_deployment`: Good execution
- `scale_deployment`: Frequent use (sometimes overused)
- `quarantine_pod`: Basic security response
- `notify_only`: Appropriate for simple cases

### Action Selection Issues:
- **Limited Action Vocabulary**: Uses narrow set of actions
- **Storage Confusion**: Confuses storage actions with resource actions
- **Security Specificity**: Misses specialized security actions
- **Complex Scenarios**: Fails on multi-step reasoning

## 🧠 **Model Capability Assessment**

### Strengths:
- **Outstanding Speed**: 1.45s average response time (fastest tested)
- **Memory Efficiency**: Only 934MB model size (smallest)
- **Basic Understanding**: Grasps fundamental Kubernetes concepts
- **Confident Simple Decisions**: Good confidence on basic scenarios

### Weaknesses:
- **Insufficient Accuracy**: 71.4% below 85% production threshold
- **Limited Reasoning**: Struggles with complex multi-step analysis
- **Action Variety**: Uses narrow range of available actions
- **Context Understanding**: Poor interpretation of nuanced scenarios
- **Consistency**: Highly variable performance across test types

## 📋 **Detailed Test Results**

### Alert Analysis Scenarios (6/10 passed):
- ✅ DeploymentFailureRollback: 1.95s
- ❌ StorageSpaceExhaustion: Wrong action category
- ✅ NodeMaintenanceRequired: 1.13s  
- ✅ HighMemoryUsage: 1.01s
- ❌ LowSeverityDiskSpace: Action selection error
- ✅ SecurityThreatDetected: 1.02s
- ❌ ComplexTroubleshooting: Complex reasoning failed
- ✅ DatabasePerformanceDegradation: 1.08s
- ✅ PodCrashLooping: 1.10s
- ✅ NetworkConnectivityIssue: 1.16s

### Security Scenarios (0/2 passed):
- ❌ SecurityIncident_privilege_escalation: Security action failure
- ❌ SecurityIncident_data_exfiltration: Security action failure

### End-to-End Execution (4/5 passed):
- ✅ DeploymentFailureRollback_EndToEnd: 1.33s
- ✅ StorageSpaceExhaustion_EndToEnd: 1.10s
- ❌ NodeMaintenanceRequired_EndToEnd: Node management failure
- ✅ SecurityThreatDetected_EndToEnd: 0.98s
- ✅ HighMemoryUsage_EndToEnd: 1.14s

## 🎯 **Production Readiness Assessment**

### For Production Use:
- **❌ Accuracy Insufficient**: 71.4% << 85% minimum requirement
- **❌ Security Gaps**: 0% success rate on security incidents
- **❌ Complex Scenario Failures**: Poor cascading failure handling
- **❌ Action Variety**: Limited use of available actions
- **✅ Speed Excellence**: Outstanding 1.45s response time

### Risk Assessment:
- **High Risk**: 71.4% accuracy too low for critical infrastructure
- **Security Concerns**: Complete failure on security incident handling
- **Reliability Issues**: Inconsistent performance across scenarios
- **Production Unsuitability**: Cannot meet reliability requirements

### Recommended Use Cases:
- **Development Only**: Fast feedback for development environments
- **Simple Demos**: Basic functionality demonstrations
- **Speed Benchmarks**: Reference for response time optimization
- **Resource Testing**: Minimal resource consumption testing

## 📊 **Comparison Matrix**

| Metric | Qwen2 1.5B | Gemma 2B | Granite 8B | Phi-3 Mini | Delta vs Baseline |
|--------|------------|----------|------------|------------|-------------------|
| **Accuracy** | 71.4% | 85.7% | 100% | 79.6% | -28.6% |
| **Avg Response Time** | 1.45s | 2.40s | 4.78s | 5.48s | **-70%** ⚡⚡ |
| **Model Size** | 934MB | 1.7GB | 5.0GB | 2.2GB | -81% |
| **Failed Tests** | 14/49 | 7/49 | 0/18 | 10/49 | +14 failures |
| **Production Ready** | ❌ No | ⚠️ Conditional | ✅ Yes | ❌ No | Not suitable |
| **Security Success** | 0% | 50% | 100% | 100% | -100% |

## 🎯 **Recommendation**

**❌ NOT RECOMMENDED for production deployment**

**Reasons:**
1. **Accuracy Far Below Threshold**: 71.4% << 85% minimum requirement
2. **Security Failure**: 0% success on security incident handling
3. **Complex Reasoning Gaps**: Multiple failures on advanced scenarios
4. **Inconsistent Reliability**: Too many failed test categories
5. **Action Limitations**: Narrow action vocabulary unsuitable for production

**Strengths to Note:**
1. **Record Speed**: 1.45s average (3.3x faster than baseline)
2. **Resource Efficiency**: Smallest model size (934MB)
3. **Basic Competency**: Good performance on simple scenarios

**Better suited for:**
- **Development environments** where speed > accuracy
- **Resource-constrained testing** scenarios
- **Speed optimization** research and benchmarking
- **Educational purposes** for understanding model trade-offs

**Key Insight**: Demonstrates classic accuracy vs. speed trade-off - 1.5B parameters insufficient for production Kubernetes operations despite excellent speed.

---

*Qwen2 1.5B showcases the limits of small models: exceptional speed but insufficient reasoning capacity for production reliability.*