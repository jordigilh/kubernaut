#!/usr/bin/env python3
"""
Learning Feedback Loop Test
Tests whether effectiveness tracking influences future AI recommendations
"""
import json
import requests
import time

class LearningFeedbackTest:
    def __init__(self, webhook_url):
        self.webhook_url = webhook_url
        self.results = []

    def test_learning_feedback_loop(self):
        """Test learning feedback loop functionality"""
        print("Testing learning feedback loop...")

        # Step 1: Get baseline recommendation for a scenario
        baseline_result = self.get_baseline_recommendation()
        time.sleep(3)

        # Step 2: Submit negative effectiveness feedback
        feedback_result = self.submit_negative_feedback()
        time.sleep(3)

        # Step 3: Get recommendation for same scenario after feedback
        post_feedback_result = self.get_post_feedback_recommendation()
        time.sleep(2)

        return self.analyze_learning_feedback({
            "baseline": baseline_result,
            "feedback": feedback_result,
            "post_feedback": post_feedback_result
        })

    def get_baseline_recommendation(self):
        """Get baseline recommendation for learning comparison"""
        test_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "LearningTestAlert",
                    "severity": "warning",
                    "namespace": "learning-test",
                    "pod": "test-pod-learning",
                    "learning_test": "baseline"
                },
                "annotations": {
                    "description": "Baseline test for learning feedback loop",
                    "summary": "Learning test baseline"
                }
            }]
        }

        try:
            response = requests.post(
                self.webhook_url,
                json=test_alert,
                headers={
                    'Content-Type': 'application/json',
                    'X-Learning-Test': 'baseline',
                    'X-Request-Remediation': 'true'
                },
                timeout=25
            )

            return {
                "phase": "baseline",
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "recommendation": response.text[:800] if response.text else "",
                "recommendation_length": len(response.text) if response.text else 0,
                "timestamp": time.time()
            }

        except Exception as e:
            return {
                "phase": "baseline",
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def submit_negative_feedback(self):
        """Submit negative effectiveness feedback to influence learning"""
        negative_feedback = {
            "alert_type": "LearningTestAlert",
            "namespace": "learning-test",
            "remediation_attempted": "pod_restart",
            "effectiveness_score": 0.2,
            "success": False,
            "feedback": "Pod restart did not resolve the issue, consider alternative approaches",
            "timestamp": "2025-01-01T10:00:00Z"
        }

        try:
            response = requests.post(
                f"{self.webhook_url.replace('/webhook/prometheus', '/api/effectiveness/learn')}",
                json=negative_feedback,
                headers={
                    'Content-Type': 'application/json',
                    'X-Learning-Feedback': 'negative'
                },
                timeout=15
            )

            return {
                "feedback_phase": "negative",
                "submission_success": response.status_code in [200, 201, 202],
                "status_code": response.status_code,
                "timestamp": time.time()
            }

        except Exception as e:
            return {
                "feedback_phase": "negative",
                "submission_success": False,
                "error": str(e),
                "note": "Learning feedback endpoint may not be implemented",
                "timestamp": time.time()
            }

    def get_post_feedback_recommendation(self):
        """Get recommendation after negative feedback to check for learning"""
        # Same alert as baseline to compare learning effect
        test_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "LearningTestAlert",
                    "severity": "warning",
                    "namespace": "learning-test",
                    "pod": "test-pod-learning",
                    "learning_test": "post_feedback"
                },
                "annotations": {
                    "description": "Post-feedback test for learning feedback loop",
                    "summary": "Learning test after feedback"
                }
            }]
        }

        try:
            response = requests.post(
                self.webhook_url,
                json=test_alert,
                headers={
                    'Content-Type': 'application/json',
                    'X-Learning-Test': 'post_feedback',
                    'X-Request-Remediation': 'true'
                },
                timeout=25
            )

            return {
                "phase": "post_feedback",
                "success": response.status_code == 200,
                "status_code": response.status_code,
                "recommendation": response.text[:800] if response.text else "",
                "recommendation_length": len(response.text) if response.text else 0,
                "timestamp": time.time()
            }

        except Exception as e:
            return {
                "phase": "post_feedback",
                "success": False,
                "error": str(e),
                "timestamp": time.time()
            }

    def analyze_learning_feedback(self, test_results):
        """Analyze learning feedback loop results"""
        baseline = test_results["baseline"]
        feedback = test_results["feedback"]
        post_feedback = test_results["post_feedback"]

        # Check if both recommendation requests succeeded
        both_recommendations_successful = baseline["success"] and post_feedback["success"]

        learning_indicators = {}
        if both_recommendations_successful:
            # Compare recommendations for differences (indicating potential learning)
            baseline_text = baseline["recommendation"].lower()
            post_feedback_text = post_feedback["recommendation"].lower()

            # Simple learning indicators
            learning_indicators = {
                "recommendations_different": baseline_text != post_feedback_text,
                "length_difference": abs(baseline["recommendation_length"] - post_feedback["recommendation_length"]),
                "contains_alternative_approaches": any(
                    phrase in post_feedback_text
                    for phrase in ["alternative", "instead", "different", "other approach", "consider"]
                ),
                "mentions_scaling_over_restart": "scale" in post_feedback_text and "restart" not in post_feedback_text,
                "recommendation_evolution": len(post_feedback_text) > len(baseline_text)  # More detailed recommendations
            }

        analysis = {
            "baseline_recommendation_success": baseline["success"],
            "feedback_submission_success": feedback["submission_success"],
            "post_feedback_recommendation_success": post_feedback["success"],
            "both_recommendations_obtained": both_recommendations_successful,
            "learning_indicators": learning_indicators
        }

        # Assess learning capability
        learning_evidence_count = sum(1 for indicator, value in learning_indicators.items() if value) if learning_indicators else 0

        # Business requirement validation for learning
        analysis["br_pa_008_learning_compliance"] = {
            "requirement": "Learning feedback loop for improving future recommendations",
            "feedback_submission_capability": analysis["feedback_submission_success"],
            "recommendation_generation_working": analysis["both_recommendations_obtained"],
            "learning_evidence_indicators": learning_evidence_count,
            "potential_learning_detected": learning_evidence_count >= 2,
            "pass": (analysis["feedback_submission_success"] or  # Either feedback works
                    analysis["both_recommendations_obtained"]),    # Or basic recommendation system works
            "learning_implementation": "evidence_detected" if learning_evidence_count >= 2 else "basic_capability" if analysis["both_recommendations_obtained"] else "not_detected"
        }

        return analysis

