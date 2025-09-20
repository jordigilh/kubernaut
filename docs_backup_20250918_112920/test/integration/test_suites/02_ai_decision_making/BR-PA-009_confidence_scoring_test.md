# BR-PA-009: Confidence Scoring Test

**Business Requirement**: BR-PA-009 - MUST provide confidence scores (0-1 scale) for recommendations
**Test Strategy**: Functional Integration Testing
**Priority**: Medium - Decision quality assessment functionality
**Estimated Duration**: 45 minutes

---

## 🎯 **Business Requirement Context**

### **BR-PA-009 Definition**
From `docs/requirements/01_MAIN_APPLICATIONS.md`:
> **BR-PA-009**: MUST provide confidence scores (0-1 scale) for recommendations

### **Success Criteria**
- **Confidence scores provided** for all remediation recommendations
- **0-1 scale compliance** with scores between 0.0 and 1.0 inclusive
- **Score correlation** with recommendation quality and specificity
- **Consistent scoring** across similar alert scenarios
- **Score interpretation** available for operational decision-making

### **Business Impact**
- **Decision support**: Enables operators to prioritize recommendations based on confidence
- **Risk management**: Allows assessment of recommendation reliability before execution
- **Quality indication**: Provides immediate feedback on AI decision certainty
- **Operational guidance**: Helps determine when human intervention may be needed

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
export TEST_SESSION="confidence_scoring_test_$(date +%Y%m%d_%H%M%S)"
mkdir -p "results/$TEST_SESSION"
```

### **Test Scenarios Preparation**
```bash
# Create confidence scoring test scenarios
# Copy the script to your test session directory
cp "../../../scripts/02_ai_decision_making/confidence_scoring_scenarios.json" "results/$TEST_SESSION/confidence_scoring_scenarios.json"
chmod +x "results/$TEST_SESSION/confidence_scoring_scenarios.json" 
```

---

## 🧪 **Functional Integration Tests**

### **Test 1: Confidence Score Provision**

**Objective**: Verify that confidence scores are provided with recommendations

```bash
echo "=== Test 1: Confidence Score Provision ==="
echo "Objective: Test that confidence scores are provided for all recommendations"

# Create confidence score provision test
# Copy the script to your test session directory
cp "../../../scripts/02_ai_decision_making/confidence_provision_test.py" "results/$TEST_SESSION/confidence_provision_test.py"
chmod +x "results/$TEST_SESSION/confidence_provision_test.py" 

# Run confidence score provision test
python3 "results/$TEST_SESSION/confidence_provision_test.py" "$WEBHOOK_URL" "$TEST_SESSION"
```

### **Test 2: Confidence Score Consistency**

**Objective**: Verify consistency of confidence scores across similar scenarios

```bash
echo "=== Test 2: Confidence Score Consistency ==="
echo "Objective: Test consistency of confidence scoring across similar scenarios"

# Create confidence consistency test
# Copy the script to your test session directory
cp "../../../scripts/02_ai_decision_making/confidence_consistency_test.py" "results/$TEST_SESSION/confidence_consistency_test.py"
chmod +x "results/$TEST_SESSION/confidence_consistency_test.py" 

# Run confidence consistency test
python3 "results/$TEST_SESSION/confidence_consistency_test.py" "$WEBHOOK_URL" "$TEST_SESSION"
```

---

## 📊 **Test Results Analysis**

### **Comprehensive Confidence Scoring Report**

```bash
echo "=== BR-PA-009 Confidence Scoring - Final Analysis ==="

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
provision_results = load_results('confidence_provision_results.json')
consistency_results = load_results('confidence_consistency_results.json')

print("=== BR-PA-009 Confidence Scoring - Comprehensive Results ===")

# Confidence provision analysis
if provision_results and "error" not in provision_results:
    provision_compliance = provision_results['br_pa_009_compliance']
    print(f"\n1. Confidence Score Provision:")
    print(f"   Total Scenarios: {provision_results['total_scenarios_tested']}")
    print(f"   Successful Responses: {provision_results['successful_responses']}")
    print(f"   Provision Rate: {provision_compliance['provision_rate']:.1f}%")
    print(f"   Scale Compliance: {provision_compliance['scale_compliance_rate']:.1f}%")
    print(f"   Range Appropriateness: {provision_compliance['range_appropriateness']:.1f}%")
    print(f"   Confidence Capability: {provision_compliance['confidence_capability']}")
    print(f"   Result: {'✅ PASS' if provision_compliance['pass'] else '❌ FAIL'}")

    if 'score_statistics' in provision_results:
        stats = provision_results['score_statistics']
        print(f"   Score Statistics:")
        print(f"     Mean: {stats['mean_score']:.3f}")
        print(f"     Range: {stats['min_score']:.3f} - {stats['max_score']:.3f}")
        print(f"     Unique Scores: {stats['unique_scores']}")

elif provision_results:
    print(f"\n1. Confidence Score Provision: ❌ ERROR - {provision_results.get('error', 'Unknown error')}")

# Confidence consistency analysis
if consistency_results and "error" not in consistency_results:
    consistency_compliance = consistency_results['br_pa_009_consistency_compliance']
    print(f"\n2. Confidence Score Consistency:")
    print(f"   Scenario Groups Tested: {consistency_results['total_scenario_groups']}")
    print(f"   Valid Groups: {consistency_results['valid_groups_for_analysis']}")
    print(f"   Consistent Groups: {consistency_results['consistent_groups']}")
    print(f"   Consistency Rate: {consistency_compliance['consistency_rate']:.1f}%")
    print(f"   Consistency Quality: {consistency_compliance['consistency_quality']}")
    print(f"   Result: {'✅ PASS' if consistency_compliance['pass'] else '❌ FAIL'}")

    print(f"   Group Details:")
    for detail in consistency_results['group_consistency_details']:
        status = '✅' if detail['consistent'] else '❌'
        print(f"     {detail['group_name']}: {status} ({detail['consistency_rating']})")

elif consistency_results:
    print(f"\n2. Confidence Score Consistency: ❌ ERROR - {consistency_results.get('error', 'Unknown error')}")

# Overall BR-PA-009 Compliance
provision_pass = provision_results and provision_results.get('br_pa_009_compliance', {}).get('pass', False) if "error" not in (provision_results or {}) else False
consistency_pass = consistency_results and consistency_results.get('br_pa_009_consistency_compliance', {}).get('pass', False) if "error" not in (consistency_results or {}) else False

# For this requirement, provision is essential, consistency is desirable
overall_pass = provision_pass and (consistency_pass or not consistency_results)

print(f"\n=== Overall BR-PA-009 Compliance ===")
print(f"Business Requirement: Provide confidence scores (0-1 scale) for recommendations")
print(f"Overall Result: {'✅ PASS' if overall_pass else '❌ FAIL'}")

if overall_pass:
    print("\n✅ System provides confidence scores for recommendations")
    print("✅ Confidence scoring demonstrates proper 0-1 scale compliance")
    print("✅ Ready for production deployment from confidence scoring perspective")
else:
    print("\n❌ Confidence scoring system has implementation gaps")
    print("❌ Limited decision support capability")

# Generate recommendations
print(f"\n=== Recommendations ===")
if overall_pass:
    print("- Confidence scoring is functioning correctly")
    print("- Scores provide valuable decision support information")
    print("- Continue monitoring score appropriateness and consistency")
    print("- Continue with other AI decision making tests")
else:
    print("- Implement confidence score provision in AI responses")
    print("- Ensure scores are within 0-1 range as required")
    print("- Improve score correlation with recommendation quality")
    print("- Test confidence scoring with diverse alert scenarios")

EOF

echo ""
echo "🎯 BR-PA-009 Confidence Scoring Test Complete!"
echo "📊 Results validate AI confidence scoring capability"
echo "🎯 System decision support quality assessed"
```

---

## 🎉 **Expected Outcomes**

### **Success Indicators**
- ✅ **Confidence scores provided**: All recommendations include confidence scores
- ✅ **0-1 scale compliance**: Scores strictly within required range (0.0 to 1.0)
- ✅ **Score appropriateness**: Scores correlate with scenario complexity and context clarity
- ✅ **Consistency demonstrated**: Similar scenarios receive similar confidence scores
- ✅ **Decision support enabled**: Operators can prioritize actions based on confidence levels

### **Business Value**
- **Decision Support**: Enables operators to prioritize recommendations based on confidence
- **Risk Management**: Allows assessment of recommendation reliability before execution
- **Quality Indication**: Provides immediate feedback on AI decision certainty
- **Operational Guidance**: Helps determine when human intervention may be needed

### **Integration Confidence**
- **Score Generation**: AI system properly generates and formats confidence scores
- **Scale Compliance**: Scores consistently within business requirement range
- **Contextual Appropriateness**: Scores reflect scenario complexity and available context
- **Consistent Scoring**: Similar scenarios receive appropriately similar confidence levels

---

**This functional integration test validates the confidence scoring business requirement through comprehensive testing of score provision, range compliance, and consistency, ensuring reliable AI decision confidence assessment for production deployment.**
