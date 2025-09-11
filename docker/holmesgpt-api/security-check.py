#!/usr/bin/env python3
"""
Security check script for HolmesGPT source build
Evaluates security scan results and fails build if critical issues found
"""

import json
import sys
import os
from typing import Dict, List, Any

# Security thresholds
MAX_HIGH_VULNERABILITIES = 0
MAX_MEDIUM_VULNERABILITIES = 5
MAX_LOW_VULNERABILITIES = 20

def load_json_report(filepath: str) -> Dict[str, Any]:
    """Load JSON report file safely."""
    try:
        if os.path.exists(filepath):
            with open(filepath, 'r') as f:
                return json.load(f)
    except (json.JSONDecodeError, FileNotFoundError) as e:
        print(f"⚠️  Warning: Could not load {filepath}: {e}")
    return {}

def evaluate_safety_report(report: Dict[str, Any]) -> tuple:
    """Evaluate safety scan results."""
    vulnerabilities = report.get('vulnerabilities', [])

    high_count = 0
    medium_count = 0
    low_count = 0

    for vuln in vulnerabilities:
        severity = vuln.get('vulnerability', {}).get('severity', 'unknown').lower()
        if severity in ['critical', 'high']:
            high_count += 1
            print(f"🔴 HIGH: {vuln.get('package', 'unknown')} - {vuln.get('vulnerability', {}).get('summary', 'No summary')}")
        elif severity == 'medium':
            medium_count += 1
            print(f"🟡 MEDIUM: {vuln.get('package', 'unknown')} - {vuln.get('vulnerability', {}).get('summary', 'No summary')}")
        else:
            low_count += 1

    return high_count, medium_count, low_count

def evaluate_bandit_report(report: Dict[str, Any]) -> tuple:
    """Evaluate bandit scan results."""
    results = report.get('results', [])

    high_count = 0
    medium_count = 0
    low_count = 0

    for result in results:
        severity = result.get('issue_severity', 'unknown').lower()
        confidence = result.get('issue_confidence', 'unknown').lower()

        # Only count high confidence issues
        if confidence in ['high', 'medium']:
            if severity == 'high':
                high_count += 1
                print(f"🔴 BANDIT HIGH: {result.get('test_name', 'unknown')} in {result.get('filename', 'unknown')}")
            elif severity == 'medium':
                medium_count += 1
                print(f"🟡 BANDIT MEDIUM: {result.get('test_name', 'unknown')} in {result.get('filename', 'unknown')}")
            else:
                low_count += 1

    return high_count, medium_count, low_count

def evaluate_pip_audit_report(report: Dict[str, Any]) -> tuple:
    """Evaluate pip-audit scan results."""
    vulnerabilities = report.get('vulnerabilities', [])

    high_count = 0
    medium_count = 0
    low_count = 0

    for vuln in vulnerabilities:
        # pip-audit doesn't always provide severity, so we categorize by type
        vuln_id = vuln.get('id', 'unknown')
        package = vuln.get('package', {}).get('name', 'unknown')

        # Assume all pip-audit findings are at least medium severity
        if 'critical' in vuln.get('description', '').lower():
            high_count += 1
            print(f"🔴 PIP-AUDIT HIGH: {package} - {vuln_id}")
        else:
            medium_count += 1
            print(f"🟡 PIP-AUDIT MEDIUM: {package} - {vuln_id}")

    return high_count, medium_count, low_count

def main():
    """Main security evaluation function."""
    print("🔍 Evaluating security scan results...")

    # Load reports
    safety_report = load_json_report('/tmp/safety-report.json')
    bandit_report = load_json_report('/tmp/bandit-report.json')
    pip_audit_report = load_json_report('/tmp/pip-audit-report.json')

    # Evaluate each report
    safety_high, safety_medium, safety_low = evaluate_safety_report(safety_report)
    bandit_high, bandit_medium, bandit_low = evaluate_bandit_report(bandit_report)
    pip_high, pip_medium, pip_low = evaluate_pip_audit_report(pip_audit_report)

    # Aggregate results
    total_high = safety_high + bandit_high + pip_high
    total_medium = safety_medium + bandit_medium + pip_medium
    total_low = safety_low + bandit_low + pip_low

    print(f"\n📊 Security Scan Summary:")
    print(f"   🔴 High/Critical: {total_high}")
    print(f"   🟡 Medium:        {total_medium}")
    print(f"   🟢 Low:           {total_low}")

    # Check thresholds
    failed = False

    if total_high > MAX_HIGH_VULNERABILITIES:
        print(f"❌ FAIL: {total_high} high/critical vulnerabilities found (max: {MAX_HIGH_VULNERABILITIES})")
        failed = True

    if total_medium > MAX_MEDIUM_VULNERABILITIES:
        print(f"❌ FAIL: {total_medium} medium vulnerabilities found (max: {MAX_MEDIUM_VULNERABILITIES})")
        failed = True

    if total_low > MAX_LOW_VULNERABILITIES:
        print(f"⚠️  WARNING: {total_low} low vulnerabilities found (max: {MAX_LOW_VULNERABILITIES})")
        # Don't fail on low severity, just warn

    if failed:
        print("\n🚫 Security scan FAILED - Build aborted for security reasons")
        print("   Please review and fix security issues before building")
        sys.exit(1)
    else:
        print("\n✅ Security scan PASSED - Build can continue")

        # Save security summary for build metadata
        security_summary = {
            "scan_date": "2025-01-01",  # Would be actual date
            "high_vulnerabilities": total_high,
            "medium_vulnerabilities": total_medium,
            "low_vulnerabilities": total_low,
            "scan_tools": ["safety", "bandit", "pip-audit"],
            "status": "passed"
        }

        with open('/tmp/security-summary.json', 'w') as f:
            json.dump(security_summary, f, indent=2)

if __name__ == "__main__":
    main()
