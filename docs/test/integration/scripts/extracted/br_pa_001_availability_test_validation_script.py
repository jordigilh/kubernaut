#!/usr/bin/env python3
"""
Extracted script from test_suites/01_alert_processing/BR-PA-001_availability_test.md
Business requirement: Extracted following project guidelines for reusability
"""
import json
with open('results/$TEST_SESSION/availability_detailed_results.json', 'r') as f:
    data = json.load(f)
exit(0 if data['br_pa_001_compliance']['pass'] else 1)
