#!/usr/bin/env python3
"""
Recommendation Relevance and Accuracy Test
Tests the relevance and accuracy of remediation recommendations
"""
import json
import requests
import time
import sys
import os
sys.path.append(os.path.dirname(os.path.abspath(__file__)))
from remediation_analyzer import RemediationAnalyzer

class RelevanceAccuracyTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.analyzer = RemediationAnalyzer()

    def create_validation_scenarios(self):
        """Create scenarios with known correct remediation approaches"""
        return [
            {
                "scenario_name": "memory_limit_exceeded",
                "alert": {
                    "alerts": [{
                        "status": "firing",
                        "labels": {
                            "alertname": "ContainerMemoryExceeded",
                            "severity": "critical",
                            "namespace": "web-services",
                            "pod": "api-server-abc123",
                            "container": "api-container"
                        },
                        "annotations": {
                            "description": "Container memory usage exceeded limit",
                            "current_usage": "2.1GB",
                            "limit": "2GB"
                        }
                    }]
                },
                "expected_approaches": [
                    "increase memory limit",
                    "check for memory leaks",
                    "restart container",
                    "examine application logs",
                    "kubectl patch"
                ],
                "incorrect_approaches": [
                    "increase cpu limit",
                    "delete namespace",
                    "reboot node",
                    "modify network policy"
                ],
                "must_include_context": ["api-server-abc123", "web-services", "2GB", "memory"]
            },
            {
                "scenario_name": "pod_crashloop_backoff",
                "alert": {
                    "alerts": [{
                        "status": "firing",
                        "labels": {
                            "alertname": "PodCrashLoopBackOff",
                            "severity": "warning",
                            "namespace": "database",
                            "pod": "postgres-primary-xyz789",
                            "container": "postgres"
                        },
                        "annotations": {
                            "description": "Pod postgres-primary-xyz789 is in CrashLoopBackOff state",
                            "restart_count": "15"
                        }
                    }]
                },
                "expected_approaches": [
                    "check pod logs",
                    "examine events",
                    "verify configuration",
                    "check resource limits",
                    "kubectl describe pod"
                ],
                "incorrect_approaches": [
                    "scale deployment",
                    "update ingress",
                    "modify service account",
                    "change storage class"
                ],
                "must_include_context": ["postgres-primary-xyz789", "database", "CrashLoopBackOff", "logs"]
            },
            {
                "scenario_name": "service_endpoint_unavailable",
                "alert": {
                    "alerts": [{
                        "status": "firing",
                        "labels": {
                            "alertname": "ServiceEndpointsUnavailable",
                            "severity": "critical",
                            "namespace": "api-gateway",
                            "service": "auth-service",
                            "endpoint": "auth-api-endpoint"
                        },
                        "annotations": {
                            "description": "Service auth-service has no available endpoints",
                            "available_endpoints": "0",
                            "expected_endpoints": "3"
                        }
                    }]
                },
                "expected_approaches": [
                    "check pod status",
                    "verify service selector",
                    "examine endpoint configuration",
                    "check pod readiness",
                    "kubectl get endpoints"
                ],
                "incorrect_approaches": [
                    "update ingress controller",
                    "modify network policies only",
                    "change service type",
                    "restart entire cluster"
                ],
                "must_include_context": ["auth-service", "api-gateway", "endpoints", "0"]
            }
        ]

    def test_recommendation_accuracy(self):
        """Test accuracy of recommendations against expected approaches"""
        validation_scenarios = self.create_validation_scenarios()

        print(f"Testing recommendation accuracy for {len(validation_scenarios)} validation scenarios...")

        for scenario in validation_scenarios:
            print(f"\nTesting: {scenario['scenario_name']}")
            result = self.test_single_accuracy_scenario(scenario)
            self.results.append(result)

            time.sleep(3)

        return self.analyze_accuracy_results()

    def test_single_accuracy_scenario(self, scenario):
        """Test accuracy for a single scenario"""
        try:
            start_time = time.time()

            # Get recommendation
            response = requests.post(
                self.webhook_url,
                json=scenario["alert"],
                headers={
                    'Content-Type': 'application/json',
                    'X-Accuracy-Test': 'true',
                    'X-Scenario': scenario["scenario_name"]
                },
                timeout=30
            )

            end_time = time.time()

            if response.status_code != 200:
                return {
                    "scenario_name": scenario["scenario_name"],
                    "success": False,
                    "error": f"HTTP {response.status_code}",
                    "timestamp": start_time
                }

            recommendation_text = response.text.lower()

            # Analyze accuracy
            accuracy_analysis = self._analyze_recommendation_accuracy(
                recommendation_text, scenario
            )

            result = {
                "scenario_name": scenario["scenario_name"],
                "success": True,
                "response_time": end_time - start_time,
                "recommendation_text": response.text[:1500],
                "accuracy_analysis": accuracy_analysis,
                "timestamp": start_time
            }

            # Print result
            acc_score = accuracy_analysis["accuracy_score"]
            print(f"  Accuracy Score: {acc_score:.3f} ({accuracy_analysis['accuracy_rating']})")
            print(f"  Expected Approaches Found: {accuracy_analysis['expected_approaches_found']}/{accuracy_analysis['total_expected_approaches']}")
            print(f"  Context Usage: {accuracy_analysis['context_usage_score']:.3f}")

            return result

        except Exception as e:
            return {
                "scenario_name": scenario["scenario_name"],
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def _analyze_recommendation_accuracy(self, recommendation_text, scenario):
        """Analyze accuracy of recommendation against expected approaches"""
        # Check expected approaches
        expected_found = 0
        expected_details = []

        for approach in scenario["expected_approaches"]:
            if approach.lower() in recommendation_text:
                expected_found += 1
                expected_details.append(approach)

        # Check incorrect approaches (should NOT be present)
        incorrect_found = 0
        incorrect_details = []

        for incorrect in scenario["incorrect_approaches"]:
            if incorrect.lower() in recommendation_text:
                incorrect_found += 1
                incorrect_details.append(incorrect)

        # Check context usage
        context_found = 0
        context_details = []

        for context_item in scenario["must_include_context"]:
            if str(context_item).lower() in recommendation_text:
                context_found += 1
                context_details.append(context_item)

        # Calculate scores
        expected_score = expected_found / len(scenario["expected_approaches"]) if scenario["expected_approaches"] else 0
        context_score = context_found / len(scenario["must_include_context"]) if scenario["must_include_context"] else 0
        incorrect_penalty = min(0.5, incorrect_found * 0.1)  # Penalty for incorrect approaches

        accuracy_score = max(0, (expected_score * 0.6 + context_score * 0.4) - incorrect_penalty)

        return {
            "expected_approaches_found": expected_found,
            "total_expected_approaches": len(scenario["expected_approaches"]),
            "expected_approaches_score": expected_score,
            "expected_approaches_details": expected_details,
            "incorrect_approaches_found": incorrect_found,
            "incorrect_approaches_details": incorrect_details,
            "context_items_found": context_found,
            "total_context_items": len(scenario["must_include_context"]),
            "context_usage_score": context_score,
            "context_details": context_details,
            "accuracy_score": accuracy_score,
            "accuracy_rating": self._get_accuracy_rating(accuracy_score),
            "has_incorrect_suggestions": incorrect_found > 0
        }

    def _get_accuracy_rating(self, score):
        """Convert accuracy score to rating"""
        if score >= 0.85:
            return "excellent"
        elif score >= 0.7:
            return "good"
        elif score >= 0.5:
            return "adequate"
        elif score >= 0.3:
            return "poor"
        else:
            return "unacceptable"

    def analyze_accuracy_results(self):
        """Analyze overall accuracy results"""
        successful_results = [r for r in self.results if r["success"]]

        if not successful_results:
            return {"error": "No successful accuracy tests"}

        # Extract accuracy data
        accuracy_analyses = [r["accuracy_analysis"] for r in successful_results]
        accuracy_scores = [aa["accuracy_score"] for aa in accuracy_analyses]
        accuracy_ratings = [aa["accuracy_rating"] for aa in accuracy_analyses]

        # Count ratings
        rating_counts = {}
        for rating in accuracy_ratings:
            rating_counts[rating] = rating_counts.get(rating, 0) + 1

        # Calculate statistics
        analysis = {
            "total_accuracy_tests": len(self.results),
            "successful_tests": len(successful_results),
            "success_rate": (len(successful_results) / len(self.results) * 100) if self.results else 0,
            "average_accuracy_score": sum(accuracy_scores) / len(accuracy_scores) if accuracy_scores else 0,
            "max_accuracy_score": max(accuracy_scores) if accuracy_scores else 0,
            "min_accuracy_score": min(accuracy_scores) if accuracy_scores else 0,
            "accuracy_rating_distribution": rating_counts,
            "high_accuracy_count": rating_counts.get("excellent", 0) + rating_counts.get("good", 0),
            "recommendations_with_incorrect_suggestions": sum(1 for aa in accuracy_analyses if aa["has_incorrect_suggestions"])
        }

        # Business requirement validation
        high_accuracy_rate = (analysis["high_accuracy_count"] / len(accuracy_analyses) * 100) if accuracy_analyses else 0

        analysis["br_pa_007_accuracy_compliance"] = {
            "requirement": "Recommendations must be relevant and accurate for specific alert contexts",
            "average_accuracy": analysis["average_accuracy_score"],
            "high_accuracy_rate": high_accuracy_rate,
            "incorrect_suggestions_present": analysis["recommendations_with_incorrect_suggestions"] > 0,
            "pass": (analysis["average_accuracy_score"] >= 0.7 and
                    high_accuracy_rate >= 70.0 and
                    analysis["recommendations_with_incorrect_suggestions"] <= len(accuracy_analyses) * 0.2),
            "accuracy_quality": "excellent" if high_accuracy_rate >= 80 else "good" if high_accuracy_rate >= 60 else "needs_improvement"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = RelevanceAccuracyTest(webhook_url)
    results = tester.test_recommendation_accuracy()

    # Save results
    with open(f"results/{test_session}/relevance_accuracy_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    if "error" not in results:
        print(f"\n=== Recommendation Relevance and Accuracy Results ===")
        print(f"Total Tests: {results['total_accuracy_tests']}")
        print(f"Successful: {results['successful_tests']}")
        print(f"Average Accuracy Score: {results['average_accuracy_score']:.3f}")
        print(f"High Accuracy Rate: {results['high_accuracy_count']}/{len([r for r in results if r['success']])} recommendations")
        print(f"Incorrect Suggestions: {results['recommendations_with_incorrect_suggestions']} recommendations")

        print(f"\nAccuracy Rating Distribution:")
        for rating, count in results['accuracy_rating_distribution'].items():
            print(f"  {rating}: {count}")

        compliance = results["br_pa_007_accuracy_compliance"]
        print(f"\n=== Accuracy Compliance ===")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
        print(f"Accuracy Quality: {compliance['accuracy_quality']}")
    else:
        print(f"❌ Error: {results['error']}")