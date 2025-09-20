#!/usr/bin/env python3
"""
Test Result Analysis Framework
Reusable analysis modules for integration test validation following project guidelines.

Business Requirements Supported:
- BR-PA-001: Availability analysis and reporting
- BR-PA-002 through BR-PA-013: Various test result validations
"""
import json
import os
import sys
import statistics
from datetime import datetime
from typing import Dict, List, Any, Optional, Tuple
import logging

# Configure logging following project guidelines (always log errors)
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

class TestResultAnalyzer:
    """Base class for test result analysis following project guidelines"""

    def __init__(self, test_session: str = None):
        """Initialize analyzer with test session context"""
        self.test_session = test_session or os.environ.get('TEST_SESSION', 'test_session')
        self.results_dir = f"results/{self.test_session}"

        # Ensure results directory exists
        if not os.path.exists(self.results_dir):
            error_msg = f"Results directory not found: {self.results_dir}"
            logger.error(error_msg)
            raise FileNotFoundError(error_msg)

    def load_json_result(self, filename: str) -> Dict[str, Any]:
        """Load and validate JSON test results with proper error handling"""
        filepath = os.path.join(self.results_dir, filename)

        try:
            with open(filepath, 'r', encoding='utf-8') as f:
                data = json.load(f)
                logger.info(f"Successfully loaded test results from {filename}")
                return data
        except FileNotFoundError:
            error_msg = f"Test result file not found: {filepath}"
            logger.error(error_msg)
            raise FileNotFoundError(error_msg)
        except json.JSONDecodeError as e:
            error_msg = f"Invalid JSON in {filepath}: {e}"
            logger.error(error_msg)
            raise ValueError(error_msg)
        except Exception as e:
            error_msg = f"Unexpected error loading {filepath}: {e}"
            logger.error(error_msg)
            raise

    def validate_business_requirement_compliance(self, actual_value: float,
                                               required_value: float,
                                               requirement_name: str,
                                               higher_is_better: bool = True) -> Dict[str, Any]:
        """
        Validate compliance with business requirements using strong assertions
        Follows project guidelines: test business requirements, not implementation
        """
        try:
            if higher_is_better:
                compliant = actual_value >= required_value
                margin = actual_value - required_value
            else:
                compliant = actual_value <= required_value
                margin = required_value - actual_value

            result = {
                "requirement_name": requirement_name,
                "required_value": required_value,
                "actual_value": actual_value,
                "compliant": compliant,
                "margin": margin,
                "margin_percentage": (margin / required_value * 100) if required_value != 0 else 0,
                "validation_timestamp": datetime.now().isoformat()
            }

            logger.info(f"Business requirement validation: {requirement_name} - {'PASS' if compliant else 'FAIL'}")
            return result

        except Exception as e:
            error_msg = f"Error validating business requirement {requirement_name}: {e}"
            logger.error(error_msg)
            raise

class AvailabilityAnalyzer(TestResultAnalyzer):
    """Specialized analyzer for BR-PA-001 availability requirements"""

    def analyze_availability_compliance(self, results_filename: str = "availability_detailed_results.json") -> Dict[str, Any]:
        """Analyze availability test results for BR-PA-001 compliance"""
        try:
            data = self.load_json_result(results_filename)

            # Validate required fields exist
            required_fields = ['availability_percentage', 'br_pa_001_compliance', 'downtime_seconds']
            for field in required_fields:
                if field not in data:
                    raise ValueError(f"Required field missing from results: {field}")

            # Business requirement validation: 99.9% availability
            compliance_data = data['br_pa_001_compliance']

            analysis = {
                "test_session": self.test_session,
                "analysis_type": "availability_compliance",
                "business_requirement": "BR-PA-001",
                "test_duration_minutes": data.get('test_duration_minutes', 0),
                "total_checks": data.get('total_checks', 0),
                "availability_percentage": data['availability_percentage'],
                "downtime_seconds": data['downtime_seconds'],
                "compliance_result": self.validate_business_requirement_compliance(
                    actual_value=data['availability_percentage'],
                    required_value=99.9,
                    requirement_name="BR-PA-001 Availability Requirement",
                    higher_is_better=True
                ),
                "downtime_incidents": len(data.get('downtime_periods', [])),
                "analysis_timestamp": datetime.now().isoformat()
            }

            # Additional business validations
            if data.get('downtime_periods'):
                max_downtime = max(p['duration'] for p in data['downtime_periods'])
                analysis["max_single_downtime_seconds"] = max_downtime
                analysis["restart_recovery_compliance"] = self.validate_business_requirement_compliance(
                    actual_value=max_downtime,
                    required_value=30.0,
                    requirement_name="BR-PA-001 Recovery Time Requirement",
                    higher_is_better=False
                )

            return analysis

        except Exception as e:
            error_msg = f"Error analyzing availability compliance: {e}"
            logger.error(error_msg)
            raise

