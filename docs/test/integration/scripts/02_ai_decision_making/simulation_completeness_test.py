#!/usr/bin/env python3
"""
Simulation Completeness Test
Tests the completeness and detail level of dry-run simulations
"""
import json
import requests
import time
import sys
import os

sys.path.append(os.path.dirname(os.path.abspath(__file__)))
from dry_run_validator import DryRunValidator

class SimulationCompletenessTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.validator = DryRunValidator()

    def test_simulation_completeness(self):
        """Test completeness of dry-run simulations"""
        print("Testing simulation completeness...")

        # Test with scenarios requiring detailed simulations
        detailed_scenarios = self.create_detailed_scenarios()

        for scenario in detailed_scenarios:
            print(f"\nTesting completeness for: {scenario['scenario_name']}")
            result = self.test_detailed_simulation(scenario)
            self.results.append(result)

            time.sleep(3)

        return self.analyze_simulation_completeness()

    def create_detailed_scenarios(self):
        """Create scenarios requiring detailed simulation responses"""
        return [
            {
                "scenario_name": "complex_multi_step_remediation",
                "alert_data": {
                    "alerts": [{
                        "status": "firing",
                        "labels": {
                            "alertname": "ComplexSystemFailure",
                            "severity": "critical",
                            "namespace": "complex-system",
                            "affected_components": "database,cache,api,frontend"
                        },
                        "annotations": {
                            "description": "Multi-component system failure requiring coordinated remediation",
                            "summary": "Complex system restoration needed",
                            "components_affected": "4",
                            "estimated_steps": "6-8"
                        }
                    }]
                },
                "expected_detail_level": "high",
                "minimum_steps_expected": 4
            },
            {
                "scenario_name": "database_recovery_simulation",
                "alert_data": {
                    "alerts": [{
                        "status": "firing",
                        "labels": {
                            "alertname": "DatabaseConnectionFailure",
                            "severity": "critical",
                            "namespace": "database-tier",
                            "database": "postgresql-primary"
                        },
                        "annotations": {
                            "description": "Database connection pool exhausted",
                            "summary": "Database recovery required",
                            "recovery_type": "connection_pool_restart"
                        }
                    }]
                },
                "expected_detail_level": "medium",
                "minimum_steps_expected": 3
            }
        ]

    def test_detailed_simulation(self, scenario):
        """Test detailed dry-run simulation for a scenario"""
        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=scenario["alert_data"],
                headers={
                    'Content-Type': 'application/json',
                    'X-Dry-Run': 'true',
                    'X-Request-Detailed-Simulation': 'true',
                    'X-Scenario': scenario["scenario_name"]
                },
                timeout=35
            )

            end_time = time.time()

            result = {
                "scenario_name": scenario["scenario_name"],
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "response_text": response.text if response.text else "",
                "expected_detail_level": scenario["expected_detail_level"],
                "minimum_steps_expected": scenario["minimum_steps_expected"],
                "timestamp": start_time
            }

            # Analyze completeness
            if result["success"]:
                completeness_analysis = self.analyze_response_completeness(result["response_text"], scenario)
                result["completeness_analysis"] = completeness_analysis

            return result

        except Exception as e:
            return {
                "scenario_name": scenario["scenario_name"],
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def analyze_response_completeness(self, response_text, scenario):
        """Analyze the completeness of a dry-run simulation response"""
        if not response_text:
            return {"error": "Empty response"}

        # Use validator's completeness assessment
        validation = self.validator.validate_dry_run_response(response_text, scenario)
        completeness = validation["completeness_assessment"]

        # Additional detailed analysis
        response_lower = response_text.lower()

        # Count explicit steps
        step_patterns = [
            r'step \d+', r'\d+\.', r'first,', r'second,', r'then,', r'next,', r'finally'
        ]

        total_steps = 0
        for pattern in step_patterns:
            import re
            matches = re.findall(pattern, response_lower)
            total_steps += len(matches)

        # Check for detailed elements
        detailed_elements = {
            "kubectl_commands": len(re.findall(r'kubectl\s+\w+', response_lower)),
            "namespace_references": len(re.findall(r'namespace|ns', response_lower)),
            "resource_specifications": len(re.findall(r'deployment|pod|service|configmap', response_lower)),
            "action_verbs": len(re.findall(r'delete|create|scale|patch|restart', response_lower)),
            "explanatory_text": len(re.findall(r'because|since|to ensure|in order to', response_lower))
        }

        # Calculate detail score
        detail_score = (
            min(1.0, completeness["kubectl_commands_found"] / 3) * 0.3 +
            min(1.0, total_steps / scenario["minimum_steps_expected"]) * 0.3 +
            min(1.0, sum(detailed_elements.values()) / 10) * 0.2 +
            min(1.0, len(response_text) / 800) * 0.2  # Normalize to 800 chars
        )

        return {
            "completeness_from_validator": completeness,
            "step_count": total_steps,
            "meets_minimum_steps": total_steps >= scenario["minimum_steps_expected"],
            "detailed_elements": detailed_elements,
            "detail_score": detail_score,
            "detail_rating": "excellent" if detail_score >= 0.8 else "good" if detail_score >= 0.6 else "basic" if detail_score >= 0.4 else "insufficient",
            "response_length": len(response_text)
        }

    def analyze_simulation_completeness(self):
        """Analyze overall simulation completeness"""
        successful_results = [r for r in self.results if r["success"]]

        if not successful_results:
            return {"error": "No successful simulation results"}

        completeness_analyses = [r["completeness_analysis"] for r in successful_results if "completeness_analysis" in r]

        if not completeness_analyses:
            return {"error": "No completeness analyses available"}

        # Calculate statistics
        detail_scores = [ca["detail_score"] for ca in completeness_analyses]
        detail_ratings = [ca["detail_rating"] for ca in completeness_analyses]
        steps_met = [ca["meets_minimum_steps"] for ca in completeness_analyses]

        analysis = {
            "total_simulation_tests": len(self.results),
            "successful_simulations": len(successful_results),
            "simulation_success_rate": (len(successful_results) / len(self.results) * 100) if self.results else 0,
            "completeness_analyses_available": len(completeness_analyses),
            "average_detail_score": sum(detail_scores) / len(detail_scores) if detail_scores else 0,
            "minimum_steps_met_count": sum(1 for met in steps_met if met),
            "minimum_steps_compliance_rate": (sum(1 for met in steps_met if met) / len(steps_met) * 100) if steps_met else 0,
            "detail_rating_distribution": {}
        }

        # Rating distribution
        for rating in detail_ratings:
            analysis["detail_rating_distribution"][rating] = analysis["detail_rating_distribution"].get(rating, 0) + 1

        # Business requirement validation
        analysis["br_pa_010_completeness_compliance"] = {
            "requirement": "Complete simulation of remediation workflow without side effects",
            "average_detail_score": analysis["average_detail_score"],
            "minimum_steps_compliance": analysis["minimum_steps_compliance_rate"],
            "high_quality_simulations": analysis["detail_rating_distribution"].get("excellent", 0) + analysis["detail_rating_distribution"].get("good", 0),
            "pass": (analysis["average_detail_score"] >= 0.6 and
                    analysis["minimum_steps_compliance_rate"] >= 70.0),
            "simulation_quality": "high" if analysis["average_detail_score"] >= 0.7 else "adequate" if analysis["average_detail_score"] >= 0.5 else "needs_improvement"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = SimulationCompletenessTest(webhook_url)
    results = tester.test_simulation_completeness()

    # Save results
    with open(f"results/{test_session}/simulation_completeness_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    if "error" not in results:
        print(f"\n=== Simulation Completeness Results ===")
        print(f"Total Simulation Tests: {results['total_simulation_tests']}")
        print(f"Successful Simulations: {results['successful_simulations']}")
        print(f"Average Detail Score: {results['average_detail_score']:.3f}")
        print(f"Minimum Steps Met: {results['minimum_steps_met_count']}/{results['completeness_analyses_available']}")
        print(f"Steps Compliance Rate: {results['minimum_steps_compliance_rate']:.1f}%")

        print(f"\nDetail Rating Distribution:")
        for rating, count in results['detail_rating_distribution'].items():
            print(f"  {rating}: {count}")

        compliance = results["br_pa_010_completeness_compliance"]
        print(f"\n=== Completeness Compliance ===")
        print(f"Simulation Quality: {compliance['simulation_quality']}")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
    else:
        print(f"❌ Error: {results['error']}")