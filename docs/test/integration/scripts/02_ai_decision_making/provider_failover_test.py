#!/usr/bin/env python3
"""
Provider Failover Test
Tests failover mechanisms when primary LLM providers are unavailable
"""
import json
import requests
import time

class ProviderFailoverTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []

    def test_failover_scenarios(self):
        """Test various failover scenarios"""

        # Test scenario 1: Normal operation (all providers available)
        print("Testing normal operation scenario...")
        normal_result = self.test_normal_operation()
        self.results.append(normal_result)

        time.sleep(2)

        # Test scenario 2: Simulated provider unavailability
        print("Testing provider unavailability scenario...")
        unavailable_result = self.test_provider_unavailability()
        self.results.append(unavailable_result)

        return self.analyze_failover_results()

    def test_normal_operation(self):
        """Test normal operation with providers available"""
        test_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "FailoverTest_Normal",
                    "severity": "warning",
                    "namespace": "test-workloads",
                    "failover_test": "normal_operation"
                },
                "annotations": {
                    "description": "Testing normal LLM provider operation",
                    "summary": "Failover test - normal operation",
                    "test_type": "normal"
                },
                "startsAt": "2025-01-01T10:00:00Z"
            }]
        }

        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=test_alert,
                headers={
                    'Content-Type': 'application/json',
                    'X-Failover-Test': 'normal'
                },
                timeout=20
            )

            end_time = time.time()

            return {
                "scenario": "normal_operation",
                "status_code": response.status_code,
                "success": response.status_code == 200,
                "response_time": end_time - start_time,
                "response_length": len(response.text) if response.text else 0,
                "timestamp": start_time
            }

        except Exception as e:
            return {
                "scenario": "normal_operation",
                "status_code": -1,
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def test_provider_unavailability(self):
        """Test behavior when primary provider is unavailable"""
        # Note: In a real test, this might involve temporarily disabling a provider
        # For this integration test, we simulate unavailability through special headers

        test_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "FailoverTest_Unavailable",
                    "severity": "critical",
                    "namespace": "test-workloads",
                    "failover_test": "provider_unavailable"
                },
                "annotations": {
                    "description": "Testing LLM provider failover when primary unavailable",
                    "summary": "Failover test - provider unavailable",
                    "test_type": "failover"
                },
                "startsAt": "2025-01-01T10:00:00Z"
            }]
        }

        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=test_alert,
                headers={
                    'Content-Type': 'application/json',
                    'X-Failover-Test': 'unavailable',
                    'X-Simulate-Provider-Failure': 'primary'  # Hint to system to simulate failure
                },
                timeout=30  # Longer timeout to allow for failover
            )

            end_time = time.time()

            return {
                "scenario": "provider_unavailable",
                "status_code": response.status_code,
                "success": response.status_code == 200,
                "response_time": end_time - start_time,
                "response_length": len(response.text) if response.text else 0,
                "failover_attempted": True,
                "timestamp": start_time
            }

        except Exception as e:
            return {
                "scenario": "provider_unavailable",
                "status_code": -1,
                "success": False,
                "error": str(e),
                "failover_attempted": True,
                "timestamp": time.time()
            }

    def analyze_failover_results(self):
        """Analyze failover test results"""
        normal_result = next((r for r in self.results if r["scenario"] == "normal_operation"), None)
        failover_result = next((r for r in self.results if r["scenario"] == "provider_unavailable"), None)

        if not normal_result or not failover_result:
            return {"error": "Incomplete failover test results"}

        analysis = {
            "normal_operation": normal_result,
            "failover_scenario": failover_result,
            "both_scenarios_tested": True,
            "normal_success": normal_result["success"],
            "failover_success": failover_result["success"],
            "failover_response_time": failover_result.get("response_time", 0),
            "response_time_difference": failover_result.get("response_time", 0) - normal_result.get("response_time", 0)
        }

        # Failover quality assessment
        failover_quality = "excellent"
        if not failover_result["success"]:
            failover_quality = "failed"
        elif failover_result.get("response_time", 0) > normal_result.get("response_time", 0) * 3:
            failover_quality = "slow_but_functional"
        elif failover_result.get("response_time", 0) > normal_result.get("response_time", 0) * 2:
            failover_quality = "adequate"

        analysis["failover_quality"] = failover_quality

        # Business requirement validation
        analysis["br_pa_006_failover_compliance"] = {
            "requirement": "Provider failover working correctly between providers",
            "normal_operation_success": analysis["normal_success"],
            "failover_mechanism_success": analysis["failover_success"],
            "failover_quality": analysis["failover_quality"],
            "pass": (analysis["normal_success"] and analysis["failover_success"]),
            "resilience_rating": "high" if failover_quality == "excellent" else "medium" if failover_quality in ["adequate", "slow_but_functional"] else "low"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = ProviderFailoverTest(webhook_url)
    results = tester.test_failover_scenarios()

    # Save results
    with open(f"results/{test_session}/provider_failover_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    if "error" not in results:
        print(f"\n=== Provider Failover Test Results ===")
        print(f"Normal Operation: {'✅ SUCCESS' if results['normal_success'] else '❌ FAILED'}")
        print(f"Failover Scenario: {'✅ SUCCESS' if results['failover_success'] else '❌ FAILED'}")

        if results["failover_success"]:
            print(f"Failover Quality: {results['failover_quality']}")
            print(f"Response Time Difference: {results['response_time_difference']:.2f}s")

        compliance = results["br_pa_006_failover_compliance"]
        print(f"\n=== Failover Compliance ===")
        print(f"Requirement: {compliance['requirement']}")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
        print(f"Resilience Rating: {compliance['resilience_rating']}")
    else:
        print(f"❌ Error: {results['error']}")