class ConcurrentHandlingAnalyzer(TestResultAnalyzer):
    """Specialized analyzer for BR-PA-004 concurrent handling requirements"""

    def analyze_concurrent_performance(self, results_filename: str = "concurrent_test_results.json") -> Dict[str, Any]:
        """Analyze concurrent handling test results for BR-PA-004 compliance"""
        try:
            data = self.load_json_result(results_filename)

            # Validate business requirements for concurrent handling
            success_rate = data.get('success_rate_percentage', 0)
            avg_response_time = data.get('average_response_time_ms', 0)
            concurrent_requests = data.get('concurrent_requests', 0)

            analysis = {
                "test_session": self.test_session,
                "analysis_type": "concurrent_handling_compliance",
                "business_requirement": "BR-PA-004",
                "concurrent_requests": concurrent_requests,
                "success_rate_percentage": success_rate,
                "average_response_time_ms": avg_response_time,
                "success_rate_compliance": self.validate_business_requirement_compliance(
                    actual_value=success_rate,
                    required_value=95.0,  # Business requirement: 95% success rate
                    requirement_name="BR-PA-004 Success Rate Requirement",
                    higher_is_better=True
                ),
                "response_time_compliance": self.validate_business_requirement_compliance(
                    actual_value=avg_response_time,
                    required_value=5000,  # Business requirement: <5s response time
                    requirement_name="BR-PA-004 Response Time Requirement",
                    higher_is_better=False
                ),
                "analysis_timestamp": datetime.now().isoformat()
            }

            return analysis

        except Exception as e:
            error_msg = f"Error analyzing concurrent handling performance: {e}"
            logger.error(error_msg)
            raise

class AIDecisionAnalyzer(TestResultAnalyzer):
    """Specialized analyzer for AI decision making requirements (BR-PA-006 through BR-PA-010)"""

    def analyze_llm_provider_compliance(self, results_filename: str = "llm_provider_results.json") -> Dict[str, Any]:
        """Analyze LLM provider test results for BR-PA-006 compliance"""
        try:
            data = self.load_json_result(results_filename)

            # Business requirement validation for multi-provider support
            provider_count = data.get('active_providers', 0)
            failover_success_rate = data.get('failover_success_rate', 0)

            analysis = {
                "test_session": self.test_session,
                "analysis_type": "llm_provider_compliance",
                "business_requirement": "BR-PA-006",
                "active_providers": provider_count,
                "failover_success_rate": failover_success_rate,
                "provider_diversity_compliance": self.validate_business_requirement_compliance(
                    actual_value=provider_count,
                    required_value=2,  # Business requirement: minimum 2 providers
                    requirement_name="BR-PA-006 Provider Diversity Requirement",
                    higher_is_better=True
                ),
                "failover_reliability_compliance": self.validate_business_requirement_compliance(
                    actual_value=failover_success_rate,
                    required_value=98.0,  # Business requirement: 98% failover success
                    requirement_name="BR-PA-006 Failover Reliability Requirement",
                    higher_is_better=True
                ),
                "analysis_timestamp": datetime.now().isoformat()
            }

            return analysis

        except Exception as e:
            error_msg = f"Error analyzing LLM provider compliance: {e}"
            logger.error(error_msg)
            raise

