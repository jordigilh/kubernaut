#!/usr/bin/env python3
"""
Confidence Score Provision Test
Tests that confidence scores are provided for remediation recommendations
"""
import json
import requests
import time
import sys
import os

sys.path.append(os.path.dirname(os.path.abspath(__file__)))
from confidence_analyzer import ConfidenceAnalyzer

class ConfidenceProvisionTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.analyzer = ConfidenceAnalyzer()

    def test_confidence_score_provision(self, scenarios_file):
        """Test confidence score provision for all scenarios"""
        with open(scenarios_file, 'r') as f:
            config = json.load(f)

        scenarios = config["confidence_test_scenarios"]
        validation_criteria = config["confidence_validation_criteria"]

        print(f"Testing confidence score provision for {len(scenarios)} scenarios...")

        for scenario in scenarios:
            print(f"\nTesting: {scenario['scenario_name']}")
            result = self.test_single_scenario(scenario)
            self.results.append(result)

            time.sleep(3)

        return self.analyze_confidence_provision(validation_criteria)

    def test_single_scenario(self, scenario):
        """Test confidence score provision for a single scenario"""
        scenario_name = scenario["scenario_name"]
        alert_data = scenario["alert_data"]

        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=alert_data,
                headers={
                    'Content-Type': 'application/json',
                    'X-Request-Confidence': 'true',
                    'X-Scenario': scenario_name,
                    'X-Request-Remediation': 'true'
                },
                timeout=30
            )

            end_time = time.time()

            success = response.status_code == 200
            response_text = response.text if response.text else ""

            # Extract and validate confidence score
            confidence_analysis = None
            if success and response_text:
                extracted_score = self.analyzer.extract_confidence_score(response_text)
                confidence_analysis = self.analyzer.validate_confidence_score(extracted_score, scenario)

            result = {
                "scenario_name": scenario_name,
                "success": success,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "response_text": response_text[:1200],  # First 1200 chars
                "confidence_analysis": confidence_analysis,
                "expected_confidence": scenario.get("expected_confidence", "unknown"),
                "timestamp": start_time
            }

            # Print scenario result
            if success and confidence_analysis:
                if confidence_analysis["score_provided"]:
                    score = confidence_analysis["extracted_score"]
                    scale_ok = '✅' if confidence_analysis["scale_compliant"] else '❌'
                    range_ok = '✅' if confidence_analysis["within_expected_range"] else '❌'
                    print(f"  Score: {score:.3f} {scale_ok} Scale: [0-1] {range_ok} Expected Range")
                else:
                    print(f"  ❌ No confidence score found")
            else:
                print(f"  ❌ Failed to get recommendation (HTTP {response.status_code})")

            return result

        except Exception as e:
            return {
                "scenario_name": scenario_name,
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def analyze_confidence_provision(self, validation_criteria):
        """Analyze confidence score provision across all tests"""
        successful_responses = [r for r in self.results if r["success"]]
        confidence_analyses = [
            r["confidence_analysis"] for r in successful_responses
            if r.get("confidence_analysis")
        ]

        if not confidence_analyses:
            return {"error": "No confidence analyses available"}

        # Calculate provision statistics
        scores_provided = [ca for ca in confidence_analyses if ca["score_provided"]]
        scale_compliant = [ca for ca in scores_provided if ca["scale_compliant"]]
        range_compliant = [ca for ca in scores_provided if ca["within_expected_range"]]

        # Extract scores for statistical analysis
        extracted_scores = [ca["extracted_score"] for ca in scores_provided]

        analysis = {
            "total_scenarios_tested": len(self.results),
            "successful_responses": len(successful_responses),
            "response_success_rate": (len(successful_responses) / len(self.results) * 100) if self.results else 0,
            "confidence_scores_provided": len(scores_provided),
            "confidence_provision_rate": (len(scores_provided) / len(successful_responses) * 100) if successful_responses else 0,
            "scale_compliant_scores": len(scale_compliant),
            "scale_compliance_rate": (len(scale_compliant) / len(scores_provided) * 100) if scores_provided else 0,
            "range_compliant_scores": len(range_compliant),
            "range_compliance_rate": (len(range_compliant) / len(scores_provided) * 100) if scores_provided else 0,
            "extracted_scores": extracted_scores
        }

        # Score statistics
        if extracted_scores:
            analysis["score_statistics"] = {
                "mean_score": sum(extracted_scores) / len(extracted_scores),
                "min_score": min(extracted_scores),
                "max_score": max(extracted_scores),
                "score_range": max(extracted_scores) - min(extracted_scores),
                "unique_scores": len(set(extracted_scores))
            }

        # Business requirement validation
        analysis["br_pa_009_compliance"] = {
            "requirement": "Provide confidence scores (0-1 scale) for recommendations",
            "provision_rate": analysis["confidence_provision_rate"],
            "scale_compliance_rate": analysis["scale_compliance_rate"],
            "range_appropriateness": analysis["range_compliance_rate"],
            "pass": (analysis["confidence_provision_rate"] >= 80.0 and
                    analysis["scale_compliance_rate"] >= 90.0),
            "confidence_capability": "implemented" if analysis["confidence_provision_rate"] >= 60.0 else "not_implemented"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = ConfidenceProvisionTest(webhook_url)
    results = tester.test_confidence_score_provision(f"results/{test_session}/confidence_scoring_scenarios.json")

    # Save results
    with open(f"results/{test_session}/confidence_provision_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    if "error" not in results:
        print(f"\n=== Confidence Score Provision Results ===")
        print(f"Total Scenarios: {results['total_scenarios_tested']}")
        print(f"Successful Responses: {results['successful_responses']}")
        print(f"Confidence Scores Provided: {results['confidence_scores_provided']}")
        print(f"Provision Rate: {results['confidence_provision_rate']:.1f}%")
        print(f"Scale Compliance Rate: {results['scale_compliance_rate']:.1f}%")
        print(f"Range Compliance Rate: {results['range_compliance_rate']:.1f}%")

        if 'score_statistics' in results:
            stats = results['score_statistics']
            print(f"\nScore Statistics:")
            print(f"  Mean Score: {stats['mean_score']:.3f}")
            print(f"  Score Range: {stats['min_score']:.3f} - {stats['max_score']:.3f}")
            print(f"  Unique Scores: {stats['unique_scores']}")

        compliance = results["br_pa_009_compliance"]
        print(f"\n=== BR-PA-009 Compliance ===")
        print(f"Requirement: {compliance['requirement']}")
        print(f"Confidence Capability: {compliance['confidence_capability']}")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    else:
        print(f"❌ Error: {results['error']}")