#!/usr/bin/env python3
"""
Sequential Source Processing Test
Verifies that alerts from the same source maintain processing order
"""
import json
import requests
import time
import threading
from collections import defaultdict

class SequentialProcessingTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.results_lock = threading.Lock()

    def send_alert_with_tracking(self, alert_data):
        """Send alert with detailed tracking for order analysis"""
        try:
            start_time = time.time()

            # Add tracking headers
            headers = {
                'Content-Type': 'application/json',
                'X-Source-ID': alert_data['source_id'],
                'X-Sequence-Number': str(alert_data['sequence_number']),
                'X-Expected-Order': str(alert_data['expected_order'])
            }

            response = requests.post(
                self.webhook_url,
                json=alert_data["payload"],
                headers=headers,
                timeout=15
            )

            end_time = time.time()

            result = {
                "source_id": alert_data["source_id"],
                "sequence_number": alert_data["sequence_number"],
                "expected_order": alert_data["expected_order"],
                "status_code": response.status_code,
                "success": response.status_code == 200,
                "start_time": start_time,
                "end_time": end_time,
                "response_time": end_time - start_time,
                "sent_timestamp": alert_data["payload"]["alerts"][0]["startsAt"],
                "thread_id": threading.get_ident()
            }

            with self.results_lock:
                self.results.append(result)

            return result

        except Exception as e:
            error_result = {
                "source_id": alert_data["source_id"],
                "sequence_number": alert_data["sequence_number"],
                "expected_order": alert_data["expected_order"],
                "status_code": -1,
                "success": False,
                "error": str(e),
                "start_time": time.time(),
                "end_time": time.time()
            }

            with self.results_lock:
                self.results.append(error_result)

            return error_result

    def test_interleaved_sequences(self, interleaved_file):
        """Test processing of interleaved sequences from multiple sources"""
        with open(interleaved_file, 'r') as f:
            alerts = json.load(f)

        print(f"Testing interleaved sequences with {len(alerts)} alerts from multiple sources")

        # Send alerts in chronological order (as they would arrive naturally)
        for alert in alerts:
            result = self.send_alert_with_tracking(alert)
            print(f"Sent: Source {result['source_id']}, Seq {result['sequence_number']}")

            # Small delay to simulate realistic arrival timing
            time.sleep(0.5)

        return self.analyze_ordering_results()

    def analyze_ordering_results(self):
        """Analyze results for ordering compliance"""
        if not self.results:
            return {"error": "No results to analyze"}

        # Group results by source
        source_results = defaultdict(list)
        for result in self.results:
            if result["success"]:
                source_results[result["source_id"]].append(result)

        # Sort each source's results by processing completion time
        for source_id in source_results:
            source_results[source_id].sort(key=lambda x: x["end_time"])

        # Check ordering compliance for each source
        ordering_analysis = {}
        total_sequences = 0
        correctly_ordered_sequences = 0

        for source_id, results in source_results.items():
            # Check if sequence numbers are in ascending order
            sequence_numbers = [r["sequence_number"] for r in results]
            expected_sequence = sorted(sequence_numbers)

            is_correctly_ordered = sequence_numbers == expected_sequence

            # Calculate ordering violations
            violations = []
            for i in range(1, len(sequence_numbers)):
                if sequence_numbers[i] < sequence_numbers[i-1]:
                    violations.append({
                        "position": i,
                        "expected": sequence_numbers[i-1] + 1,
                        "actual": sequence_numbers[i]
                    })

            ordering_analysis[source_id] = {
                "total_alerts": len(results),
                "correctly_ordered": is_correctly_ordered,
                "sequence_received": sequence_numbers,
                "sequence_expected": expected_sequence,
                "violations": violations,
                "violation_count": len(violations)
            }

            total_sequences += 1
            if is_correctly_ordered:
                correctly_ordered_sequences += 1

        # Overall analysis
        overall_analysis = {
            "total_sources": len(source_results),
            "correctly_ordered_sources": correctly_ordered_sequences,
            "ordering_compliance_rate": (correctly_ordered_sequences / total_sequences * 100) if total_sequences > 0 else 0,
            "source_analysis": dict(ordering_analysis),
            "total_alerts_processed": len([r for r in self.results if r["success"]]),
            "failed_alerts": len([r for r in self.results if not r["success"]])
        }

        # Business requirement validation
        overall_analysis["br_pa_005_compliance"] = {
            "requirement": "Maintain alert processing order for the same alert source",
            "compliance_rate": overall_analysis["ordering_compliance_rate"],
            "pass": overall_analysis["ordering_compliance_rate"] >= 100.0,  # Must be perfect
            "sources_with_violations": [
                source_id for source_id, analysis in ordering_analysis.items()
                if not analysis["correctly_ordered"]
            ]
        }

        return overall_analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = SequentialProcessingTest(webhook_url)
    results = tester.test_interleaved_sequences(f"results/{test_session}/interleaved_sequences.json")

    # Save results
    with open(f"results/{test_session}/sequential_processing_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== Sequential Processing Test Results ===")
    print(f"Total Sources: {results['total_sources']}")
    print(f"Correctly Ordered Sources: {results['correctly_ordered_sources']}")
    print(f"Ordering Compliance Rate: {results['ordering_compliance_rate']:.1f}%")
    print(f"Total Alerts Processed: {results['total_alerts_processed']}")
    print(f"Failed Alerts: {results['failed_alerts']}")

    # Show per-source analysis
    print(f"\nPer-Source Analysis:")
    for source_id, analysis in results['source_analysis'].items():
        print(f"  {source_id}: {'✅ ORDERED' if analysis['correctly_ordered'] else '❌ VIOLATIONS'}")
        if analysis['violations']:
            print(f"    Violations: {analysis['violation_count']}")

    compliance = results["br_pa_005_compliance"]
    print(f"\n=== BR-PA-005 Compliance ===")
    print(f"Requirement: {compliance['requirement']}")
    print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")

    if not compliance['pass']:
        print(f"Sources with violations: {compliance['sources_with_violations']}")