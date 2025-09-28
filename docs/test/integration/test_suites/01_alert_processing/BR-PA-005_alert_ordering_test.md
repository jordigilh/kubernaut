# BR-PA-005: Alert Processing Order Test

**Business Requirement**: BR-PA-005 - MUST maintain alert processing order for the same alert source
**Test Strategy**: Functional Integration Testing
**Priority**: Medium - Data consistency and workflow correctness
**Estimated Duration**: 45 minutes

---

## 🎯 **Business Requirement Context**

### **BR-PA-005 Definition**
From `docs/requirements/01_MAIN_APPLICATIONS.md`:
> **BR-PA-005**: MUST maintain alert processing order for the same alert source

### **Success Criteria**
- **Sequential processing** of alerts from the same source maintained
- **Timestamp ordering** preserved for chronological alert sequences
- **State consistency** maintained for alert lifecycle (firing → resolved)
- **No out-of-order processing** causing workflow inconsistencies
- **Proper handling** of alert updates and resolutions

### **Business Impact**
- **Workflow correctness**: Ensures alert lifecycle stages processed in proper sequence
- **Data consistency**: Prevents state corruption from out-of-order updates
- **Operational reliability**: Maintains logical alert progression for decision making
- **Audit integrity**: Preserves chronological order for compliance and troubleshooting

---

## ⚙️ **Test Environment Setup**

### **Prerequisites**
```bash
# Verify environment is ready
cd ~/kubernaut-integration-test
./scripts/environment_health_check.sh

# Verify Kubernaut kubernaut is running
curl -f http://localhost:8080/health || echo "Prometheus Alerts SLM not ready"

# Set test variables
export WEBHOOK_URL="http://localhost:8080/webhook/prometheus"
export TEST_SESSION="alert_ordering_test_$(date +%Y%m%d_%H%M%S)"
mkdir -p "results/$TEST_SESSION"
```

### **Test Data Preparation**
```bash
# Create alert ordering test data generator
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/ordering_test_data_generator.py" "results/$TEST_SESSION/ordering_test_data_generator.py"
chmod +x "results/$TEST_SESSION/ordering_test_data_generator.py" 

# Generate test data
cd "results/$TEST_SESSION"
python3 ordering_test_data_generator.py
cd ../..
```

---

## 🧪 **Functional Integration Tests**

### **Test 1: Sequential Source Processing**

**Objective**: Verify that alerts from the same source are processed in order

```bash
echo "=== Test 1: Sequential Source Processing ==="
echo "Objective: Validate order preservation for alerts from same source"

# Create sequential processing test script
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/sequential_processing_test.py" "results/$TEST_SESSION/sequential_processing_test.py"
chmod +x "results/$TEST_SESSION/sequential_processing_test.py" 

# Run sequential processing test
python3 "results/$TEST_SESSION/sequential_processing_test.py" "$WEBHOOK_URL" "$TEST_SESSION"
```

### **Test 2: Rapid Sequence Processing**

**Objective**: Verify order preservation under rapid alert delivery

```bash
echo "=== Test 2: Rapid Sequence Processing ==="
echo "Objective: Test ordering under rapid-fire alert delivery"

# Create rapid sequence test script
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/rapid_sequence_test.py" "results/$TEST_SESSION/rapid_sequence_test.py"
chmod +x "results/$TEST_SESSION/rapid_sequence_test.py" 

# Run rapid sequence test
python3 "results/$TEST_SESSION/rapid_sequence_test.py" "$WEBHOOK_URL" "$TEST_SESSION"
```

### **Test 3: Alert Lifecycle Ordering**

**Objective**: Verify proper ordering of alert lifecycle stages

```bash
echo "=== Test 3: Alert Lifecycle Ordering ==="
echo "Objective: Test proper sequencing of alert lifecycle stages"

# Create lifecycle ordering test script
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/lifecycle_ordering_test.py" "results/$TEST_SESSION/lifecycle_ordering_test.py"
chmod +x "results/$TEST_SESSION/lifecycle_ordering_test.py" 

# Run lifecycle ordering test
python3 "results/$TEST_SESSION/lifecycle_ordering_test.py" "$WEBHOOK_URL" "$TEST_SESSION"
```

---

## 📊 **Test Results Analysis**

### **Comprehensive Alert Ordering Report**

