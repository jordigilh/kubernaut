#!/usr/bin/env python3
"""
Maximum Load Availability Test
Progressively increases load to find maximum sustainable rate with 99.9% availability
"""
import json
import time
import threading
from availability_monitor import AvailabilityMonitor
from detailed_response_time_test import DetailedResponseTimeTest

class MaxLoadAvailabilityTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.test_results = []

    def test_load_level(self, alerts_per_minute, duration_minutes=15):
        """Test a specific load level for availability"""
        print(f"Testing {alerts_per_minute} alerts/min for {duration_minutes} minutes...")

        availability_monitor = AvailabilityMonitor(self.webhook_url, check_interval=5)
        load_generator = DetailedResponseTimeTest(self.webhook_url, alerts_per_minute)

        # Start monitoring
        availability_thread = threading.Thread(
            target=availability_monitor.run_availability_monitoring,
            args=(duration_minutes,),
            daemon=True
        )

        load_thread = threading.Thread(
            target=load_generator.run_test,
            daemon=True
        )

        availability_thread.start()
        time.sleep(2)
        load_thread.start()

        availability_thread.join()
        load_thread.join()

        availability_results = availability_monitor.calculate_availability_statistics()
        load_results = load_generator.analyze_results()

        test_point = {
            "load_alerts_per_minute": alerts_per_minute,
            "duration_minutes": duration_minutes,
            "availability_percentage": availability_results["availability_percentage"],
            "availability_compliant": availability_results["br_pa_001_compliance"]["pass"],
            "load_success_rate": load_results["success_rate"],
            "response_time_95th": load_results["percentile_95"],
            "downtime_seconds": availability_results["downtime_seconds"]
        }

        self.test_results.append(test_point)
        return test_point

    def run_max_load_exploration(self):
        """Run progressive load testing to find maximum sustainable load"""
        # Progressive load levels
        load_levels = [100, 200, 400, 800, 1600, 3200]

        print("=== Maximum Load Availability Exploration ===")

        for load in load_levels:
            try:
                result = self.test_load_level(load)

                print(f"Load {load:>4}/min: " +
                      f"Avail {result['availability_percentage']:.3f}%, " +
                      f"Success {result['load_success_rate']:.1f}%, " +
                      f"{'COMPLIANT' if result['availability_compliant'] else 'NON-COMPLIANT'}")

                # Stop if availability drops below 99.9%
                if not result['availability_compliant']:
                    print(f"Availability threshold breached at {load} alerts/min")
                    break

                # Stop if load success rate drops below 50%
                if result['load_success_rate'] < 50:
                    print(f"Load handling failure at {load} alerts/min")
                    break

                # Recovery period between tests
                print("Recovery period...")
                time.sleep(60)

            except Exception as e:
                print(f"Error testing load {load}: {e}")
                break

        return self.analyze_max_load_results()

    def analyze_max_load_results(self):
        """Analyze maximum load test results"""
        if not self.test_results:
            return {"error": "No test results available"}

        # Find maximum compliant load
        compliant_results = [r for r in self.test_results if r["availability_compliant"]]
        max_compliant_load = max([r["load_alerts_per_minute"] for r in compliant_results]) if compliant_results else 0

        # Find degradation point
        degradation_point = None
        for result in self.test_results:
            if not result["availability_compliant"]:
                degradation_point = result["load_alerts_per_minute"]
                break

        analysis = {
            "test_points": self.test_results,
            "max_compliant_load": max_compliant_load,
            "degradation_point": degradation_point,
            "capacity_assessment": self._assess_capacity(max_compliant_load),
            "business_impact": self._assess_business_impact(max_compliant_load)
        }

        return analysis

    def _assess_capacity(self, max_load):
        """Assess capacity relative to business requirements"""
        business_requirement = 1000  # 1000 alerts/min from docs

        if max_load >= business_requirement * 3:
            return "EXCELLENT - Significant capacity headroom"
        elif max_load >= business_requirement * 2:
            return "GOOD - Adequate capacity headroom"
        elif max_load >= business_requirement:
            return "ADEQUATE - Meets business requirements"
        else:
            return "INSUFFICIENT - Does not meet business requirements"

    def _assess_business_impact(self, max_load):
        """Assess business impact of capacity findings"""
        business_requirement = 1000
        headroom = max_load - business_requirement

        return {
            "business_requirement_alerts_per_minute": business_requirement,
            "measured_capacity_alerts_per_minute": max_load,
            "capacity_headroom_alerts_per_minute": headroom,
            "headroom_percentage": (headroom / business_requirement * 100) if business_requirement > 0 else 0,
            "pilot_deployment_risk": "LOW" if headroom > 500 else "MEDIUM" if headroom > 0 else "HIGH"
        }

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = MaxLoadAvailabilityTest(webhook_url)
    results = tester.run_max_load_exploration()

    # Save results
    with open(f"results/{test_session}/max_load_availability_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== Maximum Load Availability Results ===")
    print(f"Maximum Compliant Load: {results['max_compliant_load']} alerts/min")
    print(f"Degradation Point: {results['degradation_point']} alerts/min")
    print(f"Capacity Assessment: {results['capacity_assessment']}")

    impact = results["business_impact"]
    print(f"\nBusiness Impact Assessment:")
    print(f"Business Requirement: {impact['business_requirement_alerts_per_minute']} alerts/min")
    print(f"Measured Capacity: {impact['measured_capacity_alerts_per_minute']} alerts/min")
    print(f"Capacity Headroom: {impact['capacity_headroom_alerts_per_minute']} alerts/min ({impact['headroom_percentage']:.1f}%)")
    print(f"Pilot Deployment Risk: {impact['pilot_deployment_risk']}")