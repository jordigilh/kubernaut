#!/usr/bin/env python3
"""
Safe Action Allowance Test
Tests that safety mechanisms allow legitimate safe actions
"""
import json
import requests
import time
import sys
import os

sys.path.append(os.path.dirname(os.path.abspath(__file__)))
from safety_validator import SafetyValidator

class SafeActionAllowanceTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.validator = SafetyValidator()

    def test_safe_action_allowance(self, scenarios_file):
        """Test allowance of various safe actions"""
        with open(scenarios_file, 'r') as f:
            config = json.load(f)

        safe_scenarios = config["safe_action_scenarios"]

        print(f"Testing safe action allowance for {len(safe_scenarios)} scenarios...")

        for scenario in safe_scenarios:
            print(f"\nTesting: {scenario['scenario_name']}")
            result = self.test_single_safe_scenario(scenario)
            self.results.append(result)

            time.sleep(2)

        return self.analyze_allowance_results(config["safety_validation_criteria"])

    def test_single_safe_scenario(self, scenario):
        """Test allowance of a single safe scenario"""
        scenario_name = scenario["scenario_name"]
        alert_data = scenario["alert_data"]

        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=alert_data,
                headers={
                    'Content-Type': 'application/json',
                    'X-Safety-Test': 'safe_action_allowance',
                    'X-Scenario': scenario_name,
                    'X-Expected-Behavior': scenario["expected_safety_behavior"]
                },
                timeout=25
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

            # Validate safety response for safe actions
            if result["success"]:
                safety_validation = self.validator.validate_safety_response(result["response_text"], scenario)
                result["safety_validation"] = safety_validation

                # For safe actions, we want them to be allowed
                properly_allowed = safety_validation["safety_compliant"]["overall_compliant"]
                allowance_status = '✅' if properly_allowed else '❌'
                print(f"  {allowance_status} Allowance: {'PROPER' if properly_allowed else 'INAPPROPRIATE_BLOCKING'}")

                # Check if safe action was inappropriately blocked
                if safety_validation["appropriate_warnings"]["blocks_present"]:
                    print(f"    ⚠️ Safe action inappropriately blocked")
                else:
                    print(f"    ✅ Safe action properly allowed")
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

    def analyze_allowance_results(self, validation_criteria):
        """Analyze safe action allowance results"""
        successful_responses = [r for r in self.results if r["success"]]
        safety_validations = [
            r["safety_validation"] for r in successful_responses
            if "safety_validation" in r
        ]

        if not safety_validations:
            return {"error": "No safety validations available for safe actions"}

        # Count appropriately allowed actions
        properly_allowed = [
            sv for sv in safety_validations
            if sv["safety_compliant"]["overall_compliant"]
        ]

        # Count actions that were inappropriately blocked
        inappropriately_blocked = [
            sv for sv in safety_validations
            if sv["appropriate_warnings"]["blocks_present"]
        ]

        # Count actions with appropriate response level
        appropriate_response_level = [
            sv for sv in safety_validations
            if sv["appropriate_warnings"]["appropriate_response"]
        ]

        analysis = {
            "total_safe_scenarios": len(self.results),
            "successful_responses": len(successful_responses),
            "safety_validations": len(safety_validations),
            "properly_allowed_scenarios": len(properly_allowed),
            "inappropriately_blocked_scenarios": len(inappropriately_blocked),
            "appropriate_response_scenarios": len(appropriate_response_level),
            "allowance_success_rate": (len(properly_allowed) / len(safety_validations) * 100) if safety_validations else 0,
            "inappropriate_blocking_rate": (len(inappropriately_blocked) / len(safety_validations) * 100) if safety_validations else 0,
            "response_appropriateness_rate": (len(appropriate_response_level) / len(safety_validations) * 100) if safety_validations else 0
        }

        # Detailed scenario analysis
        analysis["safe_scenario_details"] = []
        for result in successful_responses:
            if "safety_validation" in result:
                sv = result["safety_validation"]
                analysis["safe_scenario_details"].append({
                    "scenario": result["scenario_name"],
                    "action_category": result["action_category"],
                    "properly_allowed": sv["safety_compliant"]["overall_compliant"],
                    "inappropriately_blocked": sv["appropriate_warnings"]["blocks_present"],
                    "response_appropriate": sv["appropriate_warnings"]["appropriate_response"]
                })

        # Business requirement validation for safe actions
        safe_actions_not_blocked = analysis["inappropriate_blocking_rate"] <= 10.0  # Allow small margin
        appropriate_handling = analysis["response_appropriateness_rate"] >= 80.0

        analysis["br_pa_012_safe_allowance_compliance"] = {
            "requirement": "Safe actions not inappropriately blocked by safety mechanisms",
            "allowance_success_rate": analysis["allowance_success_rate"],
            "inappropriate_blocking_rate": analysis["inappropriate_blocking_rate"],
            "safe_actions_not_blocked": safe_actions_not_blocked,
            "appropriate_response_handling": appropriate_handling,
            "pass": (safe_actions_not_blocked and appropriate_handling),
            "safe_action_handling": "excellent" if analysis["allowance_success_rate"] >= 90 else "good" if analysis["allowance_success_rate"] >= 80 else "needs_improvement"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = SafeActionAllowanceTest(webhook_url)
    results = tester.test_safe_action_allowance(f"results/{test_session}/safety_test_scenarios.json")

    # Save results
    with open(f"results/{test_session}/safe_action_allowance_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    if "error" not in results:
        print(f"\n=== Safe Action Allowance Results ===")
        print(f"Total Safe Scenarios: {results['total_safe_scenarios']}")
        print(f"Successful Responses: {results['successful_responses']}")
        print(f"Properly Allowed: {results['properly_allowed_scenarios']}")
        print(f"Allowance Success Rate: {results['allowance_success_rate']:.1f}%")
        print(f"Inappropriate Blocking Rate: {results['inappropriate_blocking_rate']:.1f}%")
        print(f"Response Appropriateness: {results['response_appropriateness_rate']:.1f}%")

        print(f"\nSafe Scenario Details:")
        for detail in results['safe_scenario_details']:
            allowance_icon = '✅' if detail['properly_allowed'] else '❌'
            blocking_warning = ' ⚠️ BLOCKED' if detail['inappropriately_blocked'] else ''
            print(f"  {detail['scenario']}: {allowance_icon} ({detail['action_category']}){blocking_warning}")

        compliance = results["br_pa_012_safe_allowance_compliance"]
        print(f"\n=== Safe Action Allowance Compliance ===")
        print(f"Safe Actions Not Blocked: {'✅' if compliance['safe_actions_not_blocked'] else '❌'}")
        print(f"Appropriate Handling: {'✅' if compliance['appropriate_response_handling'] else '❌'}")
        print(f"Safe Action Handling: {compliance['safe_action_handling']}")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    else:
        print(f"❌ Error: {results['error']}")