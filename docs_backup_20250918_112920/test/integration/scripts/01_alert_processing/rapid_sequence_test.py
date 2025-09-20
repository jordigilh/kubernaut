#!/usr/bin/env python3
"""
Rapid Sequence Processing Test
Tests order preservation when alerts are delivered in rapid succession
"""
import json
import requests
import time
import threading

class RapidSequenceTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.results_lock = threading.Lock()

    def send_rapid_alert(self, alert_data):
        """Send alert with minimal delay for rapid testing"""
        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=alert_data["payload"],
                headers={
                    'Content-Type': 'application/json',
                    'X-Rapid-Sequence': str(alert_data['sequence_number'])
                },
                timeout=10
            )

            end_time = time.time()

            result = {
                "sequence_number": alert_data["sequence_number"],
                "source_id": alert_data["source_id"],
                "status_code": response.status_code,
                "success": response.status_code == 200,
                "send_time": start_time,
                "complete_time": end_time,
                "response_time": end_time - start_time
            }

            with self.results_lock:
                self.results.append(result)

            return result

        except Exception as e:
            error_result = {
                "sequence_number": alert_data["sequence_number"],
                "source_id": alert_data["source_id"],
                "status_code": -1,
                "success": False,
                "error": str(e),
                "send_time": time.time(),
                "complete_time": time.time()
            }

            with self.results_lock:
                self.results.append(error_result)

            return error_result

    def test_rapid_delivery(self, rapid_file):
        """Test rapid alert delivery with minimal delays"""
        with open(rapid_file, 'r') as f:
            alerts = json.load(f)

        print(f"Testing rapid delivery with {len(alerts)} alerts")
        print("Sending alerts with minimal delay...")

        # Send alerts rapidly (100ms intervals)
        for i, alert in enumerate(alerts):
            result = self.send_rapid_alert(alert)
            print(f"Rapid alert {i}: {'✓' if result['success'] else '✗'}")

            # Very short delay to simulate rapid-fire scenario
            time.sleep(0.1)

        return self.analyze_rapid_results()

    def analyze_rapid_results(self):
        """Analyze rapid sequence results"""
        successful_results = [r for r in self.results if r["success"]]

        if not successful_results:
            return {"error": "No successful rapid sequence results"}

        # Sort by completion time to see processing order
        successful_results.sort(key=lambda x: x["complete_time"])

        # Extract sequence numbers in completion order
        completion_order = [r["sequence_number"] for r in successful_results]
        expected_order = sorted(completion_order)

        # Check for ordering violations
        violations = []
        for i in range(1, len(completion_order)):
            if completion_order[i] < completion_order[i-1]:
                violations.append({
                    "position": i,
                    "expected_previous": completion_order[i-1] + 1,
                    "actual": completion_order[i],
                    "violation_type": "out_of_order"
                })

        # Calculate timing statistics
        response_times = [r["response_time"] for r in successful_results]

        analysis = {
            "total_rapid_alerts": len(self.results),
            "successful_alerts": len(successful_results),
            "success_rate": (len(successful_results) / len(self.results) * 100) if self.results else 0,
            "completion_order": completion_order,
            "expected_order": expected_order,
            "correctly_ordered": completion_order == expected_order,
            "violations": violations,
            "violation_count": len(violations),
            "average_response_time": sum(response_times) / len(response_times) if response_times else 0,
            "max_response_time": max(response_times) if response_times else 0,
            "min_response_time": min(response_times) if response_times else 0
        }

        # Business requirement validation for rapid sequences
        analysis["br_pa_005_rapid_compliance"] = {
            "requirement": "Maintain ordering even under rapid alert delivery",
            "correctly_ordered": analysis["correctly_ordered"],
            "success_rate": analysis["success_rate"],
            "violation_count": analysis["violation_count"],
            "pass": (analysis["correctly_ordered"] and analysis["success_rate"] >= 95.0),
            "rapid_delivery_handling": "excellent" if analysis["violation_count"] == 0 else "needs_improvement"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = RapidSequenceTest(webhook_url)
    results = tester.test_rapid_delivery(f"results/{test_session}/rapid_sequence.json")

    # Save results
    with open(f"results/{test_session}/rapid_sequence_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== Rapid Sequence Test Results ===")
    print(f"Total Rapid Alerts: {results['total_rapid_alerts']}")
    print(f"Successful: {results['successful_alerts']}")
    print(f"Success Rate: {results['success_rate']:.1f}%")
    print(f"Correctly Ordered: {results['correctly_ordered']}")
    print(f"Violations: {results['violation_count']}")
    print(f"Average Response Time: {results['average_response_time']:.3f}s")

    compliance = results["br_pa_005_rapid_compliance"]
    print(f"\n=== Rapid Sequence Compliance ===")
    print(f"Requirement: {compliance['requirement']}")
    print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    print(f"Rapid Delivery Handling: {compliance['rapid_delivery_handling']}")