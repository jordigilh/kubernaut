#!/usr/bin/env python3
"""
Extracted script from TEST_EXECUTION_GUIDE.md
Business requirement: Extracted following project guidelines for reusability
"""
import json
import glob
import statistics

# Load latest results
result_files = glob.glob("results/alert_generation_*.json")
latest_result = max(result_files, key=lambda x: os.path.getctime(x))

with open(latest_result, 'r') as f:
    data = json.load(f)

avg_response_time = data.get('avg_response_time', 0)
success_rate = data.get('success_rate', 0)

print(f"\n--- BR-PA-003 Validation Results ---")
print(f"Average Response Time: {avg_response_time:.3f}s")
print(f"Success Rate: {success_rate:.2f}%")

# Business requirement validation
if avg_response_time < 5.0 and success_rate >= 95.0:
    print("✅ BR-PA-003: PASS - Response time requirement met")
    exit(0)
else:
    print("❌ BR-PA-003: FAIL - Response time requirement not met")
    exit(1)
