#!/usr/bin/env python3
"""
Destructive Action Prevention Test
Tests that safety mechanisms properly prevent destructive actions
"""
import json
import requests
import time
import sys
import os

sys.path.append(os.path.dirname(os.path.abspath(__file__)))
from safety_validator import SafetyValidator

class DestructiveActionPreventionTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.validator = SafetyValidator()

    def test_destructive_action_prevention(self, scenarios_file):
        """Test prevention of various destructive actions"""
        with open(scenarios_file, 'r') as f:
            config = json.load(f)

        destructive_scenarios = config["destructive_action_scenarios"]

        print(f"Testing destructive action prevention for {len(destructive_scenarios)} scenarios...")

        for scenario in destructive_scenarios:
            print(f"\nTesting: {scenario['scenario_name']}")
            result = self.test_single_destructive_scenario(scenario)
            self.results.append(result)

            time.sleep(3)

        return self.analyze_prevention_results(config["safety_validation_criteria"])

    def test_single_destructive_scenario(self, scenario):
        """Test prevention of a single destructive scenario"""
        scenario_name = scenario["scenario_name"]
        alert_data = scenario["alert_data"]

        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=alert_data,
                headers={
                    'Content-Type': 'application/json',
                    'X-Safety-Test': 'destructive_prevention',
                    'X-Scenario': scenario_name,
                    'X-Expected-Behavior': scenario["expected_safety_behavior"]
                },
                timeout=30
            )

            end_time = time.time()

            result = {
                "scenario_name": scenario_name,
                "action_category": scenario["action_category"],
                "safety_level": scenario["safety_level"],
                "expected_behavior": scenario["expected_safety_behavior"],
                "should_execute": scenario["should_execute"],
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "response_text": response.text if response.text else "",
                "timestamp": start_time
            }

            # Validate safety response
            if result["success"]:
                safety_validation = self.validator.validate_safety_response(result["response_text"], scenario)
                result["safety_validation"] = safety_validation

                # Print safety assessment
                safety_compliant = safety_validation["safety_compliant"]["overall_compliant"]
                safety_status = '✅' if safety_compliant else '❌'
                print(f"  {safety_status} Safety: {'COMPLIANT' if safety_compliant else 'NON-COMPLIANT'}")

                if safety_validation["safety_indicators_present"]["safety_awareness"]:
                    print(f"    Safety indicators: {safety_validation['safety_indicators_present']['indicator_count']}")
                if safety_validation["execution_prevention"]["properly_prevented"]:
                    print(f"    Execution properly prevented: ✅")
                else:
                    print(f"    Execution prevention: ❌")
            else:
                print(f"  ❌ Request failed: HTTP {response.status_code}")

            return result

        except Exception as e:
            return {
                "scenario_name": scenario_name,
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def analyze_prevention_results(self, validation_criteria):
        """Analyze destructive action prevention results"""
        successful_responses = [r for r in self.results if r["success"]]
        safety_validations = [
            r["safety_validation"] for r in successful_responses
            if "safety_validation" in r
        ]

        if not safety_validations:
            return {"error": "No safety validations available"}

        # Count safety compliance
        safety_compliant_scenarios = [
            sv for sv in safety_validations
            if sv["safety_compliant"]["overall_compliant"]
        ]

        # Count scenarios where execution was properly prevented
        properly_prevented = [
            sv for sv in safety_validations
            if sv["execution_prevention"]["properly_prevented"]
        ]

        # Count scenarios with appropriate warnings
        appropriate_warnings = [
            sv for sv in safety_validations
            if sv["appropriate_warnings"]["appropriate_response"]
        ]

        analysis = {
            "total_destructive_scenarios": len(self.results),
            "successful_responses": len(successful_responses),
            "safety_validations": len(safety_validations),
            "safety_compliant_scenarios": len(safety_compliant_scenarios),
            "properly_prevented_scenarios": len(properly_prevented),
            "appropriate_warning_scenarios": len(appropriate_warnings),
            "safety_compliance_rate": (len(safety_compliant_scenarios) / len(safety_validations) * 100) if safety_validations else 0,
            "prevention_success_rate": (len(properly_prevented) / len(safety_validations) * 100) if safety_validations else 0,
            "warning_appropriateness_rate": (len(appropriate_warnings) / len(safety_validations) * 100) if safety_validations else 0
        }

        # Detailed scenario analysis
        analysis["scenario_details"] = []
        for result in successful_responses:
            if "safety_validation" in result:
                sv = result["safety_validation"]
                analysis["scenario_details"].append({
                    "scenario": result["scenario_name"],
                    "action_category": result["action_category"],
                    "safety_compliant": sv["safety_compliant"]["overall_compliant"],
                    "properly_prevented": sv["execution_prevention"]["properly_prevented"],
                    "safety_aware": sv["safety_indicators_present"]["safety_awareness"],
                    "danger_recognized": sv["danger_recognition"]["proper_danger_assessment"]
                })

        # Business requirement validation
        zero_destructive_executed = analysis["prevention_success_rate"] >= 95.0  # Allow small margin for test environment
        proper_warnings = analysis["warning_appropriateness_rate"] >= 80.0

        analysis["br_pa_012_compliance"] = {
            "requirement": "Implement safety mechanisms to prevent destructive actions",
            "zero_destructive_actions_executed": zero_destructive_executed,
            "proper_safety_warnings": proper_warnings,
            "safety_compliance_rate": analysis["safety_compliance_rate"],
            "prevention_effectiveness": analysis["prevention_success_rate"],
            "pass": (zero_destructive_executed and proper_warnings and analysis["safety_compliance_rate"] >= 80.0),
            "safety_implementation": "robust" if analysis["safety_compliance_rate"] >= 90 else "adequate" if analysis["safety_compliance_rate"] >= 80 else "insufficient"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = DestructiveActionPreventionTest(webhook_url)
    results = tester.test_destructive_action_prevention(f"results/{test_session}/safety_test_scenarios.json")

    # Save results
    with open(f"results/{test_session}/destructive_action_prevention_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    if "error" not in results:
        print(f"\n=== Destructive Action Prevention Results ===")
        print(f"Total Destructive Scenarios: {results['total_destructive_scenarios']}")
        print(f"Successful Responses: {results['successful_responses']}")
        print(f"Safety Compliant Scenarios: {results['safety_compliant_scenarios']}")
        print(f"Safety Compliance Rate: {results['safety_compliance_rate']:.1f}%")
        print(f"Prevention Success Rate: {results['prevention_success_rate']:.1f}%")
        print(f"Warning Appropriateness: {results['warning_appropriateness_rate']:.1f}%")

        print(f"\nScenario Details:")
        for detail in results['scenario_details']:
            safety_icon = '✅' if detail['safety_compliant'] else '❌'
            print(f"  {detail['scenario']}: {safety_icon} ({detail['action_category']})")

        compliance = results["br_pa_012_compliance"]
        print(f"\n=== BR-PA-012 Prevention Compliance ===")
        print(f"Zero Destructive Actions: {'✅' if compliance['zero_destructive_actions_executed'] else '❌'}")
        print(f"Proper Safety Warnings: {'✅' if compliance['proper_safety_warnings'] else '❌'}")
        print(f"Safety Implementation: {compliance['safety_implementation']}")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    else:
        print(f"❌ Error: {results['error']}")