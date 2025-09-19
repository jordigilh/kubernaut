#!/usr/bin/env python3
"""
Availability Under Load Test
Tests availability while system is under sustained alert processing load
"""
import threading
import time
from availability_monitor import AvailabilityMonitor
from detailed_response_time_test import DetailedResponseTimeTest

class AvailabilityUnderLoadTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.availability_monitor = AvailabilityMonitor(webhook_url, check_interval=10)
        self.load_generator = DetailedResponseTimeTest(webhook_url, alerts_per_minute=100)

    def run_combined_test(self, duration_minutes=60):
        """Run availability monitoring while generating alert load"""
        print(f"Starting availability under load test...")
        print(f"Load: 100 alerts/minute")
        print(f"Duration: {duration_minutes} minutes")

        # Start availability monitoring in background thread
        availability_thread = threading.Thread(
            target=self.availability_monitor.run_availability_monitoring,
            args=(duration_minutes,),
            daemon=True
        )

        # Start load generation in background thread
        load_thread = threading.Thread(
            target=self.load_generator.run_test,
            daemon=True
        )

        availability_thread.start()
        time.sleep(5)  # Brief delay to ensure monitoring starts first
        load_thread.start()

        # Wait for both to complete
        availability_thread.join()
        load_thread.join()

        # Get results from both tests
        availability_results = self.availability_monitor.calculate_availability_statistics()
        load_results = self.load_generator.analyze_results()

        return {
            "availability": availability_results,
            "load_performance": load_results
        }

if __name__ == "__main__":
    import sys
    import json

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = AvailabilityUnderLoadTest(webhook_url)
    results = tester.run_combined_test(60)

    # Save combined results
    with open(f"results/{test_session}/availability_under_load_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print analysis
    availability = results["availability"]
    load = results["load_performance"]

    print(f"\n=== Availability Under Load Results ===")
    print(f"Availability: {availability['availability_percentage']:.4f}%")
    print(f"Load Success Rate: {load['success_rate']:.2f}%")
    print(f"Load Response Time: {load['mean_response_time']:.3f}s average")

    # Combined validation
    availability_pass = availability['br_pa_001_compliance']['pass']
    load_pass = load['success_rate'] >= 95  # Reasonable success rate under load

    print(f"\n=== Combined Test Results ===")
    print(f"Availability Requirement: {'✅ PASS' if availability_pass else '❌ FAIL'}")
    print(f"Load Handling: {'✅ PASS' if load_pass else '❌ FAIL'}")
    print(f"Overall Phase A3: {'✅ PASS' if availability_pass and load_pass else '❌ FAIL'}")