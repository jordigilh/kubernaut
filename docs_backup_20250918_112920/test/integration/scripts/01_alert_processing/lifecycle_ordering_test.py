#!/usr/bin/env python3
"""
Alert Lifecycle Ordering Test
Tests proper sequencing of alert lifecycle stages (firing → resolved)
"""
import json
import requests
import time

class LifecycleOrderingTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []

    def send_lifecycle_alert(self, alert_data):
        """Send lifecycle alert with stage tracking"""
        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=alert_data["payload"],
                headers={
                    'Content-Type': 'application/json',
                    'X-Lifecycle-Stage': alert_data['lifecycle_stage'],
                    'X-Lifecycle-Sequence': str(alert_data['sequence_number'])
                },
                timeout=15
            )

            end_time = time.time()

            result = {
                "sequence_number": alert_data["sequence_number"],
                "lifecycle_stage": alert_data["lifecycle_stage"],
                "source_id": alert_data["source_id"],
                "status_code": response.status_code,
                "success": response.status_code == 200,
                "processing_time": end_time - start_time,
                "timestamp": start_time,
                "alert_status": alert_data["payload"]["alerts"][0]["status"],
                "alert_severity": alert_data["payload"]["alerts"][0]["labels"]["severity"]
            }

            self.results.append(result)
            return result

        except Exception as e:
            error_result = {
                "sequence_number": alert_data["sequence_number"],
                "lifecycle_stage": alert_data["lifecycle_stage"],
                "source_id": alert_data["source_id"],
                "status_code": -1,
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

            self.results.append(error_result)
            return error_result

    def test_lifecycle_sequence(self, lifecycle_file):
        """Test alert lifecycle sequence ordering"""
        with open(lifecycle_file, 'r') as f:
            alerts = json.load(f)

        print(f"Testing lifecycle sequence with {len(alerts)} stages")

        # Send lifecycle alerts in sequence with appropriate delays
        for i, alert in enumerate(alerts):
            result = self.send_lifecycle_alert(alert)
            stage = result['lifecycle_stage']
            status = '✓' if result['success'] else '✗'

            print(f"Stage {i} ({stage}): {status}")

            # Delay between lifecycle stages to simulate realistic timing
            if i < len(alerts) - 1:  # No delay after last alert
                time.sleep(2)

        return self.analyze_lifecycle_results()

    def analyze_lifecycle_results(self):
        """Analyze lifecycle ordering results"""
        successful_results = [r for r in self.results if r["success"]]

        if not successful_results:
            return {"error": "No successful lifecycle results"}

        # Expected lifecycle progression
        expected_stages = ["firing", "firing", "firing", "resolved"]  # Based on test data
        actual_stages = [r["lifecycle_stage"] for r in successful_results]

        # Check lifecycle progression validity
        valid_progression = True
        progression_issues = []

        # Rule: "resolved" should come after "firing"
        firing_seen = False
        for i, stage in enumerate(actual_stages):
            if stage == "firing":
                firing_seen = True
            elif stage == "resolved":
                if not firing_seen:
                    valid_progression = False
                    progression_issues.append({
                        "position": i,
                        "issue": "resolved_before_firing",
                        "stage": stage
                    })

        # Check for proper sequence order
        sequence_numbers = [r["sequence_number"] for r in successful_results]
        correct_sequence = sequence_numbers == sorted(sequence_numbers)

        # Analyze stage transitions
        stage_transitions = []
        for i in range(1, len(actual_stages)):
            transition = f"{actual_stages[i-1]} → {actual_stages[i]}"
            stage_transitions.append(transition)

        analysis = {
            "total_lifecycle_alerts": len(self.results),
            "successful_alerts": len(successful_results),
            "success_rate": (len(successful_results) / len(self.results) * 100) if self.results else 0,
            "expected_stages": expected_stages,
            "actual_stages": actual_stages,
            "valid_lifecycle_progression": valid_progression,
            "progression_issues": progression_issues,
            "correct_sequence_order": correct_sequence,
            "stage_transitions": stage_transitions,
            "final_stage": actual_stages[-1] if actual_stages else None,
            "lifecycle_complete": actual_stages[-1] == "resolved" if actual_stages else False
        }

        # Business requirement validation for lifecycle
        analysis["br_pa_005_lifecycle_compliance"] = {
            "requirement": "Maintain proper alert lifecycle stage ordering",
            "valid_progression": analysis["valid_lifecycle_progression"],
            "correct_sequence": analysis["correct_sequence_order"],
            "lifecycle_complete": analysis["lifecycle_complete"],
            "success_rate": analysis["success_rate"],
            "pass": (analysis["valid_lifecycle_progression"] and
                    analysis["correct_sequence_order"] and
                    analysis["success_rate"] >= 95.0),
            "progression_quality": "excellent" if len(progression_issues) == 0 else "needs_review"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = LifecycleOrderingTest(webhook_url)
    results = tester.test_lifecycle_sequence(f"results/{test_session}/lifecycle_sequence.json")

    # Save results
    with open(f"results/{test_session}/lifecycle_ordering_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== Lifecycle Ordering Test Results ===")
    print(f"Total Lifecycle Alerts: {results['total_lifecycle_alerts']}")
    print(f"Successful: {results['successful_alerts']}")
    print(f"Success Rate: {results['success_rate']:.1f}%")
    print(f"Valid Lifecycle Progression: {results['valid_lifecycle_progression']}")
    print(f"Correct Sequence Order: {results['correct_sequence_order']}")
    print(f"Lifecycle Complete: {results['lifecycle_complete']}")

    if results['stage_transitions']:
        print(f"Stage Transitions: {' → '.join(results['stage_transitions'])}")

    compliance = results["br_pa_005_lifecycle_compliance"]
    print(f"\n=== Lifecycle Compliance ===")
    print(f"Requirement: {compliance['requirement']}")
    print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    print(f"Progression Quality: {compliance['progression_quality']}")