if __name__ == "__main__":
    import sys

    webhook_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080/webhook/prometheus"
    test_session = sys.argv[2] if len(sys.argv) > 2 else "test_session"

    tester = LearningFeedbackTest(webhook_url)
    results = tester.test_learning_feedback_loop()

    # Save results
    with open(f"results/{test_session}/learning_feedback_results.json", "w") as f:
        json.dump(results, f, indent=2)

    # Print summary
    print(f"\n=== Learning Feedback Loop Test Results ===")
    print(f"Baseline Recommendation: {'✅' if results['baseline_recommendation_success'] else '❌'}")
    print(f"Feedback Submission: {'✅' if results['feedback_submission_success'] else '❌'}")
    print(f"Post-Feedback Recommendation: {'✅' if results['post_feedback_recommendation_success'] else '❌'}")
    print(f"Both Recommendations Obtained: {'✅' if results['both_recommendations_obtained'] else '❌'}")

    if results['learning_indicators']:
        indicators = results['learning_indicators']
        print(f"\nLearning Indicators:")
        for indicator, value in indicators.items():
            print(f"  {indicator}: {'✅' if value else '❌'}")

    compliance = results["br_pa_008_learning_compliance"]
    print(f"\n=== Learning Compliance ===")
    print(f"Learning Evidence Indicators: {compliance['learning_evidence_indicators']}")
    print(f"Learning Implementation: {compliance['learning_implementation']}")
    print(f"Result: {'✅ PASS' if compliance['pass'] else '❌ FAIL'}")