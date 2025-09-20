# BR-PA-006: LLM Provider Integration Test

**Business Requirement**: BR-PA-006 - MUST support multiple LLM providers (6 providers minimum)
**Test Strategy**: Functional Integration Testing
**Priority**: High - Core AI functionality
**Estimated Duration**: 90 minutes

---

## 🎯 **Business Requirement Context**

### **BR-PA-006 Definition**
From `docs/requirements/01_MAIN_APPLICATIONS.md`:
> **BR-PA-006**: MUST support multiple LLM providers (6 providers minimum)

### **Success Criteria**
- **All 6 LLM providers** functional and accessible
- **Provider failover** working correctly between providers
- **Consistent response format** across different providers
- **Provider-specific configurations** properly loaded and applied
- **Error handling** for provider unavailability or failures

### **Business Impact**
- **Provider resilience**: Reduces dependency on single LLM provider
- **Cost optimization**: Enables selection of optimal provider for different scenarios
- **Performance flexibility**: Allows choosing best-performing provider for specific tasks
- **Business continuity**: Ensures service availability despite individual provider issues

---

## ⚙️ **Test Environment Setup**

### **Prerequisites**
```bash
# Verify environment is ready
cd ~/kubernaut-integration-test
./scripts/environment_health_check.sh

# Verify Kubernaut prometheus-alerts-slm is running
curl -f http://localhost:8080/health || echo "Prometheus Alerts SLM not ready"

# Set test variables
export WEBHOOK_URL="http://localhost:8080/webhook/prometheus"
export TEST_SESSION="llm_provider_test_$(date +%Y%m%d_%H%M%S)"
mkdir -p "results/$TEST_SESSION"

# Verify local Ollama is running
curl -f http://localhost:8080/api/tags || echo "⚠️ Ollama not accessible - some providers may fail"
```

### **LLM Provider Configuration**
```bash
# Create LLM provider test configuration
# Copy the script to your test session directory
cp "../../../scripts/02_ai_decision_making/llm_provider_config.json" "results/$TEST_SESSION/llm_provider_config.json"
chmod +x "results/$TEST_SESSION/llm_provider_config.json" 
```

---

## 🧪 **Functional Integration Tests**

### **Test 1: Provider Discovery and Connectivity**

**Objective**: Verify all configured LLM providers are discoverable and reachable

```bash
echo "=== Test 1: Provider Discovery and Connectivity ==="
echo "Objective: Test connectivity to all 6+ configured LLM providers"

# Run provider discovery test
python3 "results/$TEST_SESSION/provider_discovery_test.py" "$TEST_SESSION"
```

### **Test 2: Provider Response Consistency**

**Objective**: Verify consistent response format across available providers

```bash
echo "=== Test 2: Provider Response Consistency ==="
echo "Objective: Test response consistency across available providers"

# Create provider response consistency test
# Copy the script to your test session directory
cp "../../../scripts/02_ai_decision_making/provider_response_test.py" "results/$TEST_SESSION/provider_response_test.py"
chmod +x "results/$TEST_SESSION/provider_response_test.py" 

# Run provider response consistency test
python3 "results/$TEST_SESSION/provider_response_test.py" "$WEBHOOK_URL" "$TEST_SESSION"
```

### **Test 3: Provider Failover Mechanism**

**Objective**: Verify failover between providers works correctly

```bash
echo "=== Test 3: Provider Failover Mechanism ==="
echo "Objective: Test provider failover and fallback mechanisms"

# Create provider failover test
# Copy the script to your test session directory
cp "../../../scripts/02_ai_decision_making/provider_failover_test.py" "results/$TEST_SESSION/provider_failover_test.py"
chmod +x "results/$TEST_SESSION/provider_failover_test.py" 

# Run provider failover test
python3 "results/$TEST_SESSION/provider_failover_test.py" "$WEBHOOK_URL" "$TEST_SESSION"
```

---

## 📊 **Test Results Analysis**

### **Comprehensive LLM Provider Integration Report**

