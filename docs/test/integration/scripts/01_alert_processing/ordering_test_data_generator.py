#!/usr/bin/env python3
"""
Alert Ordering Test Data Generator
Creates sequential alert scenarios to test ordering preservation
"""
import json
from datetime import datetime, timedelta

class OrderingTestDataGenerator:
    def __init__(self):
        self.base_time = datetime.now()

    def generate_alert_sequence(self, source_id, count=5):
        """Generate a sequence of alerts from the same source"""
        alerts = []

        for i in range(count):
            timestamp = self.base_time + timedelta(seconds=i * 10)  # 10 seconds apart

            alert = {
                "sequence_number": i,
                "source_id": source_id,
                "expected_order": i,
                "payload": {
                    "alerts": [{
                        "status": "firing" if i < count - 1 else "resolved",  # Last alert resolves
                        "labels": {
                            "alertname": f"SequentialAlert_{source_id}",
                            "severity": "warning",
                            "instance": f"source-{source_id}",
                            "sequence": str(i),
                            "source": source_id
                        },
                        "annotations": {
                            "description": f"Sequential alert {i} from source {source_id}",
                            "summary": f"Testing order preservation for source {source_id}",
                            "sequence_id": f"{source_id}_{i}"
                        },
                        "startsAt": timestamp.isoformat(),
                        "generatorURL": f"http://source-{source_id}:9090"
                    }]
                }
            }
            alerts.append(alert)

        return alerts

    def generate_interleaved_sequences(self, sources=3, alerts_per_source=5):
        """Generate interleaved alert sequences from multiple sources"""
        all_sequences = []

        for source_id in range(sources):
            sequence = self.generate_alert_sequence(f"source_{source_id}", alerts_per_source)
            all_sequences.extend(sequence)

        # Sort by timestamp to create natural interleaving
        all_sequences.sort(key=lambda x: x["payload"]["alerts"][0]["startsAt"])

        return all_sequences

    def generate_rapid_sequence(self, source_id, count=10):
        """Generate rapid-fire sequence to test ordering under fast delivery"""
        alerts = []

        for i in range(count):
            # Very short intervals (1 second apart)
            timestamp = self.base_time + timedelta(seconds=i)

            alert = {
                "sequence_number": i,
                "source_id": source_id,
                "expected_order": i,
                "rapid_sequence": True,
                "payload": {
                    "alerts": [{
                        "status": "firing",
                        "labels": {
                            "alertname": f"RapidAlert_{source_id}",
                            "severity": "critical" if i % 3 == 0 else "warning",
                            "instance": f"rapid-source-{source_id}",
                            "sequence": str(i),
                            "source": source_id,
                            "rapid_test": "true"
                        },
                        "annotations": {
                            "description": f"Rapid sequence alert {i} from source {source_id}",
                            "summary": f"Testing rapid-fire ordering for source {source_id}",
                            "sequence_id": f"rapid_{source_id}_{i}"
                        },
                        "startsAt": timestamp.isoformat(),
                        "generatorURL": f"http://rapid-source-{source_id}:9090"
                    }]
                }
            }
            alerts.append(alert)

        return alerts

    def generate_lifecycle_sequence(self, source_id):
        """Generate complete alert lifecycle sequence"""
        lifecycle_stages = [
            ("firing", "critical", "Alert triggered"),
            ("firing", "critical", "Alert severity escalated"),
            ("firing", "warning", "Alert severity reduced"),
            ("resolved", "info", "Alert resolved")
        ]

        alerts = []

        for i, (status, severity, description) in enumerate(lifecycle_stages):
            timestamp = self.base_time + timedelta(minutes=i * 5)  # 5 minutes apart

            alert = {
                "sequence_number": i,
                "source_id": source_id,
                "expected_order": i,
                "lifecycle_stage": status,
                "payload": {
                    "alerts": [{
                        "status": status,
                        "labels": {
                            "alertname": f"LifecycleAlert_{source_id}",
                            "severity": severity,
                            "instance": f"lifecycle-source-{source_id}",
                            "sequence": str(i),
                            "source": source_id,
                            "lifecycle_stage": status
                        },
                        "annotations": {
                            "description": description,
                            "summary": f"Lifecycle stage {i} for source {source_id}",
                            "sequence_id": f"lifecycle_{source_id}_{i}"
                        },
                        "startsAt": timestamp.isoformat(),
                        "generatorURL": f"http://lifecycle-source-{source_id}:9090"
                    }]
                }
            }
            alerts.append(alert)

        return alerts

if __name__ == "__main__":
    generator = OrderingTestDataGenerator()

    # Generate different test scenarios
    test_scenarios = {
        "interleaved_sequences": generator.generate_interleaved_sequences(3, 5),
        "rapid_sequence": generator.generate_rapid_sequence("rapid_source", 10),
        "lifecycle_sequence": generator.generate_lifecycle_sequence("lifecycle_source")
    }

    # Save test data
    for scenario_name, alerts in test_scenarios.items():
        with open(f"{scenario_name}.json", "w") as f:
            json.dump(alerts, f, indent=2)

    print("Generated test scenarios:")
    for name, alerts in test_scenarios.items():
        print(f"- {name}: {len(alerts)} alerts")