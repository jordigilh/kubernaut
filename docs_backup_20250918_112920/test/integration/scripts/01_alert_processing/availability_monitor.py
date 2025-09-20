#!/usr/bin/env python3
"""
Continuous Availability Monitor for BR-PA-001
Monitors webhook endpoint availability with precise timing and statistics
"""
import json
import time
import requests
import threading
import statistics
from datetime import datetime, timedelta
from concurrent.futures import ThreadPoolExecutor
import queue

class AvailabilityMonitor:
    def __init__(self, webhook_url, check_interval=5):
        self.webhook_url = webhook_url
        self.check_interval = check_interval  # seconds between checks
        self.results = []
        self.results_lock = threading.Lock()
        self.running = False
        self.start_time = None

    def single_availability_check(self, check_id):
        """Perform a single availability check"""
        check_start = time.time()

        # Lightweight health check alert
        health_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "AvailabilityCheck",
                    "severity": "info",
                    "check_id": str(check_id)
                },
                "annotations": {
                    "description": f"Availability check {check_id}",
                    "summary": "Automated availability monitoring"
                },
                "startsAt": datetime.now().isoformat()
            }]
        }

        try:
            response = requests.post(
                self.webhook_url,
                json=health_alert,
                headers={'Content-Type': 'application/json'},
                timeout=10  # 10 second timeout for availability check
            )

            check_end = time.time()
            response_time = check_end - check_start

            result = {
                "check_id": check_id,
                "timestamp": check_start,
                "response_time": response_time,
                "status_code": response.status_code,
                "available": response.status_code == 200,
                "error": None
            }

        except requests.exceptions.Timeout:
            result = {
                "check_id": check_id,
                "timestamp": check_start,
                "response_time": 10.0,  # timeout value
                "status_code": 408,
                "available": False,
                "error": "timeout"
            }
        except Exception as e:
            result = {
                "check_id": check_id,
                "timestamp": check_start,
                "response_time": -1,
                "status_code": -1,
                "available": False,
                "error": str(e)
            }

        with self.results_lock:
            self.results.append(result)

        return result

    def run_availability_monitoring(self, duration_minutes):
        """Run continuous availability monitoring"""
        print(f"Starting availability monitoring for {duration_minutes} minutes")
        print(f"Check interval: {self.check_interval} seconds")
        print(f"Target availability: 99.9%")
        print(f"Maximum allowed downtime: {duration_minutes * 60 * 0.001:.1f} seconds")

        self.start_time = time.time()
        end_time = self.start_time + (duration_minutes * 60)
        check_id = 0

        self.running = True

        while time.time() < end_time and self.running:
            # Perform availability check
            self.single_availability_check(check_id)
            check_id += 1

            # Progress reporting every 60 checks (5 minutes at default interval)
            if check_id % 60 == 0:
                self.print_progress_report()

            # Wait for next check
            time.sleep(self.check_interval)

        self.running = False
        return self.calculate_availability_statistics()

    def print_progress_report(self):
        """Print interim availability statistics"""
        with self.results_lock:
            if not self.results:
                return

            recent_results = self.results[-60:]  # Last 60 checks (5 minutes)
            available_count = sum(1 for r in recent_results if r["available"])
            recent_availability = (available_count / len(recent_results)) * 100

            elapsed_time = time.time() - self.start_time
            total_available = sum(1 for r in self.results if r["available"])
            overall_availability = (total_available / len(self.results)) * 100

            print(f"Progress: {len(self.results)} checks, "
                  f"{elapsed_time/60:.1f}min elapsed, "
                  f"Recent 5min: {recent_availability:.2f}%, "
                  f"Overall: {overall_availability:.3f}%")

    def calculate_availability_statistics(self):
        """Calculate comprehensive availability statistics"""
        if not self.results:
            return {"error": "No availability data collected"}

        # Basic availability calculation
        total_checks = len(self.results)
        available_checks = sum(1 for r in self.results if r["available"])
        availability_percentage = (available_checks / total_checks) * 100

        # Downtime calculation
        total_duration = self.results[-1]["timestamp"] - self.results[0]["timestamp"]
        downtime_seconds = 0

        # Calculate continuous downtime periods
        downtime_periods = []
        current_downtime_start = None

        for result in self.results:
            if not result["available"] and current_downtime_start is None:
                current_downtime_start = result["timestamp"]
            elif result["available"] and current_downtime_start is not None:
                downtime_duration = result["timestamp"] - current_downtime_start
                downtime_periods.append({
                    "start": current_downtime_start,
                    "duration": downtime_duration
                })
                downtime_seconds += downtime_duration
                current_downtime_start = None

        # Handle ongoing downtime at end
        if current_downtime_start is not None:
            downtime_duration = self.results[-1]["timestamp"] - current_downtime_start
            downtime_periods.append({
                "start": current_downtime_start,
                "duration": downtime_duration
            })
            downtime_seconds += downtime_duration

        # Response time statistics for available checks
        available_response_times = [r["response_time"] for r in self.results if r["available"]]

        statistics_result = {
            "test_duration_minutes": total_duration / 60,
            "total_checks": total_checks,
            "available_checks": available_checks,
            "availability_percentage": availability_percentage,
            "downtime_seconds": downtime_seconds,
            "downtime_periods": downtime_periods,
            "uptime_percentage": 100 - (downtime_seconds / total_duration * 100),
            "mean_response_time": statistics.mean(available_response_times) if available_response_times else 0,
            "max_response_time": max(available_response_times) if available_response_times else 0,
            "error_types": {}
        }

        # Error analysis
        error_counts = {}
        for result in self.results:
            if not result["available"]:
                error_type = result.get("error", "http_error")
                error_counts[error_type] = error_counts.get(error_type, 0) + 1

        statistics_result["error_types"] = error_counts

        # Business requirement validation
        statistics_result["br_pa_001_compliance"] = {
            "requirement": "99.9% availability",
            "measured_availability": availability_percentage,
            "measured_uptime": statistics_result["uptime_percentage"],
            "pass": availability_percentage >= 99.9,
            "margin": availability_percentage - 99.9,
            "max_allowed_downtime": total_duration * 0.001,
            "actual_downtime": downtime_seconds
        }

        return statistics_result

    def stop_monitoring(self):
        """Stop availability monitoring"""
        self.running = False

