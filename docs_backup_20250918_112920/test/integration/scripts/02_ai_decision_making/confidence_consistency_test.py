#!/usr/bin/env python3
"""
Confidence Score Consistency Test
Tests consistency of confidence scores across similar alert scenarios
"""
import json
import requests
import time
import sys
import os

sys.path.append(os.path.dirname(os.path.abspath(__file__)))
from confidence_analyzer import ConfidenceAnalyzer

class ConfidenceConsistencyTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.analyzer = ConfidenceAnalyzer()

    def test_confidence_consistency(self):
        """Test confidence score consistency with repeated scenarios"""
        print("Testing confidence score consistency...")

        # Create repeated similar scenarios
        similar_scenarios = self.create_similar_scenarios()

        for scenario_group in similar_scenarios:
            print(f"\nTesting scenario group: {scenario_group['group_name']}")

            group_results = []
            for i, scenario in enumerate(scenario_group['scenarios']):
                print(f"  Running iteration {i+1}...")
                result = self.test_single_consistency_scenario(scenario, scenario_group['group_name'])
                group_results.append(result)

                time.sleep(2)

            # Analyze group consistency
            group_analysis = self.analyze_group_consistency(scenario_group, group_results)

            self.results.append({
                "group_name": scenario_group['group_name'],
                "group_results": group_results,
                "consistency_analysis": group_analysis
            })

        return self.analyze_overall_consistency()

    def create_similar_scenarios(self):
        """Create groups of similar scenarios for consistency testing"""
        return [
            {
                "group_name": "high_memory_usage_pods",
                "expected_confidence_level": "high",
                "scenarios": [
                    {
                        "iteration": 1,
                        "alert": {
                            "alerts": [{
                                "status": "firing",
                                "labels": {
                                    "alertname": "PodMemoryHigh",
                                    "severity": "warning",
                                    "namespace": "test-apps",
                                    "pod": "webapp-pod-001"
                                },
                                "annotations": {
                                    "description": "Pod memory usage at 90%",
                                    "summary": "High memory usage detected"
                                }
                            }]
                        }
                    },
                    {
                        "iteration": 2,
                        "alert": {
                            "alerts": [{
                                "status": "firing",
                                "labels": {
                                    "alertname": "PodMemoryHigh",
                                    "severity": "warning",
                                    "namespace": "test-apps",
                                    "pod": "webapp-pod-002"
                                },
                                "annotations": {
                                    "description": "Pod memory usage at 92%",
                                    "summary": "High memory usage detected"
                                }
                            }]
                        }
                    },
                    {
                        "iteration": 3,
                        "alert": {
                            "alerts": [{
                                "status": "firing",
                                "labels": {
                                    "alertname": "PodMemoryHigh",
                                    "severity": "warning",
                                    "namespace": "test-apps",
                                    "pod": "webapp-pod-003"
                                },
                                "annotations": {
                                    "description": "Pod memory usage at 89%",
                                    "summary": "High memory usage detected"
                                }
                            }]
                        }
                    }
                ]
            },
            {
                "group_name": "ambiguous_service_alerts",
                "expected_confidence_level": "low",
                "scenarios": [
                    {
                        "iteration": 1,
                        "alert": {
                            "alerts": [{
                                "status": "firing",
                                "labels": {
                                    "alertname": "ServiceIssue",
                                    "severity": "info"
                                },
                                "annotations": {
                                    "description": "Service experiencing issues",
                                    "summary": "Issue detected"
                                }
                            }]
                        }
                    },
                    {
                        "iteration": 2,
                        "alert": {
                            "alerts": [{
                                "status": "firing",
                                "labels": {
                                    "alertname": "ServiceProblem",
                                    "severity": "info"
                                },
                                "annotations": {
                                    "description": "Service has problems",
                                    "summary": "Problem detected"
                                }
                            }]
                        }
                    }
                ]
            }
        ]

    def test_single_consistency_scenario(self, scenario, group_name):
        """Test a single scenario for consistency analysis"""
        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=scenario["alert"],
                headers={
                    'Content-Type': 'application/json',
                    'X-Consistency-Test': group_name,
                    'X-Iteration': str(scenario["iteration"]),
                    'X-Request-Remediation': 'true'
                },
                timeout=25
            )

            end_time = time.time()

            result = {
                "iteration": scenario["iteration"],
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "response_text": response.text[:1000] if response.text else "",
                "timestamp": start_time
            }

            # Extract confidence score
            if result["success"]:
                confidence_score = self.analyzer.extract_confidence_score(result["response_text"])
                result["confidence_score"] = confidence_score
                result["confidence_found"] = confidence_score is not None

            return result

        except Exception as e:
            return {
                "iteration": scenario["iteration"],
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def analyze_group_consistency(self, scenario_group, group_results):
        """Analyze consistency within a scenario group"""
        successful_results = [r for r in group_results if r["success"]]
        confidence_scores = [r["confidence_score"] for r in successful_results if r.get("confidence_score") is not None]

        if len(confidence_scores) < 2:
            return {
                "insufficient_data": True,
                "successful_iterations": len(successful_results),
                "confidence_scores_found": len(confidence_scores)
            }

        # Calculate consistency metrics
        mean_score = sum(confidence_scores) / len(confidence_scores)
        max_score = max(confidence_scores)
        min_score = min(confidence_scores)
        score_range = max_score - min_score

        # Check if scores are within acceptable consistency tolerance
        consistency_tolerance = 0.2  # 20% tolerance for similar scenarios
        consistent = score_range <= consistency_tolerance

        return {
            "group_name": scenario_group["group_name"],
            "expected_confidence_level": scenario_group["expected_confidence_level"],
            "successful_iterations": len(successful_results),
            "confidence_scores_found": len(confidence_scores),
            "confidence_scores": confidence_scores,
            "mean_score": mean_score,
            "min_score": min_score,
            "max_score": max_score,
            "score_range": score_range,
            "consistency_tolerance": consistency_tolerance,
            "consistent": consistent,
            "consistency_rating": "excellent" if score_range <= 0.1 else "good" if score_range <= 0.2 else "poor"
        }

    def analyze_overall_consistency(self):
        """Analyze overall consistency across all scenario groups"""
        valid_groups = [r for r in self.results if not r["consistency_analysis"].get("insufficient_data", False)]

        if not valid_groups:
            return {"error": "No valid consistency data available"}

        # Overall consistency statistics
        consistent_groups = [g for g in valid_groups if g["consistency_analysis"]["consistent"]]

        analysis = {
            "total_scenario_groups": len(self.results),
            "valid_groups_for_analysis": len(valid_groups),
            "consistent_groups": len(consistent_groups),
            "overall_consistency_rate": (len(consistent_groups) / len(valid_groups) * 100) if valid_groups else 0,
            "group_consistency_details": []
        }

        # Detailed group analysis
        for group_result in valid_groups:
            group_analysis = group_result["consistency_analysis"]
            analysis["group_consistency_details"].append({
                "group_name": group_analysis["group_name"],
                "mean_score": group_analysis["mean_score"],
                "score_range": group_analysis["score_range"],
                "consistent": group_analysis["consistent"],
                "consistency_rating": group_analysis["consistency_rating"]
            })

        # Business requirement validation
        analysis["br_pa_009_consistency_compliance"] = {
            "requirement": "Consistent scoring across similar alert scenarios",
            "consistency_rate": analysis["overall_consistency_rate"],
            "acceptable_consistency": analysis["overall_consistency_rate"] >= 70.0,
            "pass": (analysis["overall_consistency_rate"] >= 70.0 and len(valid_groups) >= 1),
            "consistency_quality": "excellent" if analysis["overall_consistency_rate"] >= 90 else "good" if analysis["overall_consistency_rate"] >= 70 else "needs_improvement"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = ConfidenceConsistencyTest(webhook_url)
    results = tester.test_confidence_consistency()

    # Save results
    with open(f"results/{test_session}/confidence_consistency_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    if "error" not in results:
        print(f"\n=== Confidence Score Consistency Results ===")
        print(f"Total Scenario Groups: {results['total_scenario_groups']}")
        print(f"Valid Groups for Analysis: {results['valid_groups_for_analysis']}")
        print(f"Consistent Groups: {results['consistent_groups']}")
        print(f"Overall Consistency Rate: {results['overall_consistency_rate']:.1f}%")

        print(f"\nGroup Details:")
        for detail in results['group_consistency_details']:
            consistent_indicator = '✅' if detail['consistent'] else '❌'
            print(f"  {detail['group_name']}: {consistent_indicator} {detail['consistency_rating']} (range: {detail['score_range']:.3f})")

        compliance = results["br_pa_009_consistency_compliance"]
        print(f"\n=== Consistency Compliance ===")
        print(f"Requirement: {compliance['requirement']}")
        print(f"Consistency Quality: {compliance['consistency_quality']}")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    else:
        print(f"❌ Error: {results['error']}")