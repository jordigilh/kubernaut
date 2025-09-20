#!/usr/bin/env python3
"""
Kubernetes Action Success Rate Validation Test
Tests success rate compliance for Kubernetes action execution
"""
import json
import requests
import time
import random

class SuccessRateValidationTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []

    def test_success_rate_under_conditions(self):
        """Test success rate under various conditions"""
        print("Testing Kubernetes action success rate under various conditions...")

        test_conditions = [
            {
                "condition_name": "normal_operations",
                "description": "Normal operational conditions",
                "test_count": 20,
                "delay_between_tests": 1
            },
            {
                "condition_name": "rapid_succession",
                "description": "Rapid succession execution",
                "test_count": 15,
                "delay_between_tests": 0.5
            },
            {
                "condition_name": "mixed_complexity",
                "description": "Mixed complexity actions",
                "test_count": 10,
                "delay_between_tests": 2
            }
        ]

        for condition in test_conditions:
            print(f"\n--- Testing: {condition['condition_name']} ---")
            condition_results = self.test_condition(condition)
            self.results.append(condition_results)

        return self.analyze_overall_success_rate()

    def test_condition(self, condition):
        """Test success rate under a specific condition"""
        condition_name = condition["condition_name"]
        test_count = condition["test_count"]
        delay = condition["delay_between_tests"]

        actions = self.generate_test_actions(test_count)
        condition_results = []

        print(f"Running {test_count} actions with {delay}s intervals...")

        for i, action in enumerate(actions, 1):
            print(f"  [{i}/{test_count}] {action['action_type']}", end="... ")

            result = self.execute_test_action(action, condition_name)
            condition_results.append(result)

            status = '✅' if result['success'] else '❌'
            print(status)

            if i < test_count:  # No delay after last test
                time.sleep(delay)

        # Calculate condition success rate
        successful = sum(1 for r in condition_results if r['success'])
        success_rate = (successful / len(condition_results)) * 100 if condition_results else 0

        print(f"  Condition Success Rate: {success_rate:.1f}% ({successful}/{len(condition_results)})")

        return {
            "condition_name": condition_name,
            "description": condition["description"],
            "total_tests": len(condition_results),
            "successful_tests": successful,
            "success_rate": success_rate / 100,  # Convert to decimal
            "test_results": condition_results
        }

    def generate_test_actions(self, count):
        """Generate a diverse set of test actions"""
        base_actions = [
            {"action_type": "get_pod_status", "complexity": "simple"},
            {"action_type": "describe_deployment", "complexity": "simple"},
            {"action_type": "scale_deployment", "complexity": "medium"},
            {"action_type": "restart_pod", "complexity": "medium"},
            {"action_type": "check_service_endpoints", "complexity": "simple"},
            {"action_type": "get_node_status", "complexity": "simple"},
            {"action_type": "patch_resource_limits", "complexity": "complex"},
            {"action_type": "drain_node", "complexity": "complex"},
            {"action_type": "get_events", "complexity": "simple"},
            {"action_type": "rollout_restart", "complexity": "medium"}
        ]

        # Generate required number of actions by repeating and randomizing
        actions = []
        for i in range(count):
            base_action = base_actions[i % len(base_actions)]
            action = base_action.copy()
            action["test_instance"] = i + 1
            actions.append(action)

        return actions

    def execute_test_action(self, action, condition_name):
        """Execute a single test action"""
        test_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "K8sActionTest",
                    "severity": "warning",
                    "namespace": "k8s-actions-test",
                    "test_action": action["action_type"],
                    "test_condition": condition_name
                },
                "annotations": {
                    "description": f"Test execution of {action['action_type']}",
                    "summary": f"Success rate test for {condition_name}",
                    "action_complexity": action["complexity"]
                },
                "startsAt": "2025-01-01T10:00:00Z"
            }]
        }

        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=test_alert,
                headers={
                    'Content-Type': 'application/json',
                    'X-Success-Rate-Test': condition_name,
                    'X-Action-Type': action["action_type"]
                },
                timeout=25
            )

            end_time = time.time()

            return {
                "action_type": action["action_type"],
                "test_instance": action["test_instance"],
                "complexity": action["complexity"],
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "condition": condition_name,
                "timestamp": start_time
            }

        except Exception as e:
            return {
                "action_type": action["action_type"],
                "test_instance": action["test_instance"],
                "complexity": action["complexity"],
                "success": False,
                "error": str(e),
                "condition": condition_name,
                "timestamp": time.time()
            }

    def analyze_overall_success_rate(self):
        """Analyze overall success rate across all conditions"""
        if not self.results:
            return {"error": "No test results available"}

        # Aggregate all test results
        all_tests = []
        condition_summaries = []

        for condition_result in self.results:
            all_tests.extend(condition_result["test_results"])
            condition_summaries.append({
                "condition": condition_result["condition_name"],
                "tests": condition_result["total_tests"],
                "successful": condition_result["successful_tests"],
                "success_rate": condition_result["success_rate"]
            })

        # Overall statistics
        total_tests = len(all_tests)
        successful_tests = sum(1 for test in all_tests if test["success"])
        overall_success_rate = (successful_tests / total_tests) if total_tests > 0 else 0

        # Analysis by complexity
        complexity_analysis = {}
        for test in all_tests:
            complexity = test.get("complexity", "unknown")
            if complexity not in complexity_analysis:
                complexity_analysis[complexity] = {"total": 0, "successful": 0}

            complexity_analysis[complexity]["total"] += 1
            if test["success"]:
                complexity_analysis[complexity]["successful"] += 1

        # Calculate success rates by complexity
        for complexity, data in complexity_analysis.items():
            data["success_rate"] = (data["successful"] / data["total"]) if data["total"] > 0 else 0

        analysis = {
            "total_tests_executed": total_tests,
            "successful_tests": successful_tests,
            "failed_tests": total_tests - successful_tests,
            "overall_success_rate": overall_success_rate,
            "overall_success_rate_percentage": overall_success_rate * 100,
            "condition_summaries": condition_summaries,
            "complexity_analysis": complexity_analysis,
            "test_conditions_count": len(self.results)
        }

        # Business requirement validation
        analysis["br_pa_011_success_rate_compliance"] = {
            "requirement": "95% success rate for Kubernetes action execution",
            "measured_success_rate": analysis["overall_success_rate"],
            "required_success_rate": 0.95,
            "success_rate_met": analysis["overall_success_rate"] >= 0.95,
            "margin_above_requirement": (analysis["overall_success_rate"] - 0.95) * 100,
            "pass": analysis["overall_success_rate"] >= 0.95,
            "reliability_rating": "excellent" if analysis["overall_success_rate"] >= 0.98 else "good" if analysis["overall_success_rate"] >= 0.95 else "needs_improvement"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = SuccessRateValidationTest(webhook_url)
    results = tester.test_success_rate_under_conditions()

    # Save results
    with open(f"results/{test_session}/success_rate_validation_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    if "error" not in results:
        print(f"\n=== Success Rate Validation Results ===")
        print(f"Total Tests Executed: {results['total_tests_executed']}")
        print(f"Successful Tests: {results['successful_tests']}")
        print(f"Overall Success Rate: {results['overall_success_rate_percentage']:.1f}%")

        print(f"\nCondition Summaries:")
        for summary in results['condition_summaries']:
            print(f"  {summary['condition']}: {summary['success_rate']*100:.1f}% ({summary['successful']}/{summary['tests']})")

        print(f"\nComplexity Analysis:")
        for complexity, data in results['complexity_analysis'].items():
            print(f"  {complexity}: {data['success_rate']*100:.1f}% ({data['successful']}/{data['total']})")

        compliance = results["br_pa_011_success_rate_compliance"]
        print(f"\n=== Success Rate Compliance ===")
        print(f"Requirement: {compliance['requirement']}")
        print(f"Measured: {compliance['measured_success_rate']*100:.1f}%")
        print(f"Margin: {compliance['margin_above_requirement']:.1f}% above requirement")
        print(f"Reliability Rating: {compliance['reliability_rating']}")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    else:
        print(f"❌ Error: {results['error']}")