#!/usr/bin/env python3
"""
Response Time Curve Analysis
Maps response time characteristics across different load levels
"""
import json
import time
import statistics
from detailed_response_time_test import DetailedResponseTimeTest

class ResponseTimeCurveAnalysis:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.curve_points = []

    def test_load_point(self, alerts_per_minute, duration_minutes=5):
        """Test a specific load point"""
        print(f"Testing {alerts_per_minute} alerts/minute for {duration_minutes} minutes...")

        tester = DetailedResponseTimeTest(
            self.webhook_url,
            alerts_per_minute,
            duration_minutes
        )

        results = tester.run_test()

        curve_point = {
            "load_alerts_per_minute": alerts_per_minute,
            "duration_minutes": duration_minutes,
            "success_rate": results.get("success_rate", 0),
            "mean_response_time": results.get("mean_response_time", 0),
            "percentile_95": results.get("percentile_95", 0),
            "percentile_99": results.get("percentile_99", 0),
            "max_response_time": results.get("max_response_time", 0),
            "br_compliance": results.get("br_pa_003_compliance", {}).get("pass", False)
        }

        self.curve_points.append(curve_point)
        return curve_point

    def run_curve_analysis(self):
        """Run complete curve analysis"""
        # Progressive load points
        load_points = [60, 120, 240, 480, 960, 1920, 3840]

        print("=== Response Time Curve Analysis ===")
        print(f"Testing {len(load_points)} load points")

        for load in load_points:
            try:
                point = self.test_load_point(load)

                print(f"Load {load:>4}/min: " +
                      f"{point['percentile_95']:.3f}s (95th), " +
                      f"{point['success_rate']:.1f}% success, " +
                      f"{'COMPLIANT' if point['br_compliance'] else 'NON-COMPLIANT'}")

                # Stop if success rate drops below 50% (system breaking down)
                if point['success_rate'] < 50:
                    print(f"Breaking point reached at {load} alerts/min")
                    break

                # Brief recovery period
                if load < max(load_points):
                    print("Recovery period...")
                    time.sleep(30)

            except Exception as e:
                print(f"Error testing load {load}: {e}")
                break

        return self.analyze_curve()

    def analyze_curve(self):
        """Analyze the response time curve"""
        if not self.curve_points:
            return {"error": "No curve points collected"}

        # Find performance characteristics
        compliant_points = [p for p in self.curve_points if p["br_compliance"]]
        max_compliant_load = max([p["load_alerts_per_minute"] for p in compliant_points]) if compliant_points else 0

        # Find degradation point (first point where 95th percentile > 5s)
        degradation_point = None
        for point in self.curve_points:
            if point["percentile_95"] > 5.0:
                degradation_point = point["load_alerts_per_minute"]
                break

        # Calculate capacity headroom
        business_requirement_load = 1000  # 1000 alerts/min from BR requirement
        headroom_multiplier = max_compliant_load / business_requirement_load if business_requirement_load > 0 else 0

        analysis = {
            "curve_points": self.curve_points,
            "max_compliant_load": max_compliant_load,
            "degradation_point": degradation_point,
            "business_requirement_load": business_requirement_load,
            "headroom_multiplier": headroom_multiplier,
            "capacity_assessment": self._assess_capacity(headroom_multiplier),
            "performance_characteristics": self._analyze_performance_curve()
        }

        return analysis

    def _assess_capacity(self, headroom_multiplier):
        """Assess capacity based on headroom"""
        if headroom_multiplier >= 3.0:
            return "EXCELLENT - System has significant capacity headroom"
        elif headroom_multiplier >= 2.0:
            return "GOOD - System has adequate capacity headroom"
        elif headroom_multiplier >= 1.5:
            return "ADEQUATE - System meets requirements with some headroom"
        elif headroom_multiplier >= 1.0:
            return "MINIMAL - System barely meets requirements"
        else:
            return "INSUFFICIENT - System does not meet business requirements"

    def _analyze_performance_curve(self):
        """Analyze performance curve characteristics"""
        if len(self.curve_points) < 3:
            return "Insufficient data for curve analysis"

        loads = [p["load_alerts_per_minute"] for p in self.curve_points]
        response_times = [p["percentile_95"] for p in self.curve_points]

        # Simple analysis of curve shape
        initial_response = response_times[0]
        final_response = response_times[-1]

        if final_response / initial_response < 2:
            return "LINEAR - Response time increases linearly with load"
        elif final_response / initial_response < 5:
            return "MODERATE_EXPONENTIAL - Response time increases moderately with load"
        else:
            return "STEEP_EXPONENTIAL - Response time increases rapidly with load"

if __name__ == "__main__":
    import sys
    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    analyzer = ResponseTimeCurveAnalysis(webhook_url)
    results = analyzer.run_curve_analysis()

    # Save results
    with open(f"results/{test_session}/response_time_curve_analysis.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== Response Time Curve Analysis Results ===")
    print(f"Maximum Compliant Load: {results['max_compliant_load']} alerts/min")
    print(f"Degradation Point: {results['degradation_point']} alerts/min")
    print(f"Business Requirement: {results['business_requirement_load']} alerts/min")
    print(f"Headroom Multiplier: {results['headroom_multiplier']:.1f}x")
    print(f"Capacity Assessment: {results['capacity_assessment']}")
    print(f"Performance Characteristics: {results['performance_characteristics']}")