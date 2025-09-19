#!/usr/bin/env python3
"""
Extracted script from ENVIRONMENT_SETUP.md
Business requirement: Extracted following project guidelines for reusability
"""
import csv
import statistics
import sys

results_file = f"results/concurrent_test_{sys.argv[1]}.csv"

with open(results_file, 'r') as f:
    reader = csv.DictReader(f)
    results = list(reader)

# Calculate statistics
response_times = [float(r['duration']) for r in results]
http_codes = [int(r['http_code']) for r in results if r['http_code'].isdigit()]

success_count = sum(1 for code in http_codes if code == 200)
total_requests = len(results)
success_rate = (success_count / total_requests) * 100

print(f"\n--- Concurrent Load Test Results ---")
print(f"Total Requests: {total_requests}")
print(f"Successful Requests (200): {success_count}")
print(f"Success Rate: {success_rate:.2f}%")
print(f"Average Response Time: {statistics.mean(response_times):.3f}s")
print(f"Median Response Time: {statistics.median(response_times):.3f}s")
print(f"95th Percentile Response Time: {sorted(response_times)[int(len(response_times) * 0.95)]:.3f}s")
print(f"Max Response Time: {max(response_times):.3f}s")

# Business requirement validation
print(f"\n--- Business Requirement Validation ---")
if success_rate >= 95:
    print("✅ BR-PA-004: Concurrent request handling - PASS")
else:
    print("❌ BR-PA-004: Concurrent request handling - FAIL")

avg_response = statistics.mean(response_times)
if avg_response < 5.0:
    print("✅ BR-PA-003: 5-second processing time - PASS")
else:
    print("❌ BR-PA-003: 5-second processing time - FAIL")
