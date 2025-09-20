#!/usr/bin/env python3
"""
Confidence Score Analyzer
Analyzes and validates confidence scores provided by the AI system
"""
import json
import re
import statistics

class ConfidenceAnalyzer:
    def __init__(self):
        self.score_patterns = [
            r'confidence[:\s]+([0-9]*\.?[0-9]+)',
            r'confidence score[:\s]+([0-9]*\.?[0-9]+)',
            r'score[:\s]+([0-9]*\.?[0-9]+)',
            r'certainty[:\s]+([0-9]*\.?[0-9]+)',
            r'reliability[:\s]+([0-9]*\.?[0-9]+)'
        ]

    def extract_confidence_score(self, response_text):
        """Extract confidence score from response text"""
        if not response_text:
            return None

        response_lower = response_text.lower()

        # Try different patterns to find confidence score
        for pattern in self.score_patterns:
            matches = re.findall(pattern, response_lower)
            if matches:
                try:
                    score = float(matches[0])
                    # Convert percentages to 0-1 scale if needed
                    if score > 1.0 and score <= 100.0:
                        score = score / 100.0
                    return score
                except ValueError:
                    continue

        # Look for qualitative confidence indicators
        qualitative_indicators = {
            'very confident': 0.9,
            'high confidence': 0.85,
            'confident': 0.8,
            'fairly confident': 0.7,
            'moderately confident': 0.6,
            'somewhat confident': 0.5,
            'low confidence': 0.3,
            'uncertain': 0.2,
            'very uncertain': 0.1
        }

        for indicator, score in qualitative_indicators.items():
            if indicator in response_lower:
                return score

        return None

    def validate_confidence_score(self, score, scenario_data):
        """Validate confidence score against scenario expectations"""
        if score is None:
            return {
                "score_provided": False,
                "validation_error": "No confidence score found in response"
            }

        validation = {
            "score_provided": True,
            "extracted_score": score,
            "scale_compliant": 0.0 <= score <= 1.0,
            "precision": len(str(score).split('.')[-1]) if '.' in str(score) else 0
        }

        # Check against expected range
        expected_range = scenario_data.get("expected_confidence_range", [0.0, 1.0])
        validation["within_expected_range"] = expected_range[0] <= score <= expected_range[1]
        validation["expected_range"] = expected_range

        # Assess appropriateness based on scenario characteristics
        alert_complexity = scenario_data.get("alert_complexity", "medium")
        context_clarity = scenario_data.get("context_clarity", "medium")

        validation["appropriateness_assessment"] = self._assess_score_appropriateness(
            score, alert_complexity, context_clarity
        )

        return validation

    def _assess_score_appropriateness(self, score, complexity, clarity):
        """Assess whether confidence score is appropriate for scenario characteristics"""
        # Expected score ranges based on complexity and clarity
        appropriateness_matrix = {
            ("low", "high"): (0.7, 1.0),      # Simple alert, clear context -> high confidence
            ("low", "medium"): (0.6, 0.9),    # Simple alert, partial context -> medium-high confidence
            ("low", "low"): (0.4, 0.7),      # Simple alert, unclear context -> medium confidence
            ("medium", "high"): (0.6, 0.8),   # Medium complexity, clear context -> medium-high confidence
            ("medium", "medium"): (0.4, 0.7), # Medium complexity, partial context -> medium confidence
            ("medium", "low"): (0.2, 0.5),   # Medium complexity, unclear context -> low-medium confidence
            ("high", "high"): (0.4, 0.7),     # Complex alert, clear context -> medium confidence
            ("high", "medium"): (0.3, 0.6),   # Complex alert, partial context -> low-medium confidence
            ("high", "low"): (0.1, 0.4),     # Complex alert, unclear context -> low confidence
            ("very_high", "high"): (0.3, 0.6), # Very complex, clear context -> low-medium confidence
            ("very_high", "medium"): (0.2, 0.5), # Very complex, partial context -> low confidence
            ("very_high", "low"): (0.0, 0.3)    # Very complex, unclear context -> very low confidence
        }

        expected_range = appropriateness_matrix.get((complexity, clarity), (0.0, 1.0))
        is_appropriate = expected_range[0] <= score <= expected_range[1]

        return {
            "expected_range_for_scenario": expected_range,
            "is_appropriate": is_appropriate,
            "appropriateness_score": 1.0 if is_appropriate else max(0.0, 1.0 - abs(score - sum(expected_range)/2) / 0.5)
        }

    def analyze_confidence_consistency(self, results):
        """Analyze consistency of confidence scoring across similar scenarios"""
        if len(results) < 2:
            return {"insufficient_data": True}

        # Group results by expected confidence level
        confidence_groups = {}
        for result in results:
            expected_level = result.get("expected_confidence", "unknown")
            if expected_level not in confidence_groups:
                confidence_groups[expected_level] = []

            score = result.get("confidence_analysis", {}).get("extracted_score")
            if score is not None:
                confidence_groups[expected_level].append(score)

        consistency_analysis = {}
        for level, scores in confidence_groups.items():
            if len(scores) > 1:
                consistency_analysis[level] = {
                    "score_count": len(scores),
                    "mean_score": statistics.mean(scores),
                    "std_deviation": statistics.stdev(scores) if len(scores) > 1 else 0,
                    "min_score": min(scores),
                    "max_score": max(scores),
                    "score_range": max(scores) - min(scores),
                    "consistent": statistics.stdev(scores) < 0.2 if len(scores) > 1 else True
                }

        return {
            "confidence_groups": confidence_groups,
            "consistency_by_level": consistency_analysis,
            "overall_consistency": all(
                level_data["consistent"]
                for level_data in consistency_analysis.values()
            )
        }

if __name__ == "__main__":
    # Test the analyzer
    analyzer = ConfidenceAnalyzer()

    sample_texts = [
        "Based on this alert, I have high confidence (0.85) that restarting the pod will resolve the issue.",
        "Confidence score: 0.45 - The alert lacks specific context making remediation uncertain.",
        "I'm very confident this approach will work.",
        "With low confidence, I suggest checking the logs first."
    ]

    for i, text in enumerate(sample_texts):
        score = analyzer.extract_confidence_score(text)
        print(f"Sample {i+1}: Extracted score = {score}")