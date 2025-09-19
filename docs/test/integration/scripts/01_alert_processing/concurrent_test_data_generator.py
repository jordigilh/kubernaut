#!/usr/bin/env python3
"""
Concurrent Test Data Generator
Creates diverse alert scenarios for concurrent testing
"""
import json
import random
from datetime import datetime, timedelta

class ConcurrentTestDataGenerator:
    def __init__(self):
        self.alert_types = [
            "HighMemoryUsage", "HighCPUUsage", "PodCrashLooping",
            "StorageNearlyFull", "NetworkLatencyHigh", "ServiceDown",
            "NodeNotReady", "ReplicaSetIncomplete", "ConfigMapChanged"
        ]
        self.severities = ["critical", "warning", "info"]
        self.namespaces = ["test-workloads", "kube-system", "monitoring", "default"]

    def generate_concurrent_alerts(self, count=100):
        """Generate a set of diverse alerts for concurrent testing"""
        alerts = []

        for i in range(count):
            alert = {
                "request_id": i,
                "payload": {
                    "alerts": [{
                        "status": "firing",
                        "labels": {
                            "alertname": random.choice(self.alert_types),
                            "severity": random.choice(self.severities),
                            "namespace": random.choice(self.namespaces),
                            "instance": f"instance-{i:03d}",
                            "job": f"job-{random.randint(1, 10)}",
                            "concurrent_test_id": str(i)
                        },
                        "annotations": {
                            "description": f"Concurrent test alert {i}",
                            "summary": f"Testing concurrent processing for request {i}",
                            "runbook_url": f"https://runbooks.example.com/alert-{i}"
                        },
                        "startsAt": (datetime.now() + timedelta(seconds=random.randint(0, 60))).isoformat(),
                        "generatorURL": "http://prometheus:9090"
                    }]
                }
            }
            alerts.append(alert)

        return alerts

    def generate_duplicate_detection_alerts(self, count=50):
        """Generate alerts to test duplicate handling during concurrent processing"""
        base_alert = {
            "status": "firing",
            "labels": {
                "alertname": "DuplicateTestAlert",
                "severity": "warning",
                "namespace": "test-workloads",
                "instance": "duplicate-test-instance"
            },
            "annotations": {
                "description": "Duplicate detection test alert",
                "summary": "Testing duplicate handling under concurrency"
            },
            "startsAt": datetime.now().isoformat(),
            "generatorURL": "http://prometheus:9090"
        }

        alerts = []
        for i in range(count):
            alert = {
                "request_id": f"dup_{i}",
                "payload": {
                    "alerts": [base_alert.copy()]
                }
            }
            alerts.append(alert)

        return alerts

if __name__ == "__main__":
    generator = ConcurrentTestDataGenerator()

    # Generate diverse concurrent test alerts
    concurrent_alerts = generator.generate_concurrent_alerts(100)
    with open("concurrent_alerts.json", "w") as f:
        json.dump(concurrent_alerts, f, indent=2)

    # Generate duplicate detection test alerts
    duplicate_alerts = generator.generate_duplicate_detection_alerts(50)
    with open("duplicate_alerts.json", "w") as f:
        json.dump(duplicate_alerts, f, indent=2)

    print("Generated test data:")
    print(f"- {len(concurrent_alerts)} diverse concurrent alerts")
    print(f"- {len(duplicate_alerts)} duplicate detection alerts")