#!/usr/bin/env python3
"""
Script to convert test_mock_llm_mode_integration.py to use HAPI OpenAPI client

This script automates the conversion from requests.post() to OpenAPI client calls.
"""

import re
import sys

def convert_file():
    input_file = 'test_mock_llm_mode_integration.py'
    output_file = 'test_mock_llm_mode_integration.py.new'

    with open(input_file, 'r') as f:
        content = f.read()

    # Step 1: Update imports
    old_imports = """import os
import pytest
import requests
from unittest.mock import patch"""

    new_imports = """import os
import pytest
import sys
sys.path.insert(0, 'tests/clients')

from holmesgpt_api_client import ApiClient, Configuration
from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi
from holmesgpt_api_client.models.incident_request import IncidentRequest
from holmesgpt_api_client.exceptions import ApiException"""

    content = content.replace(old_imports, new_imports)

    # Step 2: Update docstring
    content = content.replace(
        'These tests use requests library with real HAPI service HTTP calls (not TestClient).',
        'These tests use HAPI OpenAPI client to validate API contract compliance.'
    )
    content = content.replace(
        'INTEGRATION TEST COMPLIANCE:',
        'INTEGRATION TEST COMPLIANCE (MIGRATED TO OPENAPI CLIENT):'
    )

    # Step 3: Add helper functions at the end of fixtures
    helper_functions = '''

# Helper functions for OpenAPI client usage
def create_incident_api(hapi_service_url):
    """Create IncidentAnalysisApi instance"""
    config = Configuration(host=hapi_service_url)
    client = ApiClient(configuration=config)
    return IncidentAnalysisApi(client)

def dict_to_incident_request(data):
    """Convert dict to IncidentRequest model"""
    return IncidentRequest(**data)

'''

    # Insert helpers before first test class
    fixture_end = content.find('class TestMockModeIncidentIntegration:')
    if fixture_end > 0:
        content = content[:fixture_end] + helper_functions + '\n\n' + content[fixture_end:]

    # Step 4: Convert incident endpoint calls
    # Pattern: response = requests.post(f"{hapi_service_url}/api/v1/incident/analyze", json=...)
    incident_pattern = r'response = requests\.post\(\s*f"{hapi_service_url}/api/v1/incident/analyze",\s*json=([^)]+)\s*\)'

    def replace_incident(match):
        request_var = match.group(1).strip()
        if request_var.startswith('sample_incident_request'):
            return f'''incidents_api = create_incident_api(hapi_service_url)
        incident_request = dict_to_incident_request({request_var})
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )'''
        else:
            return f'''incidents_api = create_incident_api(hapi_service_url)
        incident_request = dict_to_incident_request({request_var})
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )'''

    content = re.sub(incident_pattern, replace_incident, content)

    # Step 5: Update assertions from response.status_code to direct field access
    # Most assertions check response.status_code == 200, which becomes implicit with OpenAPI client
    # Some check response.json()["field"], which becomes response.field

    # Write converted content
    with open(output_file, 'w') as f:
        f.write(content)

    print(f"✅ Conversion complete: {output_file}")
    print(f"   Review the file and rename to {input_file} if correct")
    return 0

if __name__ == '__main__':
    sys.exit(convert_file())


