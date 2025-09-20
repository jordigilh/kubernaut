# BR-PA-001: Alert Reception & Availability Test

**Business Requirement**: BR-PA-001 - MUST receive Prometheus alerts via HTTP webhooks with 99.9% availability
**Test Strategy**: Hybrid Performance Testing (Requirement Validation + Capacity Exploration)
**Priority**: Critical - Blocking for Milestone 1
**Estimated Duration**: 3 hours

---

## 🎯 **Business Requirement Context**

### **BR-PA-001 Definition**
From `docs/requirements/01_MAIN_APPLICATIONS.md`:
> **BR-PA-001**: MUST receive Prometheus alerts via HTTP webhooks with 99.9% availability

### **Success Criteria**
- **99.9% availability** during continuous operation (max 8.64 seconds downtime per 24 hours)
- **Webhook endpoint responsiveness** under sustained load
- **Service recovery time** < 30 seconds after transient failures
- **No alert loss** during service restart or brief unavailability

### **Business Impact**
- **Customer SLA compliance**: Availability commitments to monitoring services
- **Operational continuity**: Uninterrupted alert processing capability
- **Business credibility**: Reliability of core service offering

---

## 🔬 **Hybrid Testing Approach**

### **Phase A: Requirement Validation** (90 minutes)
**Objective**: Prove the system meets the documented 99.9% availability requirement

**Test Scenarios**:
1. **Continuous Monitoring**: 4-hour continuous availability test
2. **Service Restart Simulation**: Planned restart with availability measurement
3. **Load Under Availability**: Sustained load with availability tracking
4. **Statistical Validation**: Exact 99.9% calculation with confidence intervals

### **Phase B: Capacity Exploration** (90 minutes)
**Objective**: Understand availability characteristics under stress and failure conditions

**Test Scenarios**:
1. **Stress Availability**: Availability under maximum load conditions
2. **Failure Recovery**: Recovery time analysis from various failure scenarios
3. **Degradation Points**: At what load does availability start to suffer?
4. **Resilience Limits**: Maximum sustainable load while maintaining 99.9%

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
export TEST_SESSION="availability_test_$(date +%Y%m%d_%H%M%S)"
mkdir -p "results/$TEST_SESSION"
```

### **Availability Monitoring Infrastructure Setup**
```bash
# Create availability monitoring script
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/availability_monitor.py" "results/$TEST_SESSION/availability_monitor.py"
chmod +x "results/$TEST_SESSION/availability_monitor.py" 
```

---

## 🧪 **Phase A: Business Requirement Validation**

### **Test A1: Continuous Availability Validation**

**Objective**: Validate 99.9% availability requirement under normal operational conditions

```bash
echo "=== Phase A1: Continuous Availability Validation ==="
echo "Business Requirement: BR-PA-001"
echo "Duration: 240 minutes (4 hours)"
echo "Check Interval: 5 seconds"
echo "Success Criteria: ≥99.9% availability"

# Run 4-hour continuous availability test
python3 "results/$TEST_SESSION/availability_monitor.py" "$WEBHOOK_URL" 240 "$TEST_SESSION"

# Validate results
if python3 -c "
import json
with open('results/$TEST_SESSION/availability_detailed_results.json', 'r') as f:
    data = json.load(f)
exit(0 if data['br_pa_001_compliance']['pass'] else 1)
"; then
    echo "✅ Phase A1: PASSED - Continuous availability requirement met"
else
    echo "❌ Phase A1: FAILED - Continuous availability requirement not met"
fi
```

### **Test A2: Service Restart Availability Impact**

**Objective**: Validate availability during planned service restarts

```bash
echo "=== Phase A2: Service Restart Availability Impact ==="
echo "Scenario: Planned service restart during monitoring"
echo "Duration: 30 minutes with restart at 15-minute mark"

# Create restart test script
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/restart_availability_test.sh" "results/$TEST_SESSION/restart_availability_test.sh"
chmod +x "results/$TEST_SESSION/restart_availability_test.sh" 

# Run restart test
./results/$TEST_SESSION/restart_availability_test.sh "$WEBHOOK_URL" "$TEST_SESSION"

# Analyze restart impact
python3 << 'RESTART_ANALYSIS_EOF'
import json