```bash
echo "=== BR-PA-005 Alert Ordering Test - Final Analysis ==="

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
sequential_results = load_results('sequential_processing_results.json')
rapid_results = load_results('rapid_sequence_results.json')
lifecycle_results = load_results('lifecycle_ordering_results.json')

print("=== BR-PA-005 Alert Ordering - Comprehensive Results ===")

# Sequential processing analysis
if sequential_results:
    seq_compliance = sequential_results['br_pa_005_compliance']
    print(f"\n1. Sequential Source Processing:")
    print(f"   Total Sources: {sequential_results['total_sources']}")
    print(f"   Ordering Compliance: {seq_compliance['compliance_rate']:.1f}%")
    print(f"   Result: {'✅ PASS' if seq_compliance['pass'] else '❌ FAIL'}")

    if seq_compliance['sources_with_violations']:
        print(f"   Sources with violations: {seq_compliance['sources_with_violations']}")

# Rapid sequence analysis
if rapid_results:
    rapid_compliance = rapid_results['br_pa_005_rapid_compliance']
    print(f"\n2. Rapid Sequence Processing:")
    print(f"   Total Rapid Alerts: {rapid_results['total_rapid_alerts']}")
    print(f"   Correctly Ordered: {rapid_compliance['correctly_ordered']}")
    print(f"   Violations: {rapid_results['violation_count']}")
    print(f"   Result: {'✅ PASS' if rapid_compliance['pass'] else '❌ FAIL'}")
    print(f"   Rapid Delivery Handling: {rapid_compliance['rapid_delivery_handling']}")

# Lifecycle analysis
if lifecycle_results:
    lifecycle_compliance = lifecycle_results['br_pa_005_lifecycle_compliance']
    print(f"\n3. Alert Lifecycle Ordering:")
    print(f"   Valid Progression: {lifecycle_compliance['valid_progression']}")
    print(f"   Correct Sequence: {lifecycle_compliance['correct_sequence']}")
    print(f"   Lifecycle Complete: {lifecycle_compliance['lifecycle_complete']}")
    print(f"   Result: {'✅ PASS' if lifecycle_compliance['pass'] else '❌ FAIL'}")
    print(f"   Progression Quality: {lifecycle_compliance['progression_quality']}")

# Overall BR-PA-005 Compliance
all_tests_pass = all([
    sequential_results and sequential_results['br_pa_005_compliance']['pass'],
    rapid_results and rapid_results['br_pa_005_rapid_compliance']['pass'],
    lifecycle_results and lifecycle_results['br_pa_005_lifecycle_compliance']['pass']
])

print(f"\n=== Overall BR-PA-005 Compliance ===")
print(f"Business Requirement: Maintain alert processing order for the same alert source")
print(f"Overall Result: {'✅ PASS' if all_tests_pass else '❌ FAIL'}")

if all_tests_pass:
    print("\n✅ System properly maintains alert processing order")
    print("✅ Sequential processing, rapid delivery, and lifecycle stages handled correctly")
    print("✅ Ready for production deployment from ordering perspective")
else:
    print("\n❌ System has alert ordering issues that must be resolved")
    print("❌ Not ready for production deployment")

# Generate recommendations
print(f"\n=== Recommendations ===")
if all_tests_pass:
    print("- Alert ordering is functioning correctly across all scenarios")
    print("- System demonstrates proper sequence preservation and lifecycle handling")
    print("- Continue with other integration tests")
else:
    print("- Review alert queuing and processing logic")
    print("- Implement proper sequence preservation mechanisms")
    print("- Test with different timing scenarios to ensure robustness")

EOF

echo ""
echo "🎯 BR-PA-005 Alert Ordering Test Complete!"
echo "📊 Results validate alert sequence preservation and lifecycle correctness"
echo "⏱️ System ordering behavior under various timing scenarios assessed"
```

---

## 🎉 **Expected Outcomes**

### **Success Indicators**
- ✅ **Sequential processing maintained**: Alerts from same source processed in order
- ✅ **Rapid delivery handled correctly**: Order preserved even under fast alert delivery
- ✅ **Lifecycle progression valid**: Alert stages (firing → resolved) processed properly
- ✅ **No ordering violations**: All sequences maintain expected chronological order
- ✅ **High success rate (≥95%)**: Reliable order preservation across scenarios

### **Business Value**
- **Workflow Correctness**: Ensures alert lifecycle stages processed in proper sequence
- **Data Consistency**: Prevents state corruption from out-of-order updates
- **Operational Reliability**: Maintains logical alert progression for decision making
- **Audit Integrity**: Preserves chronological order for compliance and troubleshooting

### **Integration Confidence**
- **Queue Management**: Alert queuing and processing order working correctly
- **Source Isolation**: Different sources processed independently without cross-contamination
- **Timing Robustness**: Order maintained under various delivery timing scenarios
- **Lifecycle Management**: Proper handling of alert state transitions

---

**This functional integration test validates the alert processing order business requirement through comprehensive testing of sequential processing, rapid delivery scenarios, and lifecycle management, ensuring reliable chronological alert handling for production deployment.**
