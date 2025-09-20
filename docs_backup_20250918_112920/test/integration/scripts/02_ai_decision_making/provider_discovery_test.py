#!/usr/bin/env python3
"""
LLM Provider Discovery Test
Discovers and tests connectivity to configured LLM providers
"""
import json
import requests
import time
from urllib.parse import urljoin

class ProviderDiscoveryTest:
    def __init__(self):
        self.results = []

    def load_provider_config(self, config_file):
        """Load provider configuration"""
        with open(config_file, 'r') as f:
            return json.load(f)

    def test_provider_connectivity(self, provider_config):
        """Test basic connectivity to an LLM provider"""
        provider_name = provider_config["name"]

        try:
            # Different connectivity tests based on provider type
            if provider_config["name"] == "ollama":
                return self._test_ollama_connectivity(provider_config)
            elif provider_config["name"] == "openai":
                return self._test_openai_connectivity(provider_config)
            else:
                # Generic external provider test
                return self._test_generic_provider_connectivity(provider_config)

        except Exception as e:
            return {
                "provider": provider_name,
                "connectivity": False,
                "error": str(e),
                "status": "ERROR"
            }

    def _test_ollama_connectivity(self, provider_config):
        """Test Ollama-specific connectivity"""
        try:
            # Test Ollama API endpoint
            response = requests.get(
                f"{provider_config['endpoint']}/api/tags",
                timeout=10
            )

            available = response.status_code == 200
            models = []

            if available:
                try:
                    models = response.json().get('models', [])
                except:
                    models = []

            return {
                "provider": "ollama",
                "connectivity": available,
                "status": "AVAILABLE" if available else "UNAVAILABLE",
                "response_time": response.elapsed.total_seconds(),
                "models_found": len(models),
                "models": [model.get('name', 'unknown') for model in models[:5]]  # First 5 models
            }

        except requests.exceptions.Timeout:
            return {
                "provider": "ollama",
                "connectivity": False,
                "status": "TIMEOUT",
                "error": "Connection timeout"
            }
        except Exception as e:
            return {
                "provider": "ollama",
                "connectivity": False,
                "status": "ERROR",
                "error": str(e)
            }

    def _test_openai_connectivity(self, provider_config):
        """Test OpenAI-specific connectivity (without API key)"""
        try:
            # Test without API key - should get 401 but confirms endpoint is reachable
            response = requests.get(
                f"{provider_config['endpoint']}/models",
                timeout=10
            )

            # 401 means endpoint is reachable but authentication required
            reachable = response.status_code in [401, 403, 200]

            return {
                "provider": "openai",
                "connectivity": reachable,
                "status": "REACHABLE_NO_AUTH" if response.status_code == 401 else "AVAILABLE" if response.status_code == 200 else "UNAVAILABLE",
                "response_time": response.elapsed.total_seconds(),
                "status_code": response.status_code
            }

        except Exception as e:
            return {
                "provider": "openai",
                "connectivity": False,
                "status": "UNREACHABLE",
                "error": str(e)
            }

    def _test_generic_provider_connectivity(self, provider_config):
        """Generic connectivity test for external providers"""
        try:
            # Basic connectivity test
            response = requests.head(
                provider_config["endpoint"],
                timeout=10
            )

            reachable = response.status_code < 500

            return {
                "provider": provider_config["name"],
                "connectivity": reachable,
                "status": "REACHABLE" if reachable else "UNAVAILABLE",
                "response_time": response.elapsed.total_seconds(),
                "status_code": response.status_code
            }

        except Exception as e:
            return {
                "provider": provider_config["name"],
                "connectivity": False,
                "status": "UNREACHABLE",
                "error": str(e)
            }

    def test_all_providers(self, config_file):
        """Test connectivity to all configured providers"""
        config = self.load_provider_config(config_file)
        providers = config["supported_providers"]

        print(f"Testing connectivity to {len(providers)} LLM providers...")

        for provider_config in providers:
            print(f"Testing {provider_config['name']}...")
            result = self.test_provider_connectivity(provider_config)
            result["expected_available"] = provider_config["expected_available"]
            result["test_priority"] = provider_config["test_priority"]

            self.results.append(result)

            status = result["status"]
            print(f"  {provider_config['name']}: {status}")

        return self.analyze_provider_results()

    def analyze_provider_results(self):
        """Analyze provider connectivity results"""
        total_providers = len(self.results)
        available_providers = [r for r in self.results if r["connectivity"]]
        high_priority_providers = [r for r in self.results if r["test_priority"] == "high"]

        analysis = {
            "total_providers_tested": total_providers,
            "available_providers": len(available_providers),
            "unavailable_providers": total_providers - len(available_providers),
            "availability_rate": (len(available_providers) / total_providers * 100) if total_providers > 0 else 0,
            "high_priority_available": len([r for r in high_priority_providers if r["connectivity"]]),
            "provider_details": self.results
        }

        # Business requirement validation
        analysis["br_pa_006_compliance"] = {
            "requirement": "Support multiple LLM providers (6 providers minimum)",
            "total_providers_configured": total_providers,
            "minimum_required": 6,
            "providers_configured_compliant": total_providers >= 6,
            "available_providers": len(available_providers),
            "high_priority_functional": len([r for r in high_priority_providers if r["connectivity"]]) > 0,
            "pass": (total_providers >= 6 and
                    len(available_providers) >= 1 and  # At least one provider must work
                    len([r for r in high_priority_providers if r["connectivity"]]) > 0),  # High priority must work
            "provider_diversity": "excellent" if len(available_providers) >= 3 else "adequate" if len(available_providers) >= 1 else "insufficient"
        }

        return analysis

if __name__ == "__main__":
    import sys

    test_session = sys.argv[1] if len(sys.argv) > 1 else "test_session"

    tester = ProviderDiscoveryTest()
    results = tester.test_all_providers(f"results/{test_session}/llm_provider_config.json")

    # Save results
    with open(f"results/{test_session}/provider_discovery_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== LLM Provider Discovery Results ===")
    print(f"Total Providers Tested: {results['total_providers_tested']}")
    print(f"Available Providers: {results['available_providers']}")
    print(f"Availability Rate: {results['availability_rate']:.1f}%")
    print(f"High Priority Available: {results['high_priority_available']}")

    print(f"\nProvider Status Details:")
    for provider in results['provider_details']:
        print(f"  {provider['provider']}: {provider['status']} ({'High' if provider['test_priority'] == 'high' else 'Med/Low'} priority)")

    compliance = results["br_pa_006_compliance"]
    print(f"\n=== BR-PA-006 Compliance ===")
    print(f"Requirement: {compliance['requirement']}")
    print(f"Providers Configured: {compliance['total_providers_configured']} (minimum: {compliance['minimum_required']})")
    print(f"Configuration Compliant: {'✅' if compliance['providers_configured_compliant'] else '❌'}")
    print(f"Functional Providers: {compliance['available_providers']}")
    print(f"High Priority Functional: {'✅' if compliance['high_priority_functional'] else '❌'}")
    print(f"Provider Diversity: {compliance['provider_diversity']}")
    print(f"Overall Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")