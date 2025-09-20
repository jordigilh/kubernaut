#!/usr/bin/env python3
"""
Extracted script from TEST_EXECUTION_GUIDE.md
Business requirement: Extracted following project guidelines for reusability
"""
import csv

with open('results/' + '$TEST_SESSION' + '/concurrent_capacity.csv', 'r') as f:
    reader = csv.DictReader(f)
    results = list(reader)

print("\n--- Concurrent Capacity Analysis ---")
print("Concurrency | Success Rate | Avg Response Time")
print("-" * 45)

for result in results:
    concurrency = result['concurrency_level']
    success_rate = float(result['success_rate'])
    response_time = float(result['avg_response_time'])
    print(f"{concurrency:>10} | {success_rate:>10.1f}% | {response_time:>13.3f}s")

# Find maximum sustainable concurrency (95%+ success, <5s response)
sustainable_levels = []
for result in results:
    success_rate = float(result['success_rate'])
    response_time = float(result['avg_response_time'])
    if success_rate >= 95 and response_time < 5.0:
        sustainable_levels.append(int(result['concurrency_level']))

max_sustainable = max(sustainable_levels) if sustainable_levels else 0

print(f"\n--- Concurrent Capacity Results ---")
print(f"Maximum Sustainable Concurrency (95%+ success, <5s): {max_sustainable}")
print(f"Business Requirement (100 concurrent): {'SUSTAINABLE' if max_sustainable >= 100 else 'AT RISK'}")

if max_sustainable >= 500:
    print("✅ Excellent concurrent request handling capacity")
elif max_sustainable >= 200:
    print("✅ Good concurrent request handling capacity")
elif max_sustainable >= 100:
    print("⚠️  Adequate capacity, meets minimum requirements")
else:
    print("❌ Insufficient capacity for business requirements")
