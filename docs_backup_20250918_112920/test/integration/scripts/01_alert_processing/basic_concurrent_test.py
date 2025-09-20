#!/usr/bin/env python3
"""
Basic Concurrent Processing Test
Tests the ability to handle 100 concurrent requests correctly
"""
import json
import requests
import time
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed
import queue

class BasicConcurrentTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results_queue = queue.Queue()
        self.results = []

    def send_single_request(self, alert_data):
        """Send a single alert request"""
        request_id = alert_data["request_id"]

        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=alert_data["payload"],
                headers={'Content-Type': 'application/json'},
                timeout=30  # Increased timeout for concurrent processing
            )

            end_time = time.time()

            result = {
                "request_id": request_id,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "success": response.status_code == 200,
                "timestamp": start_time,
                "response_size": len(response.content) if response.content else 0
            }

            self.results_queue.put(result)
            return result

        except Exception as e:
            error_result = {
                "request_id": request_id,
                "status_code": -1,
                "response_time": -1,
                "success": False,
                "timestamp": time.time(),
                "error": str(e)
            }

            self.results_queue.put(error_result)
            return error_result

    def test_concurrent_processing(self, alerts_file, max_workers=100):
        """Execute concurrent alert processing test"""
        # Load test alerts
        with open(alerts_file, 'r') as f:
            alerts = json.load(f)

        print(f"Starting concurrent processing test with {len(alerts)} alerts")
        print(f"Max concurrent workers: {max_workers}")

        start_time = time.time()

        # Execute concurrent requests
        with ThreadPoolExecutor(max_workers=max_workers) as executor:
            # Submit all requests
            future_to_alert = {
                executor.submit(self.send_single_request, alert): alert
                for alert in alerts
            }

            # Collect results as they complete
            completed_count = 0
            for future in as_completed(future_to_alert):
                try:
                    result = future.result()
                    completed_count += 1

                    # Progress reporting
                    if completed_count % 20 == 0:
                        print(f"Completed: {completed_count}/{len(alerts)}")

                except Exception as e:
                    print(f"Request failed with exception: {e}")

        end_time = time.time()

        # Collect all results from queue
        while not self.results_queue.empty():
            self.results.append(self.results_queue.get())

        total_time = end_time - start_time

        return self.analyze_concurrent_results(total_time)

    def analyze_concurrent_results(self, total_time):
        """Analyze concurrent processing results"""
        if not self.results:
            return {"error": "No results to analyze"}

        successful_requests = [r for r in self.results if r["success"]]
        failed_requests = [r for r in self.results if not r["success"]]

        analysis = {
            "total_requests": len(self.results),
            "successful_requests": len(successful_requests),
            "failed_requests": len(failed_requests),
            "success_rate": (len(successful_requests) / len(self.results)) * 100,
            "total_execution_time": total_time,
            "average_response_time": sum(r["response_time"] for r in successful_requests) / max(len(successful_requests), 1),
            "max_response_time": max((r["response_time"] for r in successful_requests), default=0),
            "min_response_time": min((r["response_time"] for r in successful_requests), default=0),
            "concurrent_throughput": len(self.results) / total_time if total_time > 0 else 0
        }

        # Business requirement validation
        analysis["br_pa_004_compliance"] = {
            "requirement": "100 concurrent requests processed successfully",
            "total_concurrent": analysis["total_requests"],
            "successful_concurrent": analysis["successful_requests"],
            "success_rate": analysis["success_rate"],
            "pass": (analysis["total_requests"] >= 100 and
                    analysis["successful_requests"] >= 100 and
                    analysis["success_rate"] >= 99.0),  # Allow 1% error margin
            "failed_request_ids": [r["request_id"] for r in failed_requests]
        }

        # Concurrency analysis
        analysis["concurrency_analysis"] = self._analyze_concurrency_patterns()

        return analysis

    def _analyze_concurrency_patterns(self):
        """Analyze concurrency-specific patterns"""
        if not self.results:
            return {}

        # Group requests by timestamp to analyze true concurrency
        timestamps = [r["timestamp"] for r in self.results if r["timestamp"] > 0]

        if not timestamps:
            return {}

        timestamps.sort()

        # Calculate actual concurrency (requests started within 1-second windows)
        concurrency_windows = []
        window_start = timestamps[0]
        window_requests = 0

        for ts in timestamps:
            if ts - window_start <= 1.0:  # 1-second window
                window_requests += 1
            else:
                concurrency_windows.append(window_requests)
                window_start = ts
                window_requests = 1

        if window_requests > 0:
            concurrency_windows.append(window_requests)

        return {
            "max_concurrent_in_window": max(concurrency_windows) if concurrency_windows else 0,
            "average_concurrent_in_window": sum(concurrency_windows) / len(concurrency_windows) if concurrency_windows else 0,
            "total_time_windows": len(concurrency_windows)
        }

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = BasicConcurrentTest(webhook_url)
    results = tester.test_concurrent_processing(f"results/{test_session}/concurrent_alerts.json")

    # Save results
    with open(f"results/{test_session}/basic_concurrent_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== Basic Concurrent Processing Results ===")
    print(f"Total Requests: {results['total_requests']}")
    print(f"Successful: {results['successful_requests']}")
    print(f"Failed: {results['failed_requests']}")
    print(f"Success Rate: {results['success_rate']:.2f}%")
    print(f"Total Execution Time: {results['total_execution_time']:.2f}s")
    print(f"Average Response Time: {results['average_response_time']:.3f}s")
    print(f"Concurrent Throughput: {results['concurrent_throughput']:.2f} requests/second")

    concurrency = results["concurrency_analysis"]
    print(f"Max Concurrent in 1s Window: {concurrency.get('max_concurrent_in_window', 0)}")

    compliance = results["br_pa_004_compliance"]
    print(f"\n=== BR-PA-004 Compliance ===")
    print(f"Requirement: {compliance['requirement']}")
    print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")

    if not compliance['pass']:
        print(f"Failed Request IDs: {compliance['failed_request_ids'][:10]}...")  # Show first 10