try:
    with open("results/" + "$TEST_SESSION" + "_restart/availability_detailed_results.json", "r") as f:
        data = json.load(f)

    print("\n=== Service Restart Impact Analysis ===")
    print(f"Overall Availability: {data['availability_percentage']:.4f}%")
    print(f"Total Downtime: {data['downtime_seconds']:.2f} seconds")
    print(f"Downtime Incidents: {len(data['downtime_periods'])}")

    # Check if availability remained above 99.9% even with restart
    compliance = data["br_pa_001_compliance"]
    print(f"\nRestart Tolerance: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    print(f"Availability with restart: {compliance['measured_availability']:.4f}%")

    if data['downtime_periods']:
        max_downtime = max(p['duration'] for p in data['downtime_periods'])
        print(f"Maximum single downtime: {max_downtime:.2f} seconds")

        # Business requirement: restart should not cause >30 seconds downtime
        restart_compliant = max_downtime <= 30
        print(f"Restart Recovery: {'✅ PASS' if restart_compliant else '❌ FAIL'} (<30s requirement)")

except FileNotFoundError:
    print("❌ Restart test results not found")

RESTART_ANALYSIS_EOF
```

### **Test A3: Availability Under Load**

**Objective**: Validate availability maintenance under sustained alert load

```bash
echo "=== Phase A3: Availability Under Load ==="
echo "Scenario: 99.9% availability while processing 100 alerts/minute"
echo "Duration: 60 minutes"

# Create availability under load test
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/availability_under_load_test.py" "results/$TEST_SESSION/availability_under_load_test.py"
chmod +x "results/$TEST_SESSION/availability_under_load_test.py" 

# Run availability under load test
python3 "results/$TEST_SESSION/availability_under_load_test.py" "$WEBHOOK_URL" "$TEST_SESSION"
```

---

## 📊 **Phase B: Capacity Exploration**

### **Test B1: Availability Under Maximum Load**

**Objective**: Find the maximum load the system can handle while maintaining 99.9% availability

```bash
echo "=== Phase B1: Availability Under Maximum Load ==="
echo "Objective: Find maximum sustainable load with 99.9% availability"

# Create maximum load availability test
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/max_load_availability_test.py" "results/$TEST_SESSION/max_load_availability_test.py"
chmod +x "results/$TEST_SESSION/max_load_availability_test.py" 

# Run maximum load availability exploration
python3 "results/$TEST_SESSION/max_load_availability_test.py" "$WEBHOOK_URL" "$TEST_SESSION"
```

---

## 📈 **Test Results Analysis & Final Report**

### **Comprehensive Availability Assessment**

```bash
echo "=== BR-PA-001 Availability Test - Final Analysis ==="

# Create comprehensive availability report
# Copy the script to your test session directory
cp "../../../scripts/01_alert_processing/BR_PA_001_FINAL_REPORT.md" "results/$TEST_SESSION/BR_PA_001_FINAL_REPORT.md"
chmod +x "results/$TEST_SESSION/BR_PA_001_FINAL_REPORT.md" 

echo "Availability test report template created: results/$TEST_SESSION/BR_PA_001_FINAL_REPORT.md"
echo ""
echo "🎯 BR-PA-001 Availability Test Complete!"
echo "📊 Results validate webhook availability under various conditions"
echo "🚀 Data informs production deployment availability confidence"
```

---

## 🎉 **Expected Outcomes**

### **Phase A Success Indicators**
- ✅ 99.9% availability maintained during 4-hour continuous operation
- ✅ Service restart impact ≤30 seconds downtime
- ✅ Availability maintained under 100 alerts/minute load
- ✅ Statistical confidence in availability measurement

### **Phase B Insights**
- 📊 **Maximum Sustainable Load**: Highest alert rate with 99.9% availability
- 🔍 **Degradation Characteristics**: How availability degrades under extreme load
- 💡 **Capacity Planning**: Headroom above business requirements
- ⚠️ **Risk Assessment**: Pilot deployment availability risk evaluation

### **Business Value**
- **SLA Confidence**: Validated ability to meet availability commitments
- **Operational Planning**: Understanding of service restart impacts
- **Capacity Planning**: Data-driven scaling decisions for future growth
- **Risk Management**: Quantified availability risks for pilot deployment

---

**This test suite validates the critical availability business requirement through comprehensive testing of normal operations, failure scenarios, and capacity limits, providing confidence for production deployment decisions.**
