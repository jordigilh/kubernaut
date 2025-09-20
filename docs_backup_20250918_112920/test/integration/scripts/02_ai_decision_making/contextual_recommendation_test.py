#!/usr/bin/env python3
"""
Contextual Recommendation Generation Test
Tests generation of contextual remediation recommendations for various alert scenarios
"""
import json
import requests
import time
import sys
import os

# Add the current directory to path to import analyzer
sys.path.append(os.path.dirname(os.path.abspath(__file__)))
from remediation_analyzer import RemediationAnalyzer

class ContextualRecommendationTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.analyzer = RemediationAnalyzer()

    def test_all_scenarios(self, scenarios_file):
        """Test remediation generation for all scenarios"""
        with open(scenarios_file, 'r') as f:
            config = json.load(f)

        scenarios = config["test_scenarios"]
        quality_metrics = config["quality_metrics"]

        print(f"Testing contextual recommendations for {len(scenarios)} scenarios...")

        for scenario in scenarios:
            print(f"\nTesting scenario: {scenario['scenario_name']}")
            result = self.test_single_scenario(scenario, quality_metrics)
            self.results.append(result)

            # Brief delay between scenarios
            time.sleep(3)

        return self.analyze_all_recommendations(quality_metrics)

    def test_single_scenario(self, scenario, quality_metrics):
        """Test recommendation generation for a single scenario"""
        scenario_name = scenario["scenario_name"]
        alert_data = scenario["alert_data"]

        try:
            start_time = time.time()

            # Send alert and request remediation recommendations
            response = requests.post(
                self.webhook_url,
                json=alert_data,
                headers={
                    'Content-Type': 'application/json',
                    'X-Request-Remediation': 'true',
                    'X-Scenario': scenario_name
                },
                timeout=45  # Longer timeout for AI processing
            )

            end_time = time.time()

            success = response.status_code == 200
            response_text = response.text if response.text else ""

            # Analyze recommendation quality
            quality_analysis = None
            if success and response_text:
                quality_analysis = self.analyzer.analyze_recommendation_quality(response_text, scenario)

            result = {
                "scenario_name": scenario_name,
                "status_code": response.status_code,
                "success": success,
                "response_time": end_time - start_time,
                "recommendation_text": response_text[:2000],  # First 2000 chars
                "recommendation_length": len(response_text),
                "quality_analysis": quality_analysis,
                "timestamp": start_time
            }

            # Print scenario result
            if success:
                if quality_analysis:
                    quality_rating = quality_analysis["quality_rating"]
                    quality_score = quality_analysis["overall_quality_score"]
                    print(f"  ✅ Generated recommendation - Quality: {quality_rating} (score: {quality_score:.3f})")
                else:
                    print(f"  ✅ Generated recommendation - Quality analysis failed")
            else:
                print(f"  ❌ Failed to generate recommendation (HTTP {response.status_code})")

            return result

        except Exception as e:
            error_result = {
                "scenario_name": scenario_name,
                "status_code": -1,
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

            print(f"  ❌ Error: {str(e)}")
            return error_result

    def analyze_all_recommendations(self, quality_metrics):
        """Analyze all recommendation results"""
        successful_results = [r for r in self.results if r["success"]]

        if not successful_results:
            return {"error": "No successful recommendation generations"}

        # Overall statistics
        total_scenarios = len(self.results)
        successful_count = len(successful_results)

        # Quality analysis statistics
        quality_analyses = [r["quality_analysis"] for r in successful_results if r.get("quality_analysis")]

        if quality_analyses:
            quality_scores = [qa["overall_quality_score"] for qa in quality_analyses]
            quality_ratings = [qa["quality_rating"] for qa in quality_analyses]

            # Count ratings
            rating_counts = {}
            for rating in quality_ratings:
                rating_counts[rating] = rating_counts.get(rating, 0) + 1
        else:
            quality_scores = []
            quality_ratings = []
            rating_counts = {}

        # Response time analysis
        response_times = [r["response_time"] for r in successful_results]

        analysis = {
            "total_scenarios_tested": total_scenarios,
            "successful_generations": successful_count,
            "success_rate": (successful_count / total_scenarios * 100) if total_scenarios > 0 else 0,
            "average_response_time": sum(response_times) / len(response_times) if response_times else 0,
            "max_response_time": max(response_times) if response_times else 0,
            "min_response_time": min(response_times) if response_times else 0,
            "quality_statistics": {
                "recommendations_analyzed": len(quality_analyses),
                "average_quality_score": sum(quality_scores) / len(quality_scores) if quality_scores else 0,
                "max_quality_score": max(quality_scores) if quality_scores else 0,
                "min_quality_score": min(quality_scores) if quality_scores else 0,
                "quality_rating_distribution": rating_counts
            }
        }

        # Business requirement validation
        avg_quality = analysis["quality_statistics"]["average_quality_score"]
        excellent_or_good = rating_counts.get("excellent", 0) + rating_counts.get("good", 0)
        quality_pass_rate = (excellent_or_good / len(quality_analyses) * 100) if quality_analyses else 0

        analysis["br_pa_007_compliance"] = {
            "requirement": "Provide contextual remediation recommendations for alerts",
            "success_rate": analysis["success_rate"],
            "average_quality_score": avg_quality,
            "quality_pass_rate": quality_pass_rate,
            "meets_quality_threshold": avg_quality >= quality_metrics["expected_actionability_score"],
            "pass": (analysis["success_rate"] >= 80.0 and
                    avg_quality >= quality_metrics["expected_actionability_score"] and
                    quality_pass_rate >= 60.0),
            "recommendation_capability": "excellent" if quality_pass_rate >= 80 else "good" if quality_pass_rate >= 60 else "needs_improvement"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = ContextualRecommendationTest(webhook_url)
    results = tester.test_all_scenarios(f"results/{test_session}/remediation_test_scenarios.json")

    # Save results
    with open(f"results/{test_session}/contextual_recommendation_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    if "error" not in results:
        print(f"\n=== Contextual Recommendation Generation Results ===")
        print(f"Total Scenarios: {results['total_scenarios_tested']}")
        print(f"Successful Generations: {results['successful_generations']}")
        print(f"Success Rate: {results['success_rate']:.1f}%")
        print(f"Average Response Time: {results['average_response_time']:.2f}s")

        quality_stats = results["quality_statistics"]
        print(f"\nQuality Analysis:")
        print(f"Average Quality Score: {quality_stats['average_quality_score']:.3f}")
        print(f"Quality Ratings: {quality_stats['quality_rating_distribution']}")

        compliance = results["br_pa_007_compliance"]
        print(f"\n=== BR-PA-007 Compliance ===")
        print(f"Requirement: {compliance['requirement']}")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
        print(f"Recommendation Capability: {compliance['recommendation_capability']}")
    else:
        print(f"❌ Error: {results['error']}")