if __name__ == "__main__":
    import sys
    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    duration = int(sys.argv[2]) if len(sys.argv) > 2 else 60  # default 60 minutes
    test_session = sys.argv[3] if len(sys.argv) > 3 else "test_session"

    monitor = AvailabilityMonitor(webhook_url, check_interval=5)
    results = monitor.run_availability_monitoring(duration)

    # Save detailed results
    with open(f"results/{test_session}/availability_detailed_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== BR-PA-001 Availability Test Results ===")
    print(f"Test Duration: {results['test_duration_minutes']:.1f} minutes")
    print(f"Total Checks: {results['total_checks']}")
    print(f"Available Checks: {results['available_checks']}")
    print(f"Availability: {results['availability_percentage']:.4f}%")
    print(f"Uptime: {results['uptime_percentage']:.4f}%")
    print(f"Total Downtime: {results['downtime_seconds']:.2f} seconds")
    print(f"Mean Response Time: {results['mean_response_time']:.3f} seconds")

    if results['downtime_periods']:
        print(f"Downtime Incidents: {len(results['downtime_periods'])}")
        for i, period in enumerate(results['downtime_periods'], 1):
            print(f"  Incident {i}: {period['duration']:.2f} seconds")

    print(f"\n=== BR-PA-001 Compliance ===")
    compliance = results["br_pa_001_compliance"]
    print(f"Requirement: {compliance['requirement']}")
    print(f"Measured Availability: {compliance['measured_availability']:.4f}%")
    print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    if compliance['pass']:
        print(f"Margin: {compliance['margin']:.4f}% above requirement")
    else:
        print(f"Shortfall: {-compliance['margin']:.4f}% below requirement")

    print(f"Max Allowed Downtime: {compliance['max_allowed_downtime']:.2f} seconds")
    print(f"Actual Downtime: {compliance['actual_downtime']:.2f} seconds")