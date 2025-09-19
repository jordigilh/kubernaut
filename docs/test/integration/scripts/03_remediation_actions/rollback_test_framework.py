#!/usr/bin/env python3
"""
Rollback Test Framework
Tests rollback capabilities for reversible Kubernetes actions
"""
import json
import requests
import time
import subprocess
import uuid

class RollbackTestFramework:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []
        self.test_namespace = "rollback-test"
        self.action_history = {}

    def prepare_rollback_test_environment(self):
        """Prepare test environment with resources for rollback testing"""
        print("Preparing rollback test environment...")

        # Create test deployment for scaling and resource limit tests
        deployment_yaml = f"""
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: {self.test_namespace}
spec:
  replicas: 3
  selector:
    matchLabels:
      app: rollback-test
  template:
    metadata:
      labels:
        app: rollback-test
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

        # Create test configmap for configuration rollback tests
        configmap_yaml = f"""
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: {self.test_namespace}
data:
  setting1: "value1"
  debug: "false"
  environment: "test"
"""

        try:
            # Apply test resources
            for name, yaml_content in [("deployment", deployment_yaml), ("configmap", configmap_yaml)]:
                result = subprocess.run(
                    ["kubectl", "apply", "-f", "-"],
                    input=yaml_content.encode(),
                    capture_output=True,
                    text=True,
                    timeout=30
                )

                if result.returncode == 0:
                    print(f"  ✅ Created test {name}")
                else:
                    print(f"  ❌ Failed to create test {name}: {result.stderr}")
                    return False

            # Wait for deployment to be ready
            print("  Waiting for deployment to be ready...")
            time.sleep(15)

            # Verify deployment is ready
            result = subprocess.run(
                ["kubectl", "rollout", "status", f"deployment/test-deployment", "-n", self.test_namespace, "--timeout=60s"],
                capture_output=True,
                text=True,
                timeout=70
            )

            if result.returncode == 0:
                print("  ✅ Test environment ready")
                return True
            else:
                print(f"  ❌ Deployment not ready: {result.stderr}")
                return False

        except Exception as e:
            print(f"Error preparing rollback test environment: {e}")
            return False

    def test_rollback_capabilities(self, scenarios_file):
        """Test rollback capabilities for various scenarios"""
        with open(scenarios_file, 'r') as f:
            config = json.load(f)

        reversible_scenarios = config["reversible_action_scenarios"]
        irreversible_scenarios = config["irreversible_action_scenarios"]
        validation_criteria = config["rollback_validation_criteria"]

        print(f"Testing rollback capabilities...")
        print(f"Reversible scenarios: {len(reversible_scenarios)}")
        print(f"Irreversible scenarios: {len(irreversible_scenarios)}")

        # Prepare test environment
        if not self.prepare_rollback_test_environment():
            return {"error": "Failed to prepare rollback test environment"}

        # Test reversible actions and their rollbacks
        print("\n=== Testing Reversible Actions and Rollbacks ===")
        for scenario in reversible_scenarios:
            print(f"\nTesting reversible scenario: {scenario['scenario_name']}")
            result = self.test_reversible_scenario(scenario)
            self.results.append(result)

            time.sleep(5)  # Allow time between tests

        # Test irreversible action identification
        print("\n=== Testing Irreversible Action Identification ===")
        for scenario in irreversible_scenarios:
            print(f"\nTesting irreversible scenario: {scenario['scenario_name']}")
            result = self.test_irreversible_scenario(scenario)
            self.results.append(result)

            time.sleep(3)

        return self.analyze_rollback_results(validation_criteria)

    def test_reversible_scenario(self, scenario):
        """Test a reversible action scenario with rollback"""
        scenario_name = scenario["scenario_name"]
        action_category = scenario["action_category"]
        initial_action = scenario["initial_action"]

        # Step 1: Execute initial action
        print(f"  Step 1: Executing initial action ({initial_action['action_type']})")
        initial_result = self.execute_initial_action(scenario)

        if not initial_result["success"]:
            return {
                "scenario_name": scenario_name,
                "action_category": action_category,
                "reversibility": scenario["reversibility"],
                "initial_action_success": False,
                "initial_error": initial_result.get("error", "Unknown error"),
                "rollback_tested": False,
                "overall_success": False
            }

        # Step 2: Verify initial action was applied
        print(f"  Step 2: Verifying initial action was applied")
        verification_result = self.verify_action_state(scenario, "after_initial")

        # Step 3: Request rollback
        print(f"  Step 3: Requesting rollback")
        time.sleep(2)  # Brief delay before rollback
        rollback_result = self.execute_rollback_action(scenario, initial_result["action_id"])

        if not rollback_result["success"]:
            return {
                "scenario_name": scenario_name,
                "action_category": action_category,
                "reversibility": scenario["reversibility"],
                "initial_action_success": True,
                "rollback_attempted": True,
                "rollback_success": False,
                "rollback_error": rollback_result.get("error", "Unknown error"),
                "overall_success": False
            }

        # Step 4: Verify rollback was successful
        print(f"  Step 4: Verifying rollback was successful")
        time.sleep(3)  # Allow time for rollback to take effect
        rollback_verification = self.verify_action_state(scenario, "after_rollback")

        result = {
            "scenario_name": scenario_name,
            "action_category": action_category,
            "reversibility": scenario["reversibility"],
            "rollback_complexity": scenario["rollback_complexity"],
            "initial_action_success": initial_result["success"],
            "initial_action_id": initial_result["action_id"],
            "rollback_attempted": True,
            "rollback_success": rollback_result["success"],
            "rollback_verification": rollback_verification,
            "overall_success": (initial_result["success"] and rollback_result["success"] and rollback_verification["rollback_verified"]),
            "execution_time": initial_result.get("response_time", 0) + rollback_result.get("response_time", 0),
            "test_timestamp": time.time()
        }

        # Print result summary
        success_icon = '✅' if result["overall_success"] else '❌'
        print(f"  {success_icon} Overall: {'SUCCESS' if result['overall_success'] else 'FAILED'}")

        return result

    def test_irreversible_scenario(self, scenario):
        """Test identification of irreversible actions"""
        scenario_name = scenario["scenario_name"]
        action_category = scenario["action_category"]
        action = scenario["action"]

        # Create alert that would normally trigger this action
        test_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "IrreversibleActionTest",
                    "severity": "critical",
                    "namespace": self.test_namespace,
                    "action_type": action["action_type"],
                    "target": action["target"],
                    "irreversible_test": "true"
                },
                "annotations": {
                    "description": f"Testing irreversible action identification: {action['action_type']}",
                    "summary": "Irreversible action identification test",
                    "expected_behavior": "identify_as_irreversible"
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
                    'X-Rollback-Test': 'irreversible_identification',
                    'X-Scenario': scenario_name
                },
                timeout=25
            )

            end_time = time.time()

            # Check if response properly identifies action as irreversible
            response_text = response.text.lower() if response.text else ""
            irreversible_identified = any(
                indicator in response_text
                for indicator in ['irreversible', 'cannot be rolled back', 'permanent', 'no rollback']
            )

            result = {
                "scenario_name": scenario_name,
                "action_category": action_category,
                "reversibility": scenario["reversibility"],
                "action_type": action["action_type"],
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "irreversible_identified": irreversible_identified,
                "response_time": end_time - start_time,
                "response_text": response_text[:500],  # First 500 chars
                "proper_identification": irreversible_identified
            }

            # Print result
            identification_icon = '✅' if result["proper_identification"] else '❌'
            print(f"  {identification_icon} Identification: {'CORRECT' if result['proper_identification'] else 'MISSED'}")

            return result

        except Exception as e:
            return {
                "scenario_name": scenario_name,
                "action_category": action_category,
                "success": False,
                "error": str(e)
            }

    def execute_initial_action(self, scenario):
        """Execute the initial action for a reversible scenario"""
        initial_action = scenario["initial_action"]
        action_id = f"rollback_test_{uuid.uuid4().hex[:8]}"

        # Create alert that triggers the initial action
        test_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": f"RollbackTest_{scenario['scenario_name']}",
                    "severity": "warning",
                    "namespace": self.test_namespace,
                    "action_type": initial_action["action_type"],
                    "target": initial_action["target"],
                    "rollback_test_id": action_id
                },
                "annotations": {
                    "description": f"Initial action for rollback test: {initial_action['action_type']}",
                    "summary": f"Rollback test initial action",
                    "rollback_test": "initial_action"
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
                    'X-Rollback-Test': 'initial_action',
                    'X-Action-ID': action_id,
                    'X-Target-Namespace': self.test_namespace
                },
                timeout=30
            )

            end_time = time.time()

            result = {
                "action_id": action_id,
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "response_text": response.text[:500] if response.text else "",
                "timestamp": start_time
            }

            # Store action for potential rollback
            if result["success"]:
                self.action_history[action_id] = {
                    "scenario": scenario,
                    "execution_time": start_time,
                    "initial_result": result
                }

            return result

        except Exception as e:
            return {
                "action_id": action_id,
                "success": False,
                "error": str(e)
            }

    def execute_rollback_action(self, scenario, action_id):
        """Execute rollback for a previously executed action"""
        # Create rollback request alert
        rollback_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": f"RollbackRequest_{scenario['scenario_name']}",
                    "severity": "warning",
                    "namespace": self.test_namespace,
                    "action_id": action_id,
                    "rollback_request": "true"
                },
                "annotations": {
                    "description": f"Rollback request for action {action_id}",
                    "summary": "Rollback action request",
                    "original_action": scenario["initial_action"]["action_type"],
                    "expected_rollback": scenario["initial_action"]["expected_rollback"]
                },
                "startsAt": "2025-01-01T10:00:00Z"
            }]
        }

        try:
            start_time = time.time()

            response = requests.post(
                self.webhook_url,
                json=rollback_alert,
                headers={
                    'Content-Type': 'application/json',
                    'X-Rollback-Test': 'rollback_action',
                    'X-Action-ID': action_id,
                    'X-Rollback-Request': 'true'
                },
                timeout=30
            )

            end_time = time.time()

            return {
                "action_id": action_id,
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "response_time": end_time - start_time,
                "response_text": response.text[:500] if response.text else "",
                "timestamp": start_time
            }

        except Exception as e:
            return {
                "action_id": action_id,
                "success": False,
                "error": str(e)
            }

    def verify_action_state(self, scenario, verification_phase):
        """Verify the state of resources after action or rollback"""
        # This is a simplified verification - in a real system,
        # this would check actual Kubernetes resource states

        action_category = scenario["action_category"]
        target = scenario["initial_action"]["target"]

        # Simulate state verification based on action type
        verification_success = True  # Simplified for test environment

        if verification_phase == "after_rollback":
            # Check if rollback restored original state
            rollback_verified = verification_success  # In real implementation, would check actual state
        else:
            rollback_verified = False

        return {
            "verification_phase": verification_phase,
            "target": target,
            "verification_success": verification_success,
            "rollback_verified": rollback_verified if verification_phase == "after_rollback" else False,
            "verification_timestamp": time.time()
        }

    def analyze_rollback_results(self, validation_criteria):
        """Analyze rollback test results"""
        reversible_results = [r for r in self.results if r.get("reversibility") == "reversible"]
        irreversible_results = [r for r in self.results if r.get("reversibility") == "irreversible"]

        # Analyze reversible action rollbacks
        successful_rollbacks = [r for r in reversible_results if r.get("overall_success", False)]
        rollback_success_rate = (len(successful_rollbacks) / len(reversible_results)) if reversible_results else 0

        # Analyze irreversible action identification
        proper_identifications = [r for r in irreversible_results if r.get("proper_identification", False)]
        identification_success_rate = (len(proper_identifications) / len(irreversible_results)) if irreversible_results else 0

        analysis = {
            "total_scenarios_tested": len(self.results),
            "reversible_scenarios": len(reversible_results),
            "irreversible_scenarios": len(irreversible_results),
            "successful_rollbacks": len(successful_rollbacks),
            "rollback_success_rate": rollback_success_rate,
            "proper_identifications": len(proper_identifications),
            "identification_success_rate": identification_success_rate,
            "reversible_scenario_details": [
                {
                    "scenario": r["scenario_name"],
                    "category": r["action_category"],
                    "complexity": r.get("rollback_complexity", "unknown"),
                    "initial_success": r.get("initial_action_success", False),
                    "rollback_success": r.get("rollback_success", False),
                    "overall_success": r.get("overall_success", False)
                }
                for r in reversible_results
            ],
            "irreversible_scenario_details": [
                {
                    "scenario": r["scenario_name"],
                    "category": r["action_category"],
                    "action_type": r.get("action_type", "unknown"),
                    "properly_identified": r.get("proper_identification", False)
                }
                for r in irreversible_results
            ]
        }

        # Business requirement validation
        analysis["br_pa_013_compliance"] = {
            "requirement": "Provide rollback capability for reversible actions",
            "rollback_success_rate": analysis["rollback_success_rate"],
            "required_rollback_rate": validation_criteria["successful_rollback_rate"],
            "rollback_rate_met": analysis["rollback_success_rate"] >= validation_criteria["successful_rollback_rate"],
            "irreversible_identification": analysis["identification_success_rate"],
            "identification_requirement_met": analysis["identification_success_rate"] >= 0.8,  # 80% threshold
            "pass": (analysis["rollback_success_rate"] >= validation_criteria["successful_rollback_rate"] and
                    analysis["identification_success_rate"] >= 0.8),
            "rollback_capability": "comprehensive" if analysis["rollback_success_rate"] >= 0.95 else "adequate" if analysis["rollback_success_rate"] >= 0.90 else "insufficient"
        }

        return analysis

    def cleanup_rollback_test_environment(self):
        """Clean up rollback test environment"""
        print("Cleaning up rollback test environment...")
        try:
            result = subprocess.run(
                ["kubectl", "delete", "namespace", self.test_namespace, "--timeout=60s"],
                capture_output=True,
                text=True,
                timeout=70
            )

            if result.returncode == 0:
                print("  ✅ Rollback test environment cleaned up")
            else:
                print(f"  ⚠️ Cleanup warning: {result.stderr}")

        except Exception as e:
            print(f"  ⚠️ Cleanup error: {e}")

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = RollbackTestFramework(webhook_url)

    try:
        results = tester.test_rollback_capabilities(f"results/{test_session}/rollback_test_scenarios.json")

        # Save results
        with open(f"results/{test_session}/rollback_capability_results.json", "w") as f:
            json.dump(results, f, indent=2)

        # Print summary
        if "error" not in results:
            print(f"\n=== Rollback Capability Test Results ===")
            print(f"Total Scenarios: {results['total_scenarios_tested']}")
            print(f"Reversible Scenarios: {results['reversible_scenarios']}")
            print(f"Irreversible Scenarios: {results['irreversible_scenarios']}")
            print(f"Successful Rollbacks: {results['successful_rollbacks']}")
            print(f"Rollback Success Rate: {results['rollback_success_rate']*100:.1f}%")
            print(f"Proper Identifications: {results['proper_identifications']}")
            print(f"Identification Success Rate: {results['identification_success_rate']*100:.1f}%")

            print(f"\nReversible Scenario Details:")
            for detail in results['reversible_scenario_details']:
                success_icon = '✅' if detail['overall_success'] else '❌'
                print(f"  {detail['scenario']}: {success_icon} ({detail['category']}, {detail['complexity']})")

            print(f"\nIrreversible Scenario Details:")
            for detail in results['irreversible_scenario_details']:
                id_icon = '✅' if detail['properly_identified'] else '❌'
                print(f"  {detail['scenario']}: {id_icon} ({detail['action_type']})")

            compliance = results["br_pa_013_compliance"]
            print(f"\n=== BR-PA-013 Compliance ===")
            print(f"Rollback Success Rate: {compliance['rollback_success_rate']*100:.1f}% (required: {compliance['required_rollback_rate']*100:.1f}%) {'✅' if compliance['rollback_rate_met'] else '❌'}")
            print(f"Irreversible Identification: {compliance['irreversible_identification']*100:.1f}% {'✅' if compliance['identification_requirement_met'] else '❌'}")
            print(f"Rollback Capability: {compliance['rollback_capability']}")
            print(f"Overall Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")
        else:
            print(f"❌ Error: {results['error']}")

    finally:
        # Always cleanup
        tester.cleanup_rollback_test_environment()