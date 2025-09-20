#!/usr/bin/env python3
"""
Provider Response Consistency Test
Tests that available LLM providers return consistent response formats
"""
import json
import requests
import time

class ProviderResponseTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []

    def create_test_alert(self, scenario_name, prompt_data):
        """Create a test alert that will trigger LLM analysis"""
        return {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "LLMProviderTest",
                    "severity": "warning",
                    "namespace": "test-workloads",
                    "test_scenario": scenario_name,
                    "llm_test_prompt": "true"
                },
                "annotations": {
                    "description": prompt_data["prompt"],
                    "summary": f"LLM provider test - {scenario_name}",
                    "test_category": prompt_data["category"]
                },
                "startsAt": "2025-01-01T10:00:00Z",
                "generatorURL": "http://prometheus:9090"
            }]
        }

    def test_provider_response_format(self, config_file):
        """Test response format consistency across providers"""
        # Load configuration
        with open(config_file, 'r') as f:
            config = json.load(f)

        # Load discovery results to see which providers are available
        try:
            with open(config_file.replace('llm_provider_config', 'provider_discovery_results'), 'r') as f:
                discovery_results = json.load(f)
                available_providers = [
                    p for p in discovery_results['provider_details']
                    if p['connectivity'] and p['test_priority'] == 'high'
                ]
        except:
            print("Warning: Could not load discovery results, testing all providers")
            available_providers = config['supported_providers']

        if not available_providers:
            return {"error": "No available providers to test"}

        test_prompts = config["test_prompts"]
        print(f"Testing response consistency with {len(test_prompts)} prompts on available providers")

        for prompt_data in test_prompts:
            print(f"Testing prompt: {prompt_data['name']}")

            # Create test alert with LLM prompt
            test_alert = self.create_test_alert(prompt_data['name'], prompt_data)

            # Send alert and capture response
            result = self.send_llm_test_alert(test_alert, prompt_data)
            self.results.append(result)

            # Small delay between tests
            time.sleep(2)

        return self.analyze_response_consistency()

    def send_llm_test_alert(self, test_alert, prompt_data):
        """Send test alert and analyze LLM response"""
        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=test_alert,
                headers={
                    'Content-Type': 'application/json',
                    'X-LLM-Test': 'true',
                    'X-Test-Category': prompt_data['category']
                },
                timeout=30  # Longer timeout for LLM processing
            )

            end_time = time.time()

            result = {
                "prompt_name": prompt_data["name"],
                "prompt_category": prompt_data["category"],
                "expected_elements": prompt_data["expected_elements"],
                "status_code": response.status_code,
                "success": response.status_code == 200,
                "response_time": end_time - start_time,
                "response_text": response.text[:1000] if response.text else "",  # First 1000 chars
                "timestamp": start_time
            }

            # Analyze response content for expected elements
            if result["success"] and result["response_text"]:
                result["content_analysis"] = self._analyze_response_content(
                    result["response_text"],
                    prompt_data["expected_elements"]
                )

            return result

        except Exception as e:
            return {
                "prompt_name": prompt_data["name"],
                "prompt_category": prompt_data["category"],
                "status_code": -1,
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def _analyze_response_content(self, response_text, expected_elements):
        """Analyze response content for expected elements"""
        response_lower = response_text.lower()

        element_analysis = {}
        for element in expected_elements:
            found = element.lower() in response_lower
            element_analysis[element] = found

        elements_found = sum(1 for found in element_analysis.values() if found)
        completeness_score = (elements_found / len(expected_elements)) * 100

        return {
            "element_analysis": element_analysis,
            "elements_found": elements_found,
            "total_elements": len(expected_elements),
            "completeness_score": completeness_score,
            "response_quality": "excellent" if completeness_score >= 75 else "good" if completeness_score >= 50 else "poor"
        }

    def analyze_response_consistency(self):
        """Analyze response consistency across tests"""
        successful_responses = [r for r in self.results if r["success"]]

        if not successful_responses:
            return {"error": "No successful LLM responses to analyze"}

        # Analyze response characteristics
        response_times = [r["response_time"] for r in successful_responses]
        content_analyses = [r.get("content_analysis", {}) for r in successful_responses if "content_analysis" in r]

        consistency_analysis = {
            "total_prompts_tested": len(self.results),
            "successful_responses": len(successful_responses),
            "success_rate": (len(successful_responses) / len(self.results) * 100) if self.results else 0,
            "average_response_time": sum(response_times) / len(response_times) if response_times else 0,
            "max_response_time": max(response_times) if response_times else 0,
            "min_response_time": min(response_times) if response_times else 0
        }

        # Content quality analysis
        if content_analyses:
            quality_scores = [ca.get("completeness_score", 0) for ca in content_analyses]
            consistency_analysis.update({
                "average_content_quality": sum(quality_scores) / len(quality_scores),
                "content_quality_consistency": "high" if all(q >= 50 for q in quality_scores) else "variable",
                "responses_with_expected_elements": len([ca for ca in content_analyses if ca.get("completeness_score", 0) >= 50])
            })

        # Business requirement validation
        consistency_analysis["br_pa_006_response_compliance"] = {
            "requirement": "Consistent response format across LLM providers",
            "success_rate": consistency_analysis["success_rate"],
            "response_quality": consistency_analysis.get("average_content_quality", 0),
            "consistency_rating": consistency_analysis.get("content_quality_consistency", "unknown"),
            "pass": (consistency_analysis["success_rate"] >= 80.0 and
                    consistency_analysis.get("average_content_quality", 0) >= 50.0),
            "provider_integration_quality": "excellent" if consistency_analysis["success_rate"] >= 90 else "good" if consistency_analysis["success_rate"] >= 80 else "needs_improvement"
        }

        return consistency_analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = ProviderResponseTest(webhook_url)
    results = tester.test_provider_response_format(f"results/{test_session}/llm_provider_config.json")

    # Save results
    with open(f"results/{test_session}/provider_response_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    if "error" not in results:
        print(f"\n=== Provider Response Consistency Results ===")
        print(f"Prompts Tested: {results['total_prompts_tested']}")
        print(f"Successful Responses: {results['successful_responses']}")
        print(f"Success Rate: {results['success_rate']:.1f}%")
        print(f"Average Response Time: {results['average_response_time']:.2f}s")

        if 'average_content_quality' in results:
            print(f"Average Content Quality: {results['average_content_quality']:.1f}%")
            print(f"Quality Consistency: {results['content_quality_consistency']}")

        compliance = results["br_pa_006_response_compliance"]
        print(f"\n=== Response Consistency Compliance ===")
        print(f"Requirement: {compliance['requirement']}")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
        print(f"Provider Integration Quality: {compliance['provider_integration_quality']}")
    else:
        print(f"❌ Error: {results['error']}")