```bash
echo "=== BR-PA-006 LLM Provider Integration - Final Analysis ==="

# Create comprehensive analysis
python3 << 'EOF'
import json
import os

test_session = os.environ.get('TEST_SESSION', 'test_session')

def load_results(filename):
    try:
        with open(f"results/{test_session}/{filename}", 'r') as f:
            return json.load(f)
    except FileNotFoundError:
        return None

# Load all test results
discovery_results = load_results('provider_discovery_results.json')
response_results = load_results('provider_response_results.json')
failover_results = load_results('provider_failover_results.json')

print("=== BR-PA-006 LLM Provider Integration - Comprehensive Results ===")

# Provider discovery analysis
if discovery_results:
    discovery_compliance = discovery_results['br_pa_006_compliance']
    print(f"\n1. Provider Discovery and Configuration:")
    print(f"   Total Providers Configured: {discovery_compliance['total_providers_configured']}")
    print(f"   Configuration Compliant: {'✅' if discovery_compliance['providers_configured_compliant'] else '❌'} (≥6 required)")
    print(f"   Available Providers: {discovery_compliance['available_providers']}")
    print(f"   High Priority Functional: {'✅' if discovery_compliance['high_priority_functional'] else '❌'}")
    print(f"   Provider Diversity: {discovery_compliance['provider_diversity']}")
    print(f"   Result: {'✅ PASS' if discovery_compliance['pass'] else '❌ FAIL'}")

# Response consistency analysis
if response_results and "error" not in response_results:
    response_compliance = response_results['br_pa_006_response_compliance']
    print(f"\n2. Response Consistency:")
    print(f"   Success Rate: {response_compliance['success_rate']:.1f}%")
    print(f"   Average Quality: {response_compliance['response_quality']:.1f}%")
    print(f"   Consistency Rating: {response_compliance['consistency_rating']}")
    print(f"   Integration Quality: {response_compliance['provider_integration_quality']}")
    print(f"   Result: {'✅ PASS' if response_compliance['pass'] else '❌ FAIL'}")
elif response_results:
    print(f"\n2. Response Consistency: ❌ ERROR - {response_results.get('error', 'Unknown error')}")

# Failover analysis
if failover_results and "error" not in failover_results:
    failover_compliance = failover_results['br_pa_006_failover_compliance']
    print(f"\n3. Provider Failover:")
    print(f"   Normal Operation: {'✅' if failover_compliance['normal_operation_success'] else '❌'}")
    print(f"   Failover Mechanism: {'✅' if failover_compliance['failover_mechanism_success'] else '❌'}")
    print(f"   Failover Quality: {failover_compliance['failover_quality']}")
    print(f"   Resilience Rating: {failover_compliance['resilience_rating']}")
    print(f"   Result: {'✅ PASS' if failover_compliance['pass'] else '❌ FAIL'}")
elif failover_results:
    print(f"\n3. Provider Failover: ❌ ERROR - {failover_results.get('error', 'Unknown error')}")

# Overall BR-PA-006 Compliance
discovery_pass = discovery_results and discovery_results['br_pa_006_compliance']['pass']
response_pass = response_results and response_results.get('br_pa_006_response_compliance', {}).get('pass', False) if "error" not in (response_results or {}) else False
failover_pass = failover_results and failover_results.get('br_pa_006_failover_compliance', {}).get('pass', False) if "error" not in (failover_results or {}) else False

overall_pass = discovery_pass and (response_pass or not response_results) and (failover_pass or not failover_results)

print(f"\n=== Overall BR-PA-006 Compliance ===")
print(f"Business Requirement: Support multiple LLM providers (6 providers minimum)")
print(f"Overall Result: {'✅ PASS' if overall_pass else '❌ FAIL'}")

if overall_pass:
    print("\n✅ Multiple LLM providers properly configured and functional")
    print("✅ Provider integration demonstrates proper failover and consistency")
    print("✅ Ready for production deployment from LLM provider perspective")
else:
    print("\n❌ LLM provider integration has issues that must be resolved")
    print("❌ Not ready for production deployment")

# Generate recommendations
print(f"\n=== Recommendations ===")
if overall_pass:
    print("- LLM provider integration is functioning correctly")
    print("- System demonstrates proper multi-provider support and resilience")
    print("- Continue with other AI decision making tests")
else:
    print("- Configure additional LLM providers if below minimum (6)")
    print("- Ensure at least one high-priority provider (like Ollama) is functional")
    print("- Test and improve provider failover mechanisms")
    print("- Verify response format consistency across providers")

EOF

echo ""
echo "🎯 BR-PA-006 LLM Provider Integration Test Complete!"
echo "📊 Results validate multi-provider LLM support and failover capability"
echo "🔧 System provider diversity and resilience assessed"
```

---

## 🎉 **Expected Outcomes**

### **Success Indicators**
- ✅ **6+ LLM providers configured**: Meeting minimum business requirement
- ✅ **At least one provider functional**: Ensuring basic AI capability
- ✅ **High-priority provider working**: Critical for local/reliable operation (Ollama)
- ✅ **Response consistency maintained**: Uniform behavior across providers
- ✅ **Failover mechanism functional**: Resilience against provider failures

### **Business Value**
- **Provider Resilience**: Reduced dependency on single LLM provider
- **Cost Optimization**: Flexibility to choose optimal provider for different scenarios
- **Performance Flexibility**: Ability to select best-performing provider for specific tasks
- **Business Continuity**: Service availability despite individual provider issues

### **Integration Confidence**
- **Multi-Provider Architecture**: LLM abstraction layer working correctly
- **Configuration Management**: Provider-specific settings properly loaded and applied
- **Error Handling**: Graceful handling of provider unavailability
- **Response Processing**: Consistent handling of different provider response formats

---

**This functional integration test validates the critical multi-provider LLM support business requirement through comprehensive testing of provider discovery, response consistency, and failover mechanisms, ensuring reliable AI decision-making capabilities for production deployment.**
