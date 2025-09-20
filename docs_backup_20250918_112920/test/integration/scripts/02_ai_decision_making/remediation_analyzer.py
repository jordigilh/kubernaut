#!/usr/bin/env python3
"""
Remediation Recommendation Analyzer
Analyzes quality and relevance of AI-generated remediation recommendations
"""
import re
import json
from collections import Counter

class RemediationAnalyzer:
    def __init__(self):
        self.kubernetes_keywords = [
            'kubectl', 'pod', 'deployment', 'service', 'namespace', 'container',
            'node', 'cluster', 'logs', 'describe', 'get', 'scale', 'restart',
            'apply', 'delete', 'create', 'patch', 'exec', 'port-forward'
        ]

        self.action_verbs = [
            'restart', 'scale', 'delete', 'create', 'update', 'check', 'verify',
            'monitor', 'investigate', 'troubleshoot', 'clean', 'optimize', 'fix'
        ]

    def analyze_recommendation_quality(self, recommendation_text, scenario_data):
        """Analyze the quality of a remediation recommendation"""
        if not recommendation_text:
            return {"error": "Empty recommendation text"}

        analysis = {
            "text_length": len(recommendation_text),
            "word_count": len(recommendation_text.split()),
            "kubernetes_relevance": self._assess_kubernetes_relevance(recommendation_text),
            "context_usage": self._assess_context_usage(recommendation_text, scenario_data),
            "actionability": self._assess_actionability(recommendation_text),
            "specificity": self._assess_specificity(recommendation_text, scenario_data),
            "structure_quality": self._assess_structure_quality(recommendation_text)
        }

        # Calculate overall quality score
        analysis["overall_quality_score"] = self._calculate_overall_score(analysis)
        analysis["quality_rating"] = self._get_quality_rating(analysis["overall_quality_score"])

        return analysis

    def _assess_kubernetes_relevance(self, text):
        """Assess how relevant the recommendation is to Kubernetes"""
        text_lower = text.lower()
        k8s_mentions = sum(1 for keyword in self.kubernetes_keywords if keyword in text_lower)

        # Look for kubectl commands
        kubectl_commands = len(re.findall(r'kubectl\s+\w+', text_lower))

        return {
            "kubernetes_keywords_found": k8s_mentions,
            "kubectl_commands_mentioned": kubectl_commands,
            "kubernetes_relevance_score": min(1.0, (k8s_mentions + kubectl_commands * 2) / 10),
            "highly_kubernetes_relevant": k8s_mentions >= 3 and kubectl_commands >= 1
        }

    def _assess_context_usage(self, text, scenario_data):
        """Assess how well the recommendation uses alert context"""
        text_lower = text.lower()

        # Check usage of context from alert labels and annotations
        alert = scenario_data["alert_data"]["alerts"][0]
        context_elements = []

        # Check label usage
        for label, value in alert["labels"].items():
            if str(value).lower() in text_lower:
                context_elements.append(f"label_{label}")

        # Check annotation usage
        for annotation, value in alert["annotations"].items():
            if str(value).lower() in text_lower:
                context_elements.append(f"annotation_{annotation}")

        # Check required context elements
        required_context = scenario_data.get("context_requirements", [])
        required_found = sum(1 for req in required_context if req.lower() in text_lower)

        return {
            "context_elements_used": len(context_elements),
            "context_details": context_elements,
            "required_context_found": required_found,
            "required_context_total": len(required_context),
            "context_usage_score": (len(context_elements) + required_found * 2) / max(1, len(required_context) * 3),
            "good_context_usage": required_found >= len(required_context) * 0.7
        }

    def _assess_actionability(self, text):
        """Assess how actionable the recommendations are"""
        text_lower = text.lower()

        # Count action verbs
        action_mentions = sum(1 for verb in self.action_verbs if verb in text_lower)

        # Look for step-by-step structure
        step_patterns = [
            r'\d+\.',  # 1. 2. 3.
            r'step \d+',  # step 1, step 2
            r'first,|second,|then,|next,|finally',  # sequence words
        ]

        step_indicators = sum(len(re.findall(pattern, text_lower)) for pattern in step_patterns)

        # Look for specific commands
        command_patterns = [
            r'kubectl [a-zA-Z]+',
            r'docker [a-zA-Z]+',
            r'sudo [a-zA-Z]+',
            r'systemctl [a-zA-Z]+',
        ]

        specific_commands = sum(len(re.findall(pattern, text_lower)) for pattern in command_patterns)

        return {
            "action_verbs_count": action_mentions,
            "step_indicators": step_indicators,
            "specific_commands": specific_commands,
            "actionability_score": min(1.0, (action_mentions + step_indicators + specific_commands * 2) / 10),
            "highly_actionable": action_mentions >= 3 and specific_commands >= 1
        }

    def _assess_specificity(self, text, scenario_data):
        """Assess how specific the recommendations are to the scenario"""
        text_lower = text.lower()

        # Check for specific values from the alert
        alert = scenario_data["alert_data"]["alerts"][0]
        specific_values_found = 0

        # Look for specific names and values
        for label_value in alert["labels"].values():
            if str(label_value).lower() in text_lower and len(str(label_value)) > 3:
                specific_values_found += 1

        for annotation_value in alert["annotations"].values():
            # Look for specific numeric values, names, paths
            value_words = str(annotation_value).split()
            for word in value_words:
                if len(word) > 4 and word.lower() in text_lower:
                    specific_values_found += 1
                    break

        # Check for expected remediation elements
        expected_elements = scenario_data.get("expected_remediation_elements", [])
        elements_found = sum(1 for element in expected_elements if element.lower() in text_lower)

        return {
            "specific_values_found": specific_values_found,
            "expected_elements_found": elements_found,
            "expected_elements_total": len(expected_elements),
            "specificity_score": (specific_values_found + elements_found) / max(1, len(expected_elements) + 3),
            "highly_specific": elements_found >= len(expected_elements) * 0.6
        }

    def _assess_structure_quality(self, text):
        """Assess the structural quality of the recommendation"""
        lines = text.split('\n')
        non_empty_lines = [line.strip() for line in lines if line.strip()]

        # Look for organized structure
        has_sections = any('##' in line or '**' in line or line.isupper() for line in non_empty_lines)
        has_lists = any(line.strip().startswith(('-', '*', '1.', '2.')) for line in non_empty_lines)

        return {
            "total_lines": len(lines),
            "non_empty_lines": len(non_empty_lines),
            "has_sections": has_sections,
            "has_lists": has_lists,
            "well_structured": has_sections or has_lists,
            "structure_score": (int(has_sections) + int(has_lists) + min(1, len(non_empty_lines) / 10)) / 3
        }

    def _calculate_overall_score(self, analysis):
        """Calculate overall quality score from individual assessments"""
        scores = [
            analysis["kubernetes_relevance"]["kubernetes_relevance_score"] * 0.25,
            analysis["context_usage"]["context_usage_score"] * 0.25,
            analysis["actionability"]["actionability_score"] * 0.25,
            analysis["specificity"]["specificity_score"] * 0.15,
            analysis["structure_quality"]["structure_score"] * 0.10
        ]

        return sum(scores)

    def _get_quality_rating(self, score):
        """Convert numeric score to quality rating"""
        if score >= 0.8:
            return "excellent"
        elif score >= 0.65:
            return "good"
        elif score >= 0.5:
            return "adequate"
        elif score >= 0.3:
            return "poor"
        else:
            return "unacceptable"