class ReportGenerator:
    """Report generation utility following project guidelines"""

    def __init__(self, test_session: str = None):
        self.test_session = test_session or os.environ.get('TEST_SESSION', 'test_session')
        self.results_dir = f"results/{self.test_session}"

    def generate_business_requirement_report(self, analysis_results: List[Dict[str, Any]],
                                           output_filename: str) -> str:
        """Generate comprehensive business requirement compliance report"""
        try:
            report_path = os.path.join(self.results_dir, output_filename)

            with open(report_path, 'w', encoding='utf-8') as f:
                f.write(f"# Business Requirement Compliance Report\n\n")
                f.write(f"**Test Session**: {self.test_session}\n")
                f.write(f"**Generated**: {datetime.now().isoformat()}\n\n")

                overall_compliance = True
                total_requirements = len(analysis_results)
                passed_requirements = 0

                for analysis in analysis_results:
                    f.write(f"## {analysis.get('business_requirement', 'Unknown Requirement')}\n\n")
                    f.write(f"**Analysis Type**: {analysis.get('analysis_type', 'N/A')}\n")

                    # Check all compliance results in the analysis
                    for key, value in analysis.items():
                        if key.endswith('_compliance') and isinstance(value, dict):
                            compliant = value.get('compliant', False)
                            requirement_name = value.get('requirement_name', key)
                            actual_value = value.get('actual_value', 'N/A')
                            required_value = value.get('required_value', 'N/A')
                            margin = value.get('margin', 0)

                            status = "✅ PASS" if compliant else "❌ FAIL"
                            f.write(f"**{requirement_name}**: {status}\n")
                            f.write(f"- Required: {required_value}\n")
                            f.write(f"- Actual: {actual_value}\n")
                            f.write(f"- Margin: {margin:.4f}\n\n")

                            if compliant:
                                passed_requirements += 1
                            else:
                                overall_compliance = False

                # Summary
                compliance_percentage = (passed_requirements / total_requirements * 100) if total_requirements > 0 else 0
                f.write(f"## Summary\n\n")
                f.write(f"**Overall Compliance**: {'✅ PASS' if overall_compliance else '❌ FAIL'}\n")
                f.write(f"**Requirements Passed**: {passed_requirements}/{total_requirements}\n")
                f.write(f"**Compliance Rate**: {compliance_percentage:.1f}%\n")

            logger.info(f"Business requirement report generated: {report_path}")
            return report_path

        except Exception as e:
            error_msg = f"Error generating business requirement report: {e}"
            logger.error(error_msg)
            raise

def main():
    """Main function for command-line usage"""
    if len(sys.argv) < 3:
        print("Usage: python3 test_result_analyzer.py <analysis_type> <results_file> [test_session]")
        print("Analysis types: availability, concurrent, llm_provider")
        sys.exit(1)

    analysis_type = sys.argv[1]
    results_file = sys.argv[2]
    test_session = sys.argv[3] if len(sys.argv) > 3 else None

    try:
        if analysis_type == "availability":
            analyzer = AvailabilityAnalyzer(test_session)
            results = analyzer.analyze_availability_compliance(results_file)
        elif analysis_type == "concurrent":
            analyzer = ConcurrentHandlingAnalyzer(test_session)
            results = analyzer.analyze_concurrent_performance(results_file)
        elif analysis_type == "llm_provider":
            analyzer = AIDecisionAnalyzer(test_session)
            results = analyzer.analyze_llm_provider_compliance(results_file)
        else:
            raise ValueError(f"Unknown analysis type: {analysis_type}")

        # Generate report
        report_gen = ReportGenerator(test_session)
        report_path = report_gen.generate_business_requirement_report([results], f"{analysis_type}_compliance_report.md")

        print(json.dumps(results, indent=2))
        print(f"\nDetailed report generated: {report_path}")

    except Exception as e:
        logger.error(f"Analysis failed: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
