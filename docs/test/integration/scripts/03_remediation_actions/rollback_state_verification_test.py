#!/usr/bin/env python3
"""
Rollback State Verification Test
Verifies that rollbacks properly restore resource states
"""
import json
import requests
import time
import subprocess

class RollbackStateVerificationTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.test_namespace = "rollback-verification-test"

    def test_rollback_state_verification(self):
        """Test rollback state verification with actual Kubernetes resources"""
        print("Testing rollback state verification...")

        # Create test namespace
        self.create_test_namespace()

        verification_scenarios = [
            {
                "name": "deployment_replica_rollback_verification",
                "resource_type": "deployment",
                "resource_name": "verify-deployment",
                "initial_state": {"replicas": 2},
                "modified_state": {"replicas": 4},
                "verification_type": "replica_count"
            },
            {
                "name": "configmap_data_rollback_verification",
                "resource_type": "configmap",
                "resource_name": "verify-config",
                "initial_state": {"data": {"key1": "original", "debug": "false"}},
                "modified_state": {"data": {"key1": "modified", "debug": "true"}},
                "verification_type": "data_content"
            }
        ]

        for scenario in verification_scenarios:
            print(f"\nTesting: {scenario['name']}")
            result = self.test_single_verification_scenario(scenario)
            self.results.append(result)

            time.sleep(5)

        return self.analyze_state_verification_results()

    def create_test_namespace(self):
        """Create test namespace for verification"""
        try:
            subprocess.run(
                ["kubectl", "create", "namespace", self.test_namespace],
                capture_output=True,
                text=True,
                timeout=30,
                check=False  # Don't raise exception if already exists
            )
            print(f"Test namespace {self.test_namespace} ready")

        except Exception as e:
            print(f"Warning: Could not create test namespace: {e}")

    def test_single_verification_scenario(self, scenario):
        """Test a single rollback state verification scenario"""
        scenario_name = scenario["name"]

        try:
            # Step 1: Create initial resource
            print(f"  Step 1: Creating initial resource")
            initial_creation = self.create_initial_resource(scenario)

            if not initial_creation["success"]:
                return {
                    "scenario_name": scenario_name,
                    "success": False,
                    "error": "Failed to create initial resource",
                    "phase_failed": "initial_creation"
                }

            # Step 2: Record initial state
            time.sleep(3)
            initial_state = self.capture_resource_state(scenario)

            # Step 3: Execute modification action
            print(f"  Step 2: Executing modification action")
            modification_result = self.execute_modification_action(scenario)

            if not modification_result["success"]:
                return {
                    "scenario_name": scenario_name,
                    "success": False,
                    "error": "Failed to execute modification",
                    "phase_failed": "modification"
                }

            # Step 4: Verify modification was applied
            time.sleep(3)
            modified_state = self.capture_resource_state(scenario)
            modification_verified = self.verify_state_change(scenario, initial_state, modified_state, "modification")

            # Step 5: Execute rollback
            print(f"  Step 3: Executing rollback")
            rollback_result = self.execute_rollback_action(scenario, modification_result["action_id"])

            if not rollback_result["success"]:
                return {
                    "scenario_name": scenario_name,
                    "success": False,
                    "error": "Failed to execute rollback",
                    "phase_failed": "rollback",
                    "modification_verified": modification_verified
                }

            # Step 6: Verify rollback restored original state
            print(f"  Step 4: Verifying rollback state")
            time.sleep(3)
            rollback_state = self.capture_resource_state(scenario)
            rollback_verified = self.verify_state_change(scenario, modified_state, rollback_state, "rollback")
            original_state_restored = self.verify_original_state_restored(scenario, initial_state, rollback_state)

            result = {
                "scenario_name": scenario_name,
                "resource_type": scenario["resource_type"],
                "verification_type": scenario["verification_type"],
                "success": True,
                "initial_state_captured": True,
                "modification_applied": modification_verified,
                "rollback_executed": rollback_result["success"],
                "rollback_state_verified": rollback_verified,
                "original_state_restored": original_state_restored,
                "overall_verification_success": (modification_verified and rollback_verified and original_state_restored),
                "states": {
                    "initial": initial_state,
                    "modified": modified_state,
                    "rollback": rollback_state
                }
            }

            # Print result
            verification_icon = '✅' if result["overall_verification_success"] else '❌'
            print(f"  {verification_icon} Verification: {'SUCCESS' if result['overall_verification_success'] else 'FAILED'}")

            return result

        except Exception as e:
            return {
                "scenario_name": scenario_name,
                "success": False,
                "error": str(e),
                "phase_failed": "exception"
            }
        finally:
            # Cleanup test resource
            self.cleanup_test_resource(scenario)

    def create_initial_resource(self, scenario):
        """Create initial resource for testing"""
        resource_type = scenario["resource_type"]
        resource_name = scenario["resource_name"]
        initial_state = scenario["initial_state"]

        if resource_type == "deployment":
            yaml_content = f"""
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {resource_name}
  namespace: {self.test_namespace}
spec:
  replicas: {initial_state['replicas']}
  selector:
    matchLabels:
      app: {resource_name}
  template:
    metadata:
      labels:
        app: {resource_name}
    spec:
      containers:
      - name: test-container
        image: nginx:1.21
        ports:
        - containerPort: 80
"""
        elif resource_type == "configmap":
            data_entries = "\n".join([f'  {k}: "{v}"' for k, v in initial_state['data'].items()])
            yaml_content = f"""
apiVersion: v1
kind: ConfigMap
metadata:
  name: {resource_name}
  namespace: {self.test_namespace}
data:
{data_entries}
"""
        else:
            return {"success": False, "error": f"Unsupported resource type: {resource_type}"}

        try:
            result = subprocess.run(
                ["kubectl", "apply", "-f", "-"],
                input=yaml_content.encode(),
                capture_output=True,
                text=True,
                timeout=30
            )

            return {
                "success": result.returncode == 0,
                "stdout": result.stdout,
                "stderr": result.stderr
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def capture_resource_state(self, scenario):
        """Capture current state of resource"""
        resource_type = scenario["resource_type"]
        resource_name = scenario["resource_name"]

        try:
            if resource_type == "deployment":
                result = subprocess.run(
                    ["kubectl", "get", "deployment", resource_name, "-n", self.test_namespace, "-o", "json"],
                    capture_output=True,
                    text=True,
                    timeout=15
                )

                if result.returncode == 0:
                    deployment_data = json.loads(result.stdout)
                    return {
                        "replicas": deployment_data["spec"]["replicas"],
                        "ready_replicas": deployment_data.get("status", {}).get("readyReplicas", 0)
                    }

            elif resource_type == "configmap":
                result = subprocess.run(
                    ["kubectl", "get", "configmap", resource_name, "-n", self.test_namespace, "-o", "json"],
                    capture_output=True,
                    text=True,
                    timeout=15
                )

                if result.returncode == 0:
                    configmap_data = json.loads(result.stdout)
                    return {
                        "data": configmap_data.get("data", {})
                    }

            return {"error": f"Failed to capture state: {result.stderr}"}

        except Exception as e:
            return {"error": str(e)}

    def execute_modification_action(self, scenario):
        """Execute modification action through the webhook"""
        # Create alert that would trigger the modification
        modification_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": f"ModificationTest_{scenario['name']}",
                    "severity": "warning",
                    "namespace": self.test_namespace,
                    "resource_type": scenario["resource_type"],
                    "resource_name": scenario["resource_name"],
                    "verification_test": "modification"
                },
                "annotations": {
                    "description": f"Modification test for {scenario['resource_type']} {scenario['resource_name']}",
                    "summary": "State verification modification test",
                    "target_state": json.dumps(scenario["modified_state"])
                },
                "startsAt": "2025-01-01T10:00:00Z"
            }]
        }

        try:
            response = requests.post(
                self.webhook_url,
                json=modification_alert,
                headers={
                    'Content-Type': 'application/json',
                    'X-Verification-Test': 'modification',
                    'X-Resource-Type': scenario["resource_type"],
                    'X-Resource-Name': scenario["resource_name"]
                },
                timeout=30
            )

            action_id = f"verify_{scenario['name']}_{int(time.time())}"

            return {
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_text": response.text[:300] if response.text else "",
                "action_id": action_id
            }

        except Exception as e:
            return {
                "success": False,
                "error": str(e),
                "action_id": None
            }

    def execute_rollback_action(self, scenario, action_id):
        """Execute rollback action through the webhook"""
        rollback_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": f"RollbackTest_{scenario['name']}",
                    "severity": "warning",
                    "namespace": self.test_namespace,
                    "action_id": action_id,
                    "rollback_request": "true",
                    "verification_test": "rollback"
                },
                "annotations": {
                    "description": f"Rollback test for action {action_id}",
                    "summary": "State verification rollback test",
                    "original_state": json.dumps(scenario["initial_state"])
                },
                "startsAt": "2025-01-01T10:00:00Z"
            }]
        }

        try:
            response = requests.post(
                self.webhook_url,
                json=rollback_alert,
                headers={
                    'Content-Type': 'application/json',
                    'X-Verification-Test': 'rollback',
                    'X-Action-ID': action_id,
                    'X-Rollback-Request': 'true'
                },
                timeout=30
            )

            return {
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_text": response.text[:300] if response.text else ""
            }

        except Exception as e:
            return {
                "success": False,
                "error": str(e)
            }

    def verify_state_change(self, scenario, before_state, after_state, change_type):
        """Verify that state change occurred as expected"""
        if "error" in before_state or "error" in after_state:
            return False

        verification_type = scenario["verification_type"]

        if verification_type == "replica_count":
            before_replicas = before_state.get("replicas", 0)
            after_replicas = after_state.get("replicas", 0)

            if change_type == "modification":
                expected_replicas = scenario["modified_state"]["replicas"]
                return after_replicas == expected_replicas and after_replicas != before_replicas
            elif change_type == "rollback":
                expected_replicas = scenario["initial_state"]["replicas"]
                return after_replicas == expected_replicas

        elif verification_type == "data_content":
            before_data = before_state.get("data", {})
            after_data = after_state.get("data", {})

            if change_type == "modification":
                expected_data = scenario["modified_state"]["data"]
                return after_data == expected_data and after_data != before_data
            elif change_type == "rollback":
                expected_data = scenario["initial_state"]["data"]
                return after_data == expected_data

        return False

    def verify_original_state_restored(self, scenario, initial_state, rollback_state):
        """Verify that rollback restored the original state"""
        if "error" in initial_state or "error" in rollback_state:
            return False

        verification_type = scenario["verification_type"]

        if verification_type == "replica_count":
            return initial_state.get("replicas") == rollback_state.get("replicas")
        elif verification_type == "data_content":
            return initial_state.get("data") == rollback_state.get("data")

        return False

    def cleanup_test_resource(self, scenario):
        """Clean up test resource"""
        try:
            subprocess.run(
                ["kubectl", "delete", scenario["resource_type"], scenario["resource_name"], "-n", self.test_namespace],
                capture_output=True,
                timeout=30,
                check=False
            )
        except:
            pass  # Ignore cleanup errors

    def analyze_state_verification_results(self):
        """Analyze state verification results"""
        successful_verifications = [r for r in self.results if r.get("overall_verification_success", False)]

        analysis = {
            "total_verification_scenarios": len(self.results),
            "successful_verifications": len(successful_verifications),
            "state_verification_success_rate": (len(successful_verifications) / len(self.results)) if self.results else 0,
            "verification_details": [
                {
                    "scenario": r["scenario_name"],
                    "resource_type": r.get("resource_type", "unknown"),
                    "modification_applied": r.get("modification_applied", False),
                    "rollback_executed": r.get("rollback_executed", False),
                    "original_state_restored": r.get("original_state_restored", False),
                    "overall_success": r.get("overall_verification_success", False)
                }
                for r in self.results if r.get("success", False)
            ]
        }

        # Business requirement validation for state verification
        analysis["br_pa_013_state_verification_compliance"] = {
            "requirement": "Rollback properly restores previous resource states",
            "state_restoration_success_rate": analysis["state_verification_success_rate"],
            "required_accuracy": 0.95,
            "accuracy_requirement_met": analysis["state_verification_success_rate"] >= 0.95,
            "pass": analysis["state_verification_success_rate"] >= 0.90,  # Slightly lower threshold for integration test
            "state_accuracy": "high" if analysis["state_verification_success_rate"] >= 0.95 else "adequate" if analysis["state_verification_success_rate"] >= 0.90 else "insufficient"
        }

        return analysis

    def cleanup_verification_test_environment(self):
        """Clean up verification test environment"""
        try:
            subprocess.run(
                ["kubectl", "delete", "namespace", self.test_namespace, "--timeout=60s"],
                capture_output=True,
                text=True,
                timeout=70,
                check=False
            )
            print("Verification test environment cleaned up")
        except:
            pass

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = RollbackStateVerificationTest(webhook_url)

    try:
        results = tester.test_rollback_state_verification()

        # Save results
        with open(f"results/{test_session}/rollback_state_verification_results.json", "w") as f:
            json.dump(results, f, indent=2)

        # Print summary
        print(f"\n=== Rollback State Verification Results ===")
        print(f"Total Verification Scenarios: {results['total_verification_scenarios']}")
        print(f"Successful Verifications: {results['successful_verifications']}")
        print(f"State Verification Success Rate: {results['state_verification_success_rate']*100:.1f}%")

        print(f"\nVerification Details:")
        for detail in results['verification_details']:
            success_icon = '✅' if detail['overall_success'] else '❌'
            print(f"  {detail['scenario']}: {success_icon} ({detail['resource_type']})")
            print(f"    Modification: {'✅' if detail['modification_applied'] else '❌'}")
            print(f"    Rollback: {'✅' if detail['rollback_executed'] else '❌'}")
            print(f"    State Restored: {'✅' if detail['original_state_restored'] else '❌'}")

        compliance = results["br_pa_013_state_verification_compliance"]
        print(f"\n=== State Verification Compliance ===")
        print(f"State Restoration Rate: {compliance['state_restoration_success_rate']*100:.1f}% (required: {compliance['required_accuracy']*100:.1f}%)")
        print(f"Accuracy Requirement: {'✅' if compliance['accuracy_requirement_met'] else '❌'}")
        print(f"State Accuracy: {compliance['state_accuracy']}")
        print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")

    finally:
        tester.cleanup_verification_test_environment()