if __name__ == "__main__":
    # Test the analyzer with sample data
    analyzer = RemediationAnalyzer()

    sample_text = """
    To resolve the high memory usage in pod webapp-deployment-7b5d8f9c6d-xyz12:

    1. Check current resource usage: kubectl top pod webapp-deployment-7b5d8f9c6d-xyz12 -n production-workloads
    2. Examine pod logs: kubectl logs webapp-deployment-7b5d8f9c6d-xyz12 -n production-workloads
    3. Scale up memory limits: kubectl patch deployment webapp-deployment -n production-workloads -p '{"spec":{"template":{"spec":{"containers":[{"name":"webapp","resources":{"limits":{"memory":"8Gi"}}}]}}}}'
    4. Restart the pod: kubectl delete pod webapp-deployment-7b5d8f9c6d-xyz12 -n production-workloads
    """

    sample_scenario = {
        "alert_data": {
            "alerts": [{
                "labels": {"pod": "webapp-deployment-7b5d8f9c6d-xyz12", "namespace": "production-workloads"},
                "annotations": {"description": "High memory usage"}
            }]
        },
        "expected_remediation_elements": ["memory", "kubectl", "pod", "deployment"],
        "context_requirements": ["pod", "namespace"]
    }

    result = analyzer.analyze_recommendation_quality(sample_text, sample_scenario)
    print(f"Sample analysis: {result['quality_rating']} (score: {result['overall_quality_score']:.3f})")