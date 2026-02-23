#!/usr/bin/env python3
"""
Debug script to test OpenAPI client encoding of IncidentRequest.

Purpose: Isolate whether the body parsing error is due to client serialization.
"""
import sys
sys.path.insert(0, 'tests/clients')

from holmesgpt_api_client.models import IncidentRequest
import json

# Same test data that's failing in E2E
test_data = {
    "incident_id": "test-123",
    "remediation_id": "rem-456",
    "signal_name": "OOMKilled",
    "severity": "critical",
    "signal_source": "prometheus",
    "resource_namespace": "production",
    "resource_kind": "Pod",
    "resource_name": "app-xyz-123",
    "cluster_name": "e2e-test-cluster",
    "environment": "production",
    "priority": "P1",
    "risk_tolerance": "medium",
    "business_category": "standard",
    "error_message": "Container killed due to OOM",
}

print("=" * 70)
print("OPENAPI CLIENT ENCODING DEBUG")
print("=" * 70)
print()

# Create OpenAPI client model
try:
    incident_req = IncidentRequest(**test_data)
    print("✅ IncidentRequest created successfully")
    print(f"   Type: {type(incident_req)}")
    print()
except Exception as e:
    print(f"❌ Failed to create IncidentRequest: {e}")
    sys.exit(1)

# Check to_dict()
try:
    dict_output = incident_req.to_dict()
    print("📦 to_dict() output:")
    print(f"   Fields: {len(dict_output)}")
    print(f"   Keys: {list(dict_output.keys())}")
    print()
except Exception as e:
    print(f"❌ to_dict() failed: {e}")
    sys.exit(1)

# Check model_dump()
try:
    dump_output = incident_req.model_dump()
    print("📦 model_dump() output:")
    print(f"   Fields: {len(dump_output)}")
    print(f"   None fields: {[k for k, v in dump_output.items() if v is None]}")
    print()
except Exception as e:
    print(f"❌ model_dump() failed: {e}")
    sys.exit(1)

# Check JSON serialization
try:
    json_output = incident_req.to_json()
    print("📦 to_json() output:")
    print(f"   Length: {len(json_output)} bytes")
    print(f"   Valid JSON: {json.loads(json_output) is not None}")
    print()
    print("JSON Content (first 500 chars):")
    print(json_output[:500])
    print()
except Exception as e:
    print(f"❌ to_json() failed: {e}")
    sys.exit(1)

# Validate against Pydantic model
try:
    # Try to recreate from JSON (simulating server-side parsing)
    recreated = IncidentRequest.from_json(json_output)
    print("✅ Server-side parsing simulation: SUCCESS")
    print(f"   Recreated type: {type(recreated)}")
    print()
except Exception as e:
    print("❌ Server-side parsing simulation: FAILED")
    print(f"   Error: {e}")
    print()

# Compare to direct dict creation
try:
    direct_model = IncidentRequest(**test_data)
    direct_json = direct_model.to_json()
    
    if json_output == direct_json:
        print("✅ Client serialization matches direct model creation")
    else:
        print("⚠️  Client serialization DIFFERS from direct model")
        print(f"   Length diff: {len(json_output)} vs {len(direct_json)}")
except Exception as e:
    print(f"❌ Comparison failed: {e}")

print()
print("=" * 70)
print("CONCLUSION:")
if json.loads(json_output):
    print("✅ OpenAPI client produces valid JSON")
    print("   → Body parsing error likely due to server-side issue")
    print("   → Check HAPI middleware/FastAPI configuration")
else:
    print("❌ OpenAPI client produces INVALID JSON")
    print("   → Need to regenerate client from updated spec")
print("=" * 70)
