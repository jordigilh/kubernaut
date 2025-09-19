#!/usr/bin/env python3
"""
Valid Alert Processing Test
Verifies that properly formatted alerts are accepted and processed
"""
import json
import requests
import time

class ValidAlertTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []

    def test_valid_alerts(self, test_data_file):
        """Test all valid alert scenarios"""
        with open(test_data_file, 'r') as f:
            test_data = json.load(f)

        valid_alerts = test_data["valid_alerts"]
        print(f"Testing {len(valid_alerts)} valid alert scenarios...")

        for alert_test in valid_alerts:
            result = self.send_alert(alert_test)
            self.results.append(result)

            print(f"  {alert_test['name']}: {result['status']}")

        return self.analyze_valid_results()

    def send_alert(self, alert_test):
        """Send a single alert and capture response"""
        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=alert_test["payload"],
                headers={'Content-Type': 'application/json'},
                timeout=10
            )

            end_time = time.time()

            return {
                "test_name": alert_test["name"],
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "response_text": response.text[:200],  # First 200 chars
                "success": response.status_code == 200,
                "status": "PASS" if response.status_code == 200 else f"FAIL (HTTP {response.status_code})"
            }

        except Exception as e:
            return {
                "test_name": alert_test["name"],
                "status_code": -1,
                "response_time": -1,
                "response_text": str(e),
                "success": False,
                "status": f"ERROR: {str(e)}"
            }

    def analyze_valid_results(self):
        """Analyze valid alert test results"""
        total_tests = len(self.results)
        successful_tests = sum(1 for r in self.results if r["success"])

        analysis = {
            "total_valid_tests": total_tests,
            "successful_tests": successful_tests,
            "success_rate": (successful_tests / total_tests * 100) if total_tests > 0 else 0,
            "failed_tests": [r for r in self.results if not r["success"]],
            "average_response_time": sum(r["response_time"] for r in self.results if r["response_time"] > 0) / max(successful_tests, 1)
        }

        # Business requirement validation
        analysis["br_pa_002_valid_compliance"] = {
            "requirement": "Valid alerts must be accepted and processed",
            "success_rate": analysis["success_rate"],
            "pass": analysis["success_rate"] == 100.0,
            "failed_scenarios": [r["test_name"] for r in analysis["failed_tests"]]
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = ValidAlertTest(webhook_url)
    results = tester.test_valid_alerts(f"results/{test_session}/validation_test_data.json")

    # Save results
    with open(f"results/{test_session}/valid_alert_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== Valid Alert Test Results ===")
    print(f"Total Tests: {results['total_valid_tests']}")
    print(f"Successful: {results['successful_tests']}")
    print(f"Success Rate: {results['success_rate']:.1f}%")
    print(f"Average Response Time: {results['average_response_time']:.3f}s")

    compliance = results["br_pa_002_valid_compliance"]
    print(f"\n=== BR-PA-002 Valid Alert Compliance ===")
    print(f"Requirement: {compliance['requirement']}")
    print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")

    if not compliance['pass']:
        print(f"Failed Scenarios: {', '.join(compliance['failed_scenarios'])}")