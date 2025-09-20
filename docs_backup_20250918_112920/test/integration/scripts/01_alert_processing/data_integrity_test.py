#!/usr/bin/env python3
"""
Concurrent Data Integrity Test
Verifies data integrity and absence of race conditions during concurrent processing
"""
import json
import requests
import time
import threading
from concurrent.futures import ThreadPoolExecutor
import hashlib

class DataIntegrityTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.results_lock = threading.Lock()

    def send_tracked_request(self, alert_data):
        """Send a request with data integrity tracking"""
        request_id = alert_data["request_id"]

        # Create hash of request data for integrity verification
        request_hash = hashlib.md5(json.dumps(alert_data["payload"], sort_keys=True).encode()).hexdigest()

        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=alert_data["payload"],
                headers={
                    'Content-Type': 'application/json',
                    'X-Request-ID': str(request_id),
                    'X-Request-Hash': request_hash
                },
                timeout=30
            )

            end_time = time.time()

            result = {
                "request_id": request_id,
                "request_hash": request_hash,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "success": response.status_code == 200,
                "timestamp": start_time,
                "thread_id": threading.get_ident(),
                "response_headers": dict(response.headers),
                "response_data": response.text[:500] if response.text else ""  # First 500 chars
            }

            with self.results_lock:
                self.results.append(result)

            return result

        except Exception as e:
            error_result = {
                "request_id": request_id,
                "request_hash": request_hash,
                "status_code": -1,
                "response_time": -1,
                "success": False,
                "timestamp": time.time(),
                "thread_id": threading.get_ident(),
                "error": str(e)
            }

            with self.results_lock:
                self.results.append(error_result)

            return error_result

    def test_duplicate_processing(self, duplicate_alerts_file):
        """Test handling of duplicate alerts sent concurrently"""
        with open(duplicate_alerts_file, 'r') as f:
            duplicate_alerts = json.load(f)

        print(f"Testing duplicate processing with {len(duplicate_alerts)} concurrent duplicates")

        # Send all duplicate alerts concurrently
        with ThreadPoolExecutor(max_workers=50) as executor:
            futures = [
                executor.submit(self.send_tracked_request, alert)
                for alert in duplicate_alerts
            ]

            # Wait for all to complete
            for future in futures:
                future.result()

        return self.analyze_duplicate_handling()

    def analyze_duplicate_handling(self):
        """Analyze how duplicates were handled"""
        duplicate_results = [r for r in self.results if "dup_" in str(r["request_id"])]

        if not duplicate_results:
            return {"error": "No duplicate results found"}

        successful_duplicates = [r for r in duplicate_results if r["success"]]

        # Check for consistent responses to identical requests
        response_patterns = {}
        for result in successful_duplicates:
            pattern_key = (result["status_code"], len(result.get("response_data", "")))
            if pattern_key not in response_patterns:
                response_patterns[pattern_key] = []
            response_patterns[pattern_key].append(result["request_id"])

        analysis = {
            "total_duplicate_requests": len(duplicate_results),
            "successful_duplicate_requests": len(successful_duplicates),
            "duplicate_success_rate": (len(successful_duplicates) / len(duplicate_results)) * 100 if duplicate_results else 0,
            "response_patterns": {str(k): len(v) for k, v in response_patterns.items()},
            "consistent_duplicate_handling": len(response_patterns) <= 2,  # Allow for minor variations
            "unique_thread_count": len(set(r["thread_id"] for r in duplicate_results))
        }

        # Data integrity validation
        analysis["data_integrity_compliance"] = {
            "requirement": "Consistent handling of duplicate concurrent requests",
            "consistent_handling": analysis["consistent_duplicate_handling"],
            "success_rate": analysis["duplicate_success_rate"],
            "pass": (analysis["consistent_duplicate_handling"] and
                    analysis["duplicate_success_rate"] >= 95.0),
            "thread_safety": analysis["unique_thread_count"] > 1  # Confirms true concurrency
        }

        return analysis

    def test_resource_isolation(self, alerts_file):
        """Test that concurrent requests don't interfere with each other"""
        with open(alerts_file, 'r') as f:
            alerts = json.load(f)[:50]  # Test with subset for resource isolation

        print(f"Testing resource isolation with {len(alerts)} concurrent requests")

        # Send requests with timing analysis
        start_time = time.time()

        with ThreadPoolExecutor(max_workers=50) as executor:
            futures = [executor.submit(self.send_tracked_request, alert) for alert in alerts]

            for future in futures:
                future.result()

        end_time = time.time()

        return self.analyze_resource_isolation(start_time, end_time)

    def analyze_resource_isolation(self, start_time, end_time):
        """Analyze resource isolation during concurrent processing"""
        isolation_results = [r for r in self.results if r["timestamp"] >= start_time]

        if not isolation_results:
            return {"error": "No isolation test results found"}

        successful_results = [r for r in isolation_results if r["success"]]

        # Check for resource contention indicators
        response_times = [r["response_time"] for r in successful_results if r["response_time"] > 0]

        if not response_times:
            return {"error": "No valid response times for analysis"}

        # Statistical analysis of response times (looking for resource contention)
        response_times.sort()
        median_response = response_times[len(response_times) // 2]

        # Check for outliers (potential resource contention)
        outlier_threshold = median_response * 3  # 3x median as outlier threshold
        outliers = [rt for rt in response_times if rt > outlier_threshold]
        outlier_rate = (len(outliers) / len(response_times)) * 100

        analysis = {
            "total_isolation_requests": len(isolation_results),
            "successful_isolation_requests": len(successful_results),
            "median_response_time": median_response,
            "max_response_time": max(response_times),
            "min_response_time": min(response_times),
            "outlier_count": len(outliers),
            "outlier_rate": outlier_rate,
            "good_resource_isolation": outlier_rate < 10.0  # Less than 10% outliers
        }

        # Resource isolation validation
        analysis["resource_isolation_compliance"] = {
            "requirement": "No significant resource contention during concurrent processing",
            "outlier_rate": outlier_rate,
            "good_isolation": analysis["good_resource_isolation"],
            "pass": (analysis["good_resource_isolation"] and
                    len(successful_results) >= len(isolation_results) * 0.95),
            "max_response_time": analysis["max_response_time"]
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = DataIntegrityTest(webhook_url)

    # Test duplicate processing
    print("=== Testing Duplicate Processing ===")
    duplicate_results = tester.test_duplicate_processing(f"results/{test_session}/duplicate_alerts.json")

    # Test resource isolation
    print("\n=== Testing Resource Isolation ===")
    isolation_results = tester.test_resource_isolation(f"results/{test_session}/concurrent_alerts.json")

    # Combine results
    combined_results = {
        "duplicate_processing": duplicate_results,
        "resource_isolation": isolation_results
    }

    # Save results
    with open(f"results/{test_session}/data_integrity_results.json", "w") as f:
        json.dump(combined_results, f, indent=2)

    # Print summary
    print(f"\n=== Data Integrity Test Results ===")

    # Duplicate processing summary
    dup_compliance = duplicate_results["data_integrity_compliance"]
    print(f"Duplicate Processing: {'✅ PASS' if dup_compliance['pass'] else '❌ FAIL'}")
    print(f"  Consistent Handling: {dup_compliance['consistent_handling']}")
    print(f"  Success Rate: {dup_compliance['success_rate']:.1f}%")
    print(f"  Thread Safety: {dup_compliance['thread_safety']}")

    # Resource isolation summary
    iso_compliance = isolation_results["resource_isolation_compliance"]
    print(f"Resource Isolation: {'✅ PASS' if iso_compliance['pass'] else '❌ FAIL'}")
    print(f"  Good Isolation: {iso_compliance['good_isolation']}")
    print(f"  Outlier Rate: {iso_compliance['outlier_rate']:.1f}%")
    print(f"  Max Response Time: {iso_compliance['max_response_time']:.3f}s")