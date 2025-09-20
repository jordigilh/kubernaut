#!/usr/bin/env python3
"""
Detailed Response Time Test for BR-PA-003
Measures exact response times and validates against business requirements
"""
import json
import time
import requests
import statistics
from datetime import datetime
import threading
import queue

class DetailedResponseTimeTest:
    def __init__(self, webhook_url, alerts_per_minute=60, duration_minutes=10):
        self.webhook_url = webhook_url
        self.alerts_per_minute = alerts_per_minute
        self.duration_minutes = duration_minutes
        self.interval = 60.0 / alerts_per_minute
        self.results = []
        self.results_lock = threading.Lock()

    def generate_alert(self, alert_id, alert_type="simple"):
        """Generate alert based on complexity type"""
        base_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": f"ProcessingTimeTest_{alert_type}",
                    "severity": "warning",
                    "test_id": str(alert_id),
                    "alert_type": alert_type
                },
                "annotations": {
                    "description": f"Response time test alert {alert_id}",
                    "summary": f"Testing processing time with {alert_type} alert"
                },
                "startsAt": datetime.now().isoformat()
            }]
        }

        # Add complexity based on alert type
        if alert_type == "complex":
            # Add more labels and annotations
            for i in range(10):
                base_alert["alerts"][0]["labels"][f"extra_label_{i}"] = f"value_{i}"
                base_alert["alerts"][0]["annotations"][f"extra_annotation_{i}"] = f"annotation_value_{i}"
        elif alert_type == "large":
            # Add large description
            base_alert["alerts"][0]["annotations"]["description"] = "Large alert description: " + ("X" * 4500)
            for i in range(20):
                base_alert["alerts"][0]["labels"][f"label_{i}"] = f"value_{i}"

        return base_alert

    def send_alert_with_timing(self, alert_data, alert_id):
        """Send alert and measure precise timing"""
        try:
            start_time = time.perf_counter()

            response = requests.post(
                self.webhook_url,
                json=alert_data,
                headers={'Content-Type': 'application/json'},
                timeout=15
            )

            end_time = time.perf_counter()
            response_time = end_time - start_time

            result = {
                "alert_id": alert_id,
                "response_time": response_time,
                "status_code": response.status_code,
                "timestamp": start_time,
                "alert_type": alert_data["alerts"][0]["labels"].get("alert_type", "simple"),
                "success": response.status_code == 200
            }

            with self.results_lock:
                self.results.append(result)

            return result

        except Exception as e:
            error_result = {
                "alert_id": alert_id,
                "response_time": -1,
                "status_code": -1,
                "timestamp": time.perf_counter(),
                "alert_type": "error",
                "success": False,
                "error": str(e)
            }

            with self.results_lock:
                self.results.append(error_result)

            return error_result

    def run_test(self):
        """Execute the response time test"""
        print(f"Starting detailed response time test...")
        print(f"Rate: {self.alerts_per_minute} alerts/minute")
        print(f"Duration: {self.duration_minutes} minutes")
        print(f"Target: 95th percentile < 5 seconds")

        end_time = time.time() + (self.duration_minutes * 60)
        alert_id = 0

        # Alert type distribution (realistic mix)
        alert_types = ["simple"] * 70 + ["complex"] * 25 + ["large"] * 5

        while time.time() < end_time:
            alert_type = alert_types[alert_id % len(alert_types)]
            alert = self.generate_alert(alert_id, alert_type)

            # Send alert (non-blocking)
            threading.Thread(
                target=self.send_alert_with_timing,
                args=(alert, alert_id),
                daemon=True
            ).start()

            alert_id += 1

            # Progress reporting
            if alert_id % 20 == 0:
                with self.results_lock:
                    completed = len(self.results)
                    if completed > 0:
                        recent_times = [r["response_time"] for r in self.results[-10:] if r["success"]]
                        if recent_times:
                            avg_recent = statistics.mean(recent_times)
                            print(f"Sent: {alert_id}, Completed: {completed}, Recent Avg: {avg_recent:.3f}s")

            time.sleep(self.interval)

        # Wait for remaining responses (max 30 seconds)
        print("Waiting for remaining responses...")
        wait_start = time.time()
        while len(self.results) < alert_id and time.time() - wait_start < 30:
            time.sleep(0.1)

        return self.analyze_results()

    def analyze_results(self):
        """Analyze results against business requirements"""
        successful_results = [r for r in self.results if r["success"]]

        if not successful_results:
            return {"error": "No successful results to analyze"}

        response_times = [r["response_time"] for r in successful_results]

        analysis = {
            "total_alerts": len(self.results),
            "successful_alerts": len(successful_results),
            "success_rate": (len(successful_results) / len(self.results)) * 100,
            "mean_response_time": statistics.mean(response_times),
            "median_response_time": statistics.median(response_times),
            "percentile_95": sorted(response_times)[int(len(response_times) * 0.95)],
            "percentile_99": sorted(response_times)[int(len(response_times) * 0.99)],
            "max_response_time": max(response_times),
            "min_response_time": min(response_times)
        }

        # Business requirement validation
        analysis["br_pa_003_compliance"] = {
            "requirement": "95th percentile < 5 seconds",
            "measured_95th": analysis["percentile_95"],
            "pass": analysis["percentile_95"] < 5.0,
            "margin": 5.0 - analysis["percentile_95"]
        }

        # Alert type breakdown
        alert_type_analysis = {}
        for alert_type in ["simple", "complex", "large"]:
            type_results = [r for r in successful_results if r["alert_type"] == alert_type]
            if type_results:
                type_times = [r["response_time"] for r in type_results]
                alert_type_analysis[alert_type] = {
                    "count": len(type_results),
                    "mean_time": statistics.mean(type_times),
                    "percentile_95": sorted(type_times)[int(len(type_times) * 0.95)] if len(type_times) > 20 else max(type_times)
                }

        analysis["alert_type_breakdown"] = alert_type_analysis

        return analysis

if __name__ == "__main__":
    import sys
    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"

    tester = DetailedResponseTimeTest(webhook_url)
    results = tester.run_test()

    # Save detailed results
    with open(f"results/{sys.argv[2] if len(sys.argv) > 2 else 'test_session'}/phase_a1_detailed_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print("\n=== Phase A1: Business Requirement Validation Results ===")
    print(f"Total Alerts Processed: {results['total_alerts']}")
    print(f"Success Rate: {results['success_rate']:.2f}%")
    print(f"Mean Response Time: {results['mean_response_time']:.3f}s")
    print(f"95th Percentile: {results['percentile_95']:.3f}s")
    print(f"99th Percentile: {results['percentile_99']:.3f}s")
    print(f"Max Response Time: {results['max_response_time']:.3f}s")

    print(f"\n=== BR-PA-003 Compliance ===")
    compliance = results["br_pa_003_compliance"]
    print(f"Requirement: {compliance['requirement']}")
    print(f"Measured 95th Percentile: {compliance['measured_95th']:.3f}s")
    print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    if compliance['pass']:
        print(f"Margin: {compliance['margin']:.3f}s under requirement")
    else:
        print(f"Overage: {-compliance['margin']:.3f}s over requirement")

    print(f"\n=== Alert Type Performance ===")
    for alert_type, data in results["alert_type_breakdown"].items():
        print(f"{alert_type.title()} alerts: {data['count']} processed, avg {data['mean_time']:.3f}s, 95th {data['percentile_95']:.3f}s")