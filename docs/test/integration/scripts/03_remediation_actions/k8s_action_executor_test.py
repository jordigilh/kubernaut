#!/usr/bin/env python3
"""
Kubernetes Action Executor Test Framework
Tests execution of various Kubernetes remediation actions
"""
import json
import requests
import time
import subprocess
import uuid

class KubernetesActionExecutorTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.test_namespace = "k8s-actions-test"
        self.test_resources = {}

    def prepare_test_environment(self):
        """Prepare test environment with required Kubernetes resources"""
        print("Preparing test environment...")

        # Create test deployment
        deployment_yaml = f"""
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: {self.test_namespace}
spec:
  replicas: 2
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: test-container
        image: nginx:1.21
        ports:
        - containerPort: 80
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 200m
            memory: 256Mi
"""

        # Create test service
        service_yaml = f"""
apiVersion: v1
kind: Service
metadata:
  name: test-service
  namespace: {self.test_namespace}
spec:
  selector:
    app: test-app
  ports:
  - port: 80
    targetPort: 80
  type: ClusterIP
"""

        # Create test configmap
        configmap_yaml = f"""
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: {self.test_namespace}
data:
  app.properties: |
    debug=true
    logging.level=INFO
"""

        try:
            # Apply test resources
            for name, yaml_content in [("deployment", deployment_yaml), ("service", service_yaml), ("configmap", configmap_yaml)]:
                result = subprocess.run(
                    ["kubectl", "apply", "-f", "-"],
                    input=yaml_content.encode(),
                    capture_output=True,
                    text=True,
                    timeout=30
                )

                if result.returncode == 0:
                    print(f"  ✅ Created test {name}")
                    self.test_resources[name] = True
                else:
                    print(f"  ❌ Failed to create test {name}: {result.stderr}")
                    self.test_resources[name] = False

            # Wait for resources to be ready
            time.sleep(10)
            return True

        except Exception as e:
            print(f"Error preparing test environment: {e}")
            return False

    def test_kubernetes_actions(self, actions_data_file):
        """Test execution of various Kubernetes actions"""
        with open(actions_data_file, 'r') as f:
            config = json.load(f)

        categories = config["kubernetes_action_categories"]
        success_criteria = config["success_criteria"]

        print(f"Testing Kubernetes action execution across {len(categories)} categories...")

        # Prepare test environment
        if not self.prepare_test_environment():
            return {"error": "Failed to prepare test environment"}

        all_actions = []
        for category in categories:
            all_actions.extend(category["actions"])

        print(f"Total actions to test: {len(all_actions)}")

        # Test each action
        for i, action in enumerate(all_actions, 1):
            print(f"\n[{i}/{len(all_actions)}] Testing: {action['action_type']}")
            result = self.test_single_action(action)
            self.results.append(result)

            time.sleep(2)  # Brief delay between actions

        return self.analyze_action_execution_results(success_criteria)

    def test_single_action(self, action):
        """Test execution of a single Kubernetes action"""
        action_type = action["action_type"]
        test_scenario = action["test_scenario"]

        # Create alert that should trigger this specific action
        test_alert = self.create_action_specific_alert(action)

        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=test_alert,
                headers={
                    'Content-Type': 'application/json',
                    'X-Test-Action': action_type,
                    'X-Test-Scenario': test_scenario,
                    'X-Target-Namespace': self.test_namespace
                },
                timeout=45  # Longer timeout for K8s operations
            )

            end_time = time.time()

            result = {
                "action_type": action_type,
                "test_scenario": test_scenario,
                "kubectl_equivalent": action["kubectl_equivalent"],
                "expected_outcome": action["expected_outcome"],
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "response_text": response.text[:1000] if response.text else "",
                "timestamp": start_time
            }

            # Validate action execution
            if result["success"]:
                validation = self.validate_action_execution(action, result["response_text"])
                result["execution_validation"] = validation

            status = '✅' if result["success"] else '❌'
            print(f"  {status} {action_type}: {'SUCCESS' if result['success'] else f'FAILED (HTTP {response.status_code})'}")

            return result

        except Exception as e:
            error_result = {
                "action_type": action_type,
                "test_scenario": test_scenario,
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

            print(f"  ❌ {action_type}: ERROR - {str(e)}")
            return error_result

    def create_action_specific_alert(self, action):
        """Create an alert designed to trigger a specific action"""
        action_type = action["action_type"]
        test_scenario = action["test_scenario"]

        # Map action types to appropriate alert scenarios
        alert_templates = {
            "delete_pod": {
                "alertname": "PodUnresponsive",
                "description": f"Pod in namespace {self.test_namespace} is unresponsive and needs restart",
                "target_resource": "test-app"
            },
            "scale_deployment": {
                "alertname": "HighLoad",
                "description": f"Deployment in namespace {self.test_namespace} needs scaling",
                "target_resource": "test-app"
            },
            "describe_pod": {
                "alertname": "PodInvestigation",
                "description": f"Pod in namespace {self.test_namespace} needs investigation",
                "target_resource": "test-app"
            },
            "get_pod_logs": {
                "alertname": "PodFailure",
                "description": f"Pod in namespace {self.test_namespace} failing, need logs",
                "target_resource": "test-app"
            },
            "describe_service": {
                "alertname": "ServiceEndpointIssue",
                "description": f"Service in namespace {self.test_namespace} has endpoint issues",
                "target_resource": "test-service"
            }
        }

        # Use specific template if available, otherwise generic
        template = alert_templates.get(action_type, {
            "alertname": "GenericKubernetesIssue",
            "description": f"Kubernetes issue requiring {action_type}",
            "target_resource": "test-app"
        })

        return {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": template["alertname"],
                    "severity": "warning",
                    "namespace": self.test_namespace,
                    "target_action": action_type,
                    "test_resource": template["target_resource"]
                },
                "annotations": {
                    "description": template["description"],
                    "summary": f"Test scenario: {test_scenario}",
                    "expected_action": action_type,
                    "kubectl_equivalent": action["kubectl_equivalent"]
                },
                "startsAt": "2025-01-01T10:00:00Z"
            }]
        }

    def validate_action_execution(self, action, response_text):
        """Validate that the action was properly executed"""
        action_type = action["action_type"]
        expected_outcome = action["expected_outcome"]

        validation = {
            "action_mentioned": action_type.replace("_", " ") in response_text.lower(),
            "kubectl_command_present": any(cmd in response_text.lower() for cmd in ["kubectl", "k8s", "kubernetes"]),
            "namespace_referenced": self.test_namespace in response_text.lower(),
            "expected_outcome_indicated": False
        }

        # Check for expected outcome indicators
        outcome_indicators = {
            "pod_deleted_and_recreated": ["delete", "restart", "recreate"],
            "deployment_scaled": ["scale", "replicas", "scaled"],
            "detailed_pod_information": ["describe", "details", "information"],
            "pod_logs_retrieved": ["logs", "log", "output"],
            "service_details_retrieved": ["service", "details", "endpoints"],
            "node_details_retrieved": ["node", "details", "status"],
            "events_retrieved": ["events", "event", "history"]
        }

        indicators = outcome_indicators.get(expected_outcome, [expected_outcome.split("_")])
        validation["expected_outcome_indicated"] = any(
            indicator in response_text.lower() for indicator in indicators
        )

        # Overall execution validation
        validation["execution_likely"] = (
            validation["kubectl_command_present"] and
            validation["namespace_referenced"] and
            (validation["action_mentioned"] or validation["expected_outcome_indicated"])
        )

        return validation

    def analyze_action_execution_results(self, success_criteria):
        """Analyze Kubernetes action execution results"""
        total_actions = len(self.results)
        successful_actions = [r for r in self.results if r["success"]]
        failed_actions = [r for r in self.results if not r["success"]]

        # Calculate success rate
        success_rate = (len(successful_actions) / total_actions) if total_actions > 0 else 0

        # Analyze by category
        action_type_results = {}
        for result in self.results:
            action_type = result["action_type"]
            if action_type not in action_type_results:
                action_type_results[action_type] = {"total": 0, "successful": 0}

            action_type_results[action_type]["total"] += 1
            if result["success"]:
                action_type_results[action_type]["successful"] += 1

        # Response time statistics
        response_times = [r["response_time"] for r in successful_actions if "response_time" in r]

        analysis = {
            "total_actions_tested": total_actions,
            "successful_actions": len(successful_actions),
            "failed_actions": len(failed_actions),
            "success_rate": success_rate,
            "success_rate_percentage": success_rate * 100,
            "unique_action_types": len(action_type_results),
            "action_type_breakdown": action_type_results,
            "failed_action_details": [
                {
                    "action_type": r["action_type"],
                    "error": r.get("error", f"HTTP {r.get('status_code', 'unknown')}")
                }
                for r in failed_actions
            ],
            "response_time_statistics": {
                "mean": sum(response_times) / len(response_times) if response_times else 0,
                "max": max(response_times) if response_times else 0,
                "min": min(response_times) if response_times else 0
            }
        }

        # Business requirement validation
        analysis["br_pa_011_compliance"] = {
            "requirement": "Execute 25+ types of Kubernetes remediation actions with 95% success rate",
            "action_types_tested": analysis["unique_action_types"],
            "minimum_action_types_met": analysis["unique_action_types"] >= success_criteria["minimum_action_types"],
            "success_rate": analysis["success_rate"],
            "required_success_rate_met": analysis["success_rate"] >= success_criteria["required_success_rate"],
            "maximum_failures": len(failed_actions),
            "within_failure_tolerance": len(failed_actions) <= success_criteria["maximum_acceptable_failures"],
            "pass": (analysis["unique_action_types"] >= success_criteria["minimum_action_types"] and
                    analysis["success_rate"] >= success_criteria["required_success_rate"]),
            "k8s_action_capability": "comprehensive" if analysis["unique_action_types"] >= 25 and analysis["success_rate"] >= 0.95 else "adequate" if analysis["unique_action_types"] >= 20 and analysis["success_rate"] >= 0.90 else "insufficient"
        }

        return analysis

    def cleanup_test_environment(self):
        """Clean up test environment resources"""
        print("Cleaning up test environment...")
        try:
            # Delete test namespace (this removes all resources in it)
            result = subprocess.run(
                ["kubectl", "delete", "namespace", self.test_namespace, "--timeout=60s"],
                capture_output=True,
                text=True,
                timeout=70
            )

            if result.returncode == 0:
                print("  ✅ Test environment cleaned up successfully")
            else:
                print(f"  ⚠️ Cleanup warning: {result.stderr}")

        except Exception as e:
            print(f"  ⚠️ Cleanup error: {e}")

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = KubernetesActionExecutorTest(webhook_url)

    try:
        results = tester.test_kubernetes_actions(f"results/{test_session}/k8s_actions_test_data.json")

        # Save results
        with open(f"results/{test_session}/k8s_action_execution_results.json", "w") as f:
            json.dump(results, f, indent=2)

        # Print summary
        if "error" not in results:
            print(f"\n=== Kubernetes Action Execution Results ===")
            print(f"Total Actions Tested: {results['total_actions_tested']}")
            print(f"Successful Actions: {results['successful_actions']}")
            print(f"Failed Actions: {results['failed_actions']}")
            print(f"Success Rate: {results['success_rate_percentage']:.1f}%")
            print(f"Unique Action Types: {results['unique_action_types']}")

            if results['failed_action_details']:
                print(f"\nFailed Actions:")
                for failure in results['failed_action_details']:
                    print(f"  - {failure['action_type']}: {failure['error']}")

            print(f"\nResponse Time Statistics:")
            stats = results['response_time_statistics']
            print(f"  Mean: {stats['mean']:.2f}s")
            print(f"  Range: {stats['min']:.2f}s - {stats['max']:.2f}s")

            compliance = results["br_pa_011_compliance"]
            print(f"\n=== BR-PA-011 Compliance ===")
            print(f"Action Types: {compliance['action_types_tested']} (min: 25) {'✅' if compliance['minimum_action_types_met'] else '❌'}")
            print(f"Success Rate: {compliance['success_rate']:.1%} (min: 95%) {'✅' if compliance['required_success_rate_met'] else '❌'}")
            print(f"K8s Capability: {compliance['k8s_action_capability']}")
            print(f"Overall Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
        else:
            print(f"❌ Error: {results['error']}")

    finally:
        # Always cleanup
        tester.cleanup_test_environment()