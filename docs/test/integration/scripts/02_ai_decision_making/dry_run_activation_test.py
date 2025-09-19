#!/usr/bin/env python3
"""
Dry-Run Mode Activation Test
Tests activation of dry-run mode and validation of safe simulation
"""
import json
import requests
import time
import sys
import os

sys.path.append(os.path.dirname(os.path.abspath(__file__)))
from dry_run_validator import DryRunValidator

class DryRunActivationTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.validator = DryRunValidator()

    def test_dry_run_mode_activation(self, scenarios_file):
        """Test dry-run mode activation for various scenarios"""
        with open(scenarios_file, 'r') as f:
            config = json.load(f)

        scenarios = config["dry_run_scenarios"]
        validation_criteria = config["validation_criteria"]

        print(f"Testing dry-run mode activation for {len(scenarios)} scenarios...")

        for scenario in scenarios:
            print(f"\nTesting: {scenario['scenario_name']}")

            # Test both regular mode and dry-run mode for comparison
            regular_result = self.test_regular_mode(scenario)
            time.sleep(2)

            dry_run_result = self.test_dry_run_mode(scenario)
            time.sleep(2)

            # Compare results
            comparison = self.compare_modes(regular_result, dry_run_result, scenario)

            self.results.append({
                "scenario_name": scenario["scenario_name"],
                "regular_mode": regular_result,
                "dry_run_mode": dry_run_result,
                "comparison": comparison
            })

        return self.analyze_dry_run_activation(validation_criteria)

    def test_regular_mode(self, scenario):
        """Test regular mode operation for comparison"""
        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=scenario["alert_data"],
                headers={
                    'Content-Type': 'application/json',
                    'X-Regular-Mode': 'true',
                    'X-Scenario': scenario["scenario_name"]
                },
                timeout=25
            )

            end_time = time.time()

            return {
                "mode": "regular",
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "response_text": response.text[:1200] if response.text else "",
                "timestamp": start_time
            }

        except Exception as e:
            return {
                "mode": "regular",
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def test_dry_run_mode(self, scenario):
        """Test dry-run mode operation"""
        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=scenario["alert_data"],
                headers={
                    'Content-Type': 'application/json',
                    'X-Dry-Run': 'true',
                    'X-Dry-Run-Mode': 'enabled',
                    'X-Test-Mode': 'simulation',
                    'X-Scenario': scenario["scenario_name"]
                },
                timeout=25
            )

            end_time = time.time()

            result = {
                "mode": "dry_run",
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "response_text": response.text[:1200] if response.text else "",
                "timestamp": start_time
            }

            # Validate dry-run safety
            if result["success"]:
                validation = self.validator.validate_dry_run_response(result["response_text"], scenario)
                result["dry_run_validation"] = validation

            return result

        except Exception as e:
            return {
                "mode": "dry_run",
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def compare_modes(self, regular_result, dry_run_result, scenario):
        """Compare regular and dry-run mode results"""
        comparison = {
            "both_successful": regular_result["success"] and dry_run_result["success"],
            "dry_run_safer": False,
            "response_differences": {},
            "safety_assessment": "unknown"
        }

        if dry_run_result["success"] and "dry_run_validation" in dry_run_result:
            validation = dry_run_result["dry_run_validation"]

            comparison.update({
                "dry_run_indicated": validation["dry_run_indicated"]["indicated"],
                "dangerous_actions_prevented": not validation["dangerous_actions_detected"]["has_dangerous_actions"],
                "safe_simulation_provided": validation["safe_simulation_detected"]["has_safe_simulation"],
                "safety_compliant": validation["safety_compliant"]
            })

            # Assess if dry-run is meaningfully different from regular mode
            if regular_result["success"]:
                regular_text = regular_result["response_text"].lower()
                dry_run_text = dry_run_result["response_text"].lower()

                comparison["response_differences"] = {
                    "length_difference": len(dry_run_text) - len(regular_text),
                    "content_similarity": self._calculate_text_similarity(regular_text, dry_run_text),
                    "dry_run_specific_content": len([word for word in self.validator.dry_run_indicators if word in dry_run_text])
                }

            comparison["safety_assessment"] = "safe" if validation["safety_compliant"] else "unsafe"

        return comparison

    def _calculate_text_similarity(self, text1, text2):
        """Calculate basic text similarity between responses"""
        words1 = set(text1.split())
        words2 = set(text2.split())

        intersection = words1.intersection(words2)
        union = words1.union(words2)

        return len(intersection) / len(union) if union else 0

    def analyze_dry_run_activation(self, validation_criteria):
        """Analyze dry-run mode activation results"""
        total_scenarios = len(self.results)
        successful_dry_runs = [r for r in self.results if r["dry_run_mode"]["success"]]
        safety_compliant = [
            r for r in successful_dry_runs
            if r["comparison"].get("safety_compliant", False)
        ]

        analysis = {
            "total_scenarios_tested": total_scenarios,
            "successful_dry_run_activations": len(successful_dry_runs),
            "dry_run_success_rate": (len(successful_dry_runs) / total_scenarios * 100) if total_scenarios > 0 else 0,
            "safety_compliant_scenarios": len(safety_compliant),
            "safety_compliance_rate": (len(safety_compliant) / len(successful_dry_runs) * 100) if successful_dry_runs else 0,
            "scenario_safety_details": []
        }

        # Detailed safety analysis per scenario
        for result in successful_dry_runs:
            comparison = result["comparison"]
            analysis["scenario_safety_details"].append({
                "scenario": result["scenario_name"],
                "dry_run_indicated": comparison.get("dry_run_indicated", False),
                "dangerous_actions_prevented": comparison.get("dangerous_actions_prevented", False),
                "safe_simulation_provided": comparison.get("safe_simulation_provided", False),
                "safety_assessment": comparison.get("safety_assessment", "unknown")
            })

        # Business requirement validation
        analysis["br_pa_010_compliance"] = {
            "requirement": "Support dry-run mode for safe testing of remediation actions",
            "dry_run_activation_success": analysis["dry_run_success_rate"],
            "safety_compliance_rate": analysis["safety_compliance_rate"],
            "no_actual_actions_executed": analysis["safety_compliance_rate"] >= validation_criteria["safety_validation_threshold"] * 100,
            "pass": (analysis["dry_run_success_rate"] >= 80.0 and
                    analysis["safety_compliance_rate"] >= 80.0),
            "dry_run_implementation": "safe" if analysis["safety_compliance_rate"] >= 80 else "unsafe" if analysis["dry_run_success_rate"] >= 50 else "not_implemented"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = DryRunActivationTest(webhook_url)
    results = tester.test_dry_run_mode_activation(f"results/{test_session}/dry_run_test_scenarios.json")

    # Save results
    with open(f"results/{test_session}/dry_run_activation_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== Dry-Run Mode Activation Results ===")
    print(f"Total Scenarios: {results['total_scenarios_tested']}")
    print(f"Successful Dry-Run Activations: {results['successful_dry_run_activations']}")
    print(f"Dry-Run Success Rate: {results['dry_run_success_rate']:.1f}%")
    print(f"Safety Compliant Scenarios: {results['safety_compliant_scenarios']}")
    print(f"Safety Compliance Rate: {results['safety_compliance_rate']:.1f}%")

    print(f"\nScenario Safety Details:")
    for detail in results['scenario_safety_details']:
        safety_indicator = '✅' if detail['safety_assessment'] == 'safe' else '❌'
        print(f"  {detail['scenario']}: {safety_indicator} {detail['safety_assessment']}")

    compliance = results["br_pa_010_compliance"]
    print(f"\n=== BR-PA-010 Compliance ===")
    print(f"Requirement: {compliance['requirement']}")
    print(f"Dry-Run Implementation: {compliance['dry_run_implementation']}")
    print(f"No Actual Actions Executed: {'✅' if compliance['no_actual_actions_executed'] else '❌'}")
    print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")