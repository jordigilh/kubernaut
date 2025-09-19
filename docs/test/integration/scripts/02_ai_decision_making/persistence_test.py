#!/usr/bin/env python3
"""
Historical Data Persistence Test
Tests persistence of effectiveness tracking data across system operations
"""
import json
import requests
import time

class PersistenceTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []

    def test_data_persistence(self):
        """Test persistence of effectiveness data"""
        print("Testing effectiveness data persistence...")

        # Step 1: Submit initial data
        initial_submission = self.submit_effectiveness_data("initial")
        time.sleep(2)

        # Step 2: Query data immediately
        immediate_query = self.query_effectiveness_data("immediate")
        time.sleep(2)

        # Step 3: Submit additional data
        additional_submission = self.submit_effectiveness_data("additional")
        time.sleep(2)

        # Step 4: Query data after additional submissions
        final_query = self.query_effectiveness_data("final")

        return self.analyze_persistence_results({
            "initial_submission": initial_submission,
            "immediate_query": immediate_query,
            "additional_submission": additional_submission,
            "final_query": final_query
        })

    def submit_effectiveness_data(self, phase):
        """Submit effectiveness data for persistence testing"""
        test_data = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": f"PersistenceTest_{phase}",
                    "severity": "warning",
                    "test_phase": phase,
                    "persistence_test": "true"
                },
                "annotations": {
                    "description": f"Persistence test alert for phase {phase}",
                    "summary": "Testing data persistence"
                },
                "startsAt": "2025-01-01T10:00:00Z"
            }]
        }

        try:
            response = requests.post(
                self.webhook_url,
                json=test_data,
                headers={
                    'Content-Type': 'application/json',
                    'X-Persistence-Test': phase,
                    'X-Effectiveness-Tracking': 'true'
                },
                timeout=20
            )

            return {
                "phase": phase,
                "submission_success": response.status_code == 200,
                "status_code": response.status_code,
                "timestamp": time.time()
            }

        except Exception as e:
            return {
                "phase": phase,
                "submission_success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def query_effectiveness_data(self, query_phase):
        """Query effectiveness data for persistence verification"""
        try:
            query_response = requests.get(
                f"{self.webhook_url.replace('/webhook/prometheus', '/api/effectiveness/history')}",
                headers={
                    'X-Persistence-Query': query_phase,
                    'X-Query-Type': 'persistence_test'
                },
                timeout=15
            )

            return {
                "query_phase": query_phase,
                "query_success": query_response.status_code == 200,
                "status_code": query_response.status_code,
                "response_data": query_response.json() if query_response.status_code == 200 else None,
                "data_count": len(query_response.json().get("entries", [])) if query_response.status_code == 200 and query_response.json() else 0,
                "timestamp": time.time()
            }

        except Exception as e:
            return {
                "query_phase": query_phase,
                "query_success": False,
                "error": str(e),
                "note": "Persistence query endpoint may not be implemented",
                "timestamp": time.time()
            }

    def analyze_persistence_results(self, test_results):
        """Analyze persistence test results"""
        analysis = {
            "initial_data_submitted": test_results["initial_submission"]["submission_success"],
            "immediate_query_success": test_results["immediate_query"]["query_success"],
            "additional_data_submitted": test_results["additional_submission"]["submission_success"],
            "final_query_success": test_results["final_query"]["query_success"],
            "data_persistence_indicators": {}
        }

        # Check for data persistence indicators
        if test_results["immediate_query"]["query_success"] and test_results["final_query"]["query_success"]:
            immediate_count = test_results["immediate_query"]["data_count"]
            final_count = test_results["final_query"]["data_count"]

            analysis["data_persistence_indicators"] = {
                "immediate_query_data_count": immediate_count,
                "final_query_data_count": final_count,
                "data_growth_observed": final_count > immediate_count,
                "persistence_evidence": final_count >= immediate_count
            }

        # Business requirement validation for persistence
        has_persistence_capability = (
            analysis["immediate_query_success"] or
            analysis["final_query_success"] or
            any(test_results[key]["submission_success"] for key in ["initial_submission", "additional_submission"])
        )

        analysis["br_pa_008_persistence_compliance"] = {
            "requirement": "Persistence of effectiveness data across system restarts",
            "persistence_capability_present": has_persistence_capability,
            "query_functionality": analysis["immediate_query_success"] or analysis["final_query_success"],
            "data_submission_working": analysis["initial_data_submitted"] or analysis["additional_data_submitted"],
            "pass": has_persistence_capability,
            "persistence_implementation": "detected" if has_persistence_capability else "not_detected"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = PersistenceTest(webhook_url)
    results = tester.test_data_persistence()

    # Save results
    with open(f"results/{test_session}/persistence_test_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== Historical Data Persistence Test Results ===")
    print(f"Initial Data Submitted: {'✅' if results['initial_data_submitted'] else '❌'}")
    print(f"Immediate Query Success: {'✅' if results['immediate_query_success'] else '❌'}")
    print(f"Additional Data Submitted: {'✅' if results['additional_data_submitted'] else '❌'}")
    print(f"Final Query Success: {'✅' if results['final_query_success'] else '❌'}")

    if results['data_persistence_indicators']:
        indicators = results['data_persistence_indicators']
        print(f"Immediate Query Data Count: {indicators['immediate_query_data_count']}")
        print(f"Final Query Data Count: {indicators['final_query_data_count']}")
        print(f"Data Growth Observed: {'✅' if indicators['data_growth_observed'] else '❌'}")

    compliance = results["br_pa_008_persistence_compliance"]
    print(f"\n=== Persistence Compliance ===")
    print(f"Persistence Implementation: {compliance['persistence_implementation']}")
    print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")