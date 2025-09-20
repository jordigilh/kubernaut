# BR-PA-003: Alert Processing Time Test

**Business Requirement**: BR-PA-003 - MUST process alerts within 5 seconds of receipt
**Test Strategy**: Hybrid Performance Testing (Requirement Validation + Capacity Exploration)
**Priority**: Critical - Blocking for Milestone 1
**Estimated Duration**: 2 hours

---

## 🎯 **Business Requirement Context**

### **BR-PA-003 Definition**
From `docs/requirements/01_MAIN_APPLICATIONS.md`:
> **BR-PA-003**: MUST process alerts within 5 seconds of receipt

### **Success Criteria**
- **95th percentile response time** < 5 seconds
- **No individual request** > 10 seconds (hard limit)
- **Success rate** ≥ 99% under normal load
- **Consistent performance** across different alert types

### **Business Impact**
- **Customer SLA compliance**: Response time commitments to users
- **Operational effectiveness**: Rapid incident response capability
- **System reliability**: Predictable performance characteristics

---

## 🔬 **Hybrid Testing Approach**

### **Phase A: Requirement Validation** (45 minutes)
**Objective**: Prove the system meets the documented 5-second requirement

**Test Scenarios**:
1. **Normal Load Validation**: 60 alerts/minute for 10 minutes
2. **Mixed Alert Types**: Different alert complexities and sizes
3. **Sustained Load**: 30-minute continuous processing test
4. **Statistical Analysis**: 95th percentile calculation

### **Phase B: Capacity Exploration** (75 minutes)
**Objective**: Understand performance characteristics and limits

**Test Scenarios**:
1. **Response Time Curve**: How does response time change with load?
2. **Breaking Point Analysis**: At what point does response time exceed 5 seconds?
3. **Alert Complexity Impact**: How do complex alerts affect processing time?
4. **Recovery Analysis**: How quickly does system recover after overload?

---

## ⚙️ **Test Environment Setup**

### **Prerequisites**
```bash
# Verify environment is ready
cd ~/kubernaut-integration-test
./scripts/environment_health_check.sh

# Verify Kubernaut applications are running
curl -f http://localhost:8080/health || echo "Prometheus Alerts SLM not ready"

# Set test variables
export WEBHOOK_URL="http://localhost:8080/webhook/prometheus"
export TEST_SESSION="processing_time_test_$(date +%Y%m%d_%H%M%S)"
mkdir -p "results/$TEST_SESSION"
```

### **Test Data Preparation**
```bash
# Create processing time specific alert templates
cat > "results/$TEST_SESSION/alert_templates.json" << 'EOF'
{
  "simple_alert": {
    "labels_count": 5,
    "annotations_count": 3,
    "description_length": 100,
    "expected_processing": "fast"
  },
  "complex_alert": {
    "labels_count": 15,
    "annotations_count": 10,
    "description_length": 1000,
    "expected_processing": "moderate"
  },
  "large_alert": {
    "labels_count": 25,
    "annotations_count": 20,
    "description_length": 5000,
    "expected_processing": "slow"
  }
}
EOF
```

---

## 🧪 **Phase A: Business Requirement Validation**

### **Test A1: Normal Load Validation**

**Objective**: Validate 5-second requirement under normal operational load

```bash
echo "=== Phase A1: Normal Load Validation ==="
echo "Business Requirement: BR-PA-003"
echo "Load: 60 alerts/minute (normal operational rate)"
echo "Duration: 10 minutes"
echo "Success Criteria: 95th percentile < 5 seconds"

# Create detailed response time measurement script
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/detailed_response_time_test.py" "results/$TEST_SESSION/detailed_response_time_test.py"
chmod +x "results/$TEST_SESSION/detailed_response_time_test.py" 

chmod +x "results/$TEST_SESSION/detailed_response_time_test.py"

# Run Phase A1 test
python3 "results/$TEST_SESSION/detailed_response_time_test.py" "$WEBHOOK_URL" "$TEST_SESSION"

# Check if test passed
if grep -q '"pass": true' "results/$TEST_SESSION/phase_a1_detailed_results.json"; then
    echo "✅ Phase A1: PASSED - Normal load validation successful"
else
    echo "❌ Phase A1: FAILED - Normal load validation failed"
    echo "⚠️  Review results before proceeding to Phase B"
fi
```

### **Test A2: Sustained Load Validation**

**Objective**: Validate consistent performance over extended period

```bash
echo "=== Phase A2: Sustained Load Validation ==="
echo "Load: 100 alerts/minute (higher sustained rate)"
echo "Duration: 30 minutes"
echo "Success Criteria: Consistent 95th percentile < 5 seconds"

# Create sustained load test
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/sustained_load_test.sh" "results/$TEST_SESSION/sustained_load_test.sh"
chmod +x "results/$TEST_SESSION/sustained_load_test.sh" 

# Run sustained load test
./results/$TEST_SESSION/sustained_load_test.sh "$WEBHOOK_URL" "$TEST_SESSION"
```

---

## 📊 **Phase B: Capacity Exploration**

### **Test B1: Response Time Curve Analysis**

**Objective**: Understand how response time changes with increasing load

```bash
echo "=== Phase B1: Response Time Curve Analysis ==="
echo "Objective: Map response time vs load relationship"
echo "Method: Progressive load increase with detailed measurements"

# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/response_time_curve.py" "results/$TEST_SESSION/response_time_curve.py"
chmod +x "results/$TEST_SESSION/response_time_curve.py" 

# Run curve analysis
python3 "results/$TEST_SESSION/response_time_curve.py" "$WEBHOOK_URL" "$TEST_SESSION"
```

---

## 📈 **Test Results Analysis**

### **Comprehensive Results Summary**

```bash
echo "=== BR-PA-003 Processing Time Test - Final Summary ==="

# Generate comprehensive report
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/BR_PA_003_FINAL_REPORT.md" "results/$TEST_SESSION/BR_PA_003_FINAL_REPORT.md"
chmod +x "results/$TEST_SESSION/BR_PA_003_FINAL_REPORT.md" 

echo "Final report template created: results/$TEST_SESSION/BR_PA_003_FINAL_REPORT.md"
echo ""
echo "🎯 BR-PA-003 Test Execution Complete!"
echo "📊 Review detailed results in results/$TEST_SESSION/"
echo "🚀 Results inform overall Milestone 1 readiness assessment"
```

---

## 🎉 **Expected Outcomes**

### **Phase A Success Indicators**
- ✅ 95th percentile response time < 5 seconds under normal load
- ✅ Consistent performance across 30-minute sustained test
- ✅ >99% success rate for alert processing
- ✅ Minimal variation between different alert complexities

### **Phase B Insights**
- 📊 **Capacity Headroom**: How much beyond 1000 alerts/min can the system handle?
- 📈 **Performance Curve**: Linear vs exponential response time degradation
- 🔍 **Breaking Point**: Exact load level where system performance fails
- 🔄 **Recovery Time**: How quickly system returns to normal after overload

### **Business Value**
- **Pilot Deployment Confidence**: Validated performance under realistic conditions
- **Capacity Planning**: Data for scaling decisions in future milestones
- **SLA Setting**: Realistic performance commitments to stakeholders
- **Operational Limits**: Clear understanding of system boundaries

---

**This test suite demonstrates the hybrid approach: validating exact business requirements first, then exploring system capabilities for informed capacity planning and operational confidence.**
