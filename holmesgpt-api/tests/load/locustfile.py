"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Load Testing for HolmesGPT API

Uses Locust for load testing with mock LLM responses to avoid inference costs.

Usage:
    # Local development (mock LLM)
    locust -f locustfile.py --host=http://localhost:8080

    # Against OCP cluster (port-forward first)
    oc port-forward -n kubernaut-system svc/holmesgpt-api 8080:80
    locust -f locustfile.py --host=http://localhost:8080

    # Headless mode for CI/CD
    locust -f locustfile.py --host=http://localhost:8080 \
           --users 50 --spawn-rate 5 --run-time 5m --headless

Business Requirements: BR-HAPI-104 (Performance validation)
"""

import json
import random
from locust import HttpUser, task, between


class HolmesGPTAPIUser(HttpUser):
    """
    Simulates a user making investigation requests to HolmesGPT API
    """

    # Wait 1-3 seconds between tasks
    wait_time = between(1, 3)

    # Sample alert data for testing
    SAMPLE_ALERTS = [
        {
            "incident_id": "test-incident-001",
            "namespace": "production",
            "alert_name": "HighCPUUsage",
            "resource_type": "deployment",
            "resource_name": "api-server",
            "severity": "warning",
            "labels": {
                "app": "api-server",
                "environment": "production"
            },
            "investigation_results": {
                "cpu_usage": 85.5,
                "memory_usage": 60.2,
                "replica_count": 3
            }
        },
        {
            "incident_id": "test-incident-002",
            "namespace": "staging",
            "alert_name": "PodCrashLooping",
            "resource_type": "pod",
            "resource_name": "worker-pod-abc123",
            "severity": "critical",
            "labels": {
                "app": "worker",
                "environment": "staging"
            },
            "investigation_results": {
                "restart_count": 15,
                "exit_code": 137,
                "oom_killed": True
            }
        },
        {
            "incident_id": "test-incident-003",
            "namespace": "production",
            "alert_name": "HighMemoryUsage",
            "resource_type": "statefulset",
            "resource_name": "database",
            "severity": "warning",
            "labels": {
                "app": "postgresql",
                "environment": "production"
            },
            "investigation_results": {
                "memory_usage": 92.3,
                "cache_hit_ratio": 0.65,
                "active_connections": 450
            }
        },
        {
            "incident_id": "test-incident-004",
            "namespace": "production",
            "alert_name": "DiskSpaceWarning",
            "resource_type": "persistentvolumeclaim",
            "resource_name": "data-pvc",
            "severity": "warning",
            "labels": {
                "app": "storage",
                "environment": "production"
            },
            "investigation_results": {
                "disk_usage_percent": 88.5,
                "available_space_gb": 12.3,
                "total_space_gb": 100
            }
        }
    ]

    SAMPLE_POSTEXEC = {
        "execution_id": "exec-test-001",
        "incident_id": "test-incident-001",
        "strategy_id": "scale-replicas",
        "executed_actions": [
            {
                "action": "scale_deployment",
                "resource": "api-server",
                "from_replicas": 3,
                "to_replicas": 5
            }
        ],
        "pre_execution_state": {
            "cpu_usage": 85.5,
            "replica_count": 3
        },
        "post_execution_state": {
            "cpu_usage": 45.2,
            "replica_count": 5
        }
    }

    def on_start(self):
        """Called when a user starts"""
        pass

    @task(5)
    def health_check(self):
        """
        Health check endpoint (weight: 5)

        Simulates monitoring probes hitting health endpoints
        """
        self.client.get("/health", name="/health")

    @task(2)
    def ready_check(self):
        """
        Readiness check endpoint (weight: 2)

        Simulates k8s readiness probes
        """
        self.client.get("/ready", name="/ready")

    @task(10)
    def incident_analysis(self):
        """
        Incident analysis request (weight: 10)

        Primary investigation endpoint - highest weight
        """
        alert = random.choice(self.SAMPLE_ALERTS)
        # Convert to incident request format
        incident_request = {
            "incident_id": alert.get("incident_id", "test-incident-001"),
            "remediation_id": f"rem-{alert.get('incident_id', 'test')}",
            "signal_name": alert.get("alert_name", "HighCPUUsage"),
            "severity": alert.get("severity", "warning"),
            "signal_source": "prometheus",
            "resource_namespace": alert.get("namespace", "production"),
            "resource_kind": "Pod",
            "resource_name": alert.get("resource_name", "api-server"),
            "error_message": str(alert.get("investigation_results", {})),
            "environment": "production",
            "priority": "P2",
            "risk_tolerance": "medium",
            "business_category": "standard",
            "cluster_name": "test-cluster",
        }

        with self.client.post(
            "/api/v1/incident/analyze",
            json=incident_request,
            catch_response=True,
            name="/api/v1/incident/analyze"
        ) as response:
            if response.status_code == 202:
                data = response.json()
                if "session_id" in data:
                    response.success()
                else:
                    response.failure("Missing session_id in 202 response")
            elif response.status_code == 200:
                data = response.json()
                if "incident_id" in data and "analysis" in data:
                    response.success()
                else:
                    response.failure("Invalid response structure")
            elif response.status_code == 500:
                # Expected if LLM is mocked
                response.success()
            else:
                response.failure(f"Unexpected status code: {response.status_code}")

    @task(3)
    def postexec_analysis(self):
        """
        Post-execution analysis request (weight: 3)

        Secondary endpoint for analyzing execution results
        """
        postexec_data = self.SAMPLE_POSTEXEC.copy()
        postexec_data["execution_id"] = f"exec-test-{random.randint(1000, 9999)}"

        with self.client.post(
            "/api/v1/postexec/analyze",
            json=postexec_data,
            catch_response=True,
            name="/api/v1/postexec/analyze"
        ) as response:
            if response.status_code == 200:
                data = response.json()

                # Validate response structure
                if "execution_id" in data and "effectiveness_assessment" in data:
                    response.success()
                else:
                    response.failure("Invalid response structure")
            elif response.status_code == 500:
                # Expected if LLM is mocked
                response.success()
            else:
                response.failure(f"Unexpected status code: {response.status_code}")

    @task(1)
    def metrics_endpoint(self):
        """
        Metrics endpoint (weight: 1)

        Simulates Prometheus scraping metrics
        """
        with self.client.get("/metrics", name="/metrics", catch_response=True) as response:
            if response.status_code == 200:
                # Check if response contains Prometheus metrics
                if "holmesgpt_" in response.text:
                    response.success()
                else:
                    response.failure("No HolmesGPT metrics found")
            else:
                response.failure(f"Unexpected status code: {response.status_code}")


class AdminUser(HttpUser):
    """
    Simulates an admin user checking configuration

    Lower frequency, mainly checks config endpoint
    """

    wait_time = between(10, 20)

    @task
    def check_config(self):
        """Check configuration endpoint"""
        self.client.get("/config", name="/config")


# Load test scenarios
class LightLoad(HttpUser):
    """
    Light load scenario: 10 users, gradual ramp-up

    Usage:
        locust -f locustfile.py --host=http://localhost:8080 \
               --users 10 --spawn-rate 2 --run-time 5m
    """
    wait_time = between(2, 5)
    tasks = [HolmesGPTAPIUser]


class MediumLoad(HttpUser):
    """
    Medium load scenario: 50 users, moderate ramp-up

    Usage:
        locust -f locustfile.py --host=http://localhost:8080 \
               --users 50 --spawn-rate 5 --run-time 10m
    """
    wait_time = between(1, 3)
    tasks = [HolmesGPTAPIUser]


class HeavyLoad(HttpUser):
    """
    Heavy load scenario: 200 users, aggressive ramp-up

    Usage:
        locust -f locustfile.py --host=http://localhost:8080 \
               --users 200 --spawn-rate 10 --run-time 15m

    WARNING: Use mock LLM to avoid high inference costs
    """
    wait_time = between(0.5, 2)
    tasks = [HolmesGPTAPIUser]

