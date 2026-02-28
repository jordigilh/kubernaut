#!/usr/bin/env python3
"""
Quick validator test - run inside HAPI container or with dependencies installed

Run with: python3 test_validators_quick.py
"""

import sys
import os

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'src'))

try:
    from pydantic import ValidationError
    from src.models.incident_models import IncidentRequest

    print("✅ Imports successful")
except ImportError as e:
    print(f"❌ Import failed: {e}")
    print("Install dependencies: pip install pydantic")
    sys.exit(1)


def test_empty_remediation_id():
    """E2E-HAPI-008: Test empty remediation_id"""
    print("\n" + "="*80)
    print("TEST 1: Empty remediation_id (E2E-HAPI-008)")
    print("="*80)
    
    try:
        request = IncidentRequest(
            incident_id="test-001",
            remediation_id="",  # Empty string
            signal_type="CrashLoopBackOff",
            severity="high",
            signal_source="kubernetes",
            resource_namespace="default",
            resource_kind="Pod",
            resource_name="test-pod"
        )
        print("❌ FAIL: IncidentRequest accepted empty remediation_id")
        print(f"   Created request: remediation_id='{request.remediation_id}'")
        return False
    except ValidationError as e:
        print("✅ PASS: ValidationError raised as expected")
        print(f"   Error: {e}")
        return True
    except Exception as e:
        print(f"❌ FAIL: Unexpected error: {type(e).__name__}: {e}")
        return False


def test_missing_remediation_id():
    """E2E-HAPI-008: Test missing remediation_id"""
    print("\n" + "="*80)
    print("TEST 2: Missing remediation_id (E2E-HAPI-008)")
    print("="*80)
    
    try:
        request = IncidentRequest(
            incident_id="test-001",
            # remediation_id NOT provided
            signal_type="CrashLoopBackOff",
            severity="high",
            signal_source="kubernetes",
            resource_namespace="default",
            resource_kind="Pod",
            resource_name="test-pod"
        )
        print("❌ FAIL: IncidentRequest accepted missing remediation_id")
        print(f"   Created request: remediation_id='{getattr(request, 'remediation_id', 'NOT SET')}'")
        return False
    except ValidationError as e:
        print("✅ PASS: ValidationError raised as expected")
        print(f"   Error: {e}")
        return True
    except TypeError as e:
        print("✅ PASS: TypeError raised (field is required)")
        print(f"   Error: {e}")
        return True
    except Exception as e:
        print(f"❌ FAIL: Unexpected error: {type(e).__name__}: {e}")
        return False


def test_valid_inputs():
    """Test that valid inputs pass"""
    print("\n" + "="*80)
    print("TEST 3: Valid inputs should pass")
    print("="*80)
    
    try:
        incident_req = IncidentRequest(
            incident_id="test-001",
            remediation_id="test-rem-001",
            signal_type="CrashLoopBackOff",
            severity="high",
            signal_source="kubernetes",
            resource_namespace="default",
            resource_kind="Pod",
            resource_name="test-pod"
        )
        print("✅ PASS: IncidentRequest created successfully")
        print(f"   remediation_id='{incident_req.remediation_id}'")
        return True
    except Exception as e:
        print(f"❌ FAIL: Valid inputs raised error: {type(e).__name__}: {e}")
        return False


def main():
    print("="*80)
    print("PYDANTIC VALIDATOR UNIT TESTS")
    print("Testing @field_validator decorators for E2E-HAPI-008")
    print("="*80)
    
    results = []
    results.append(("Empty remediation_id", test_empty_remediation_id()))
    results.append(("Missing remediation_id", test_missing_remediation_id()))
    results.append(("Valid inputs", test_valid_inputs()))
    
    print("\n" + "="*80)
    print("SUMMARY")
    print("="*80)
    passed = sum(1 for _, result in results if result)
    total = len(results)
    
    for name, result in results:
        status = "✅ PASS" if result else "❌ FAIL"
        print(f"{status}: {name}")
    
    print(f"\n{passed}/{total} tests passed ({passed/total*100:.1f}%)")
    
    if passed == total:
        print("\n✅ ALL TESTS PASSED - Pydantic validators are working correctly!")
        print("   Issue is likely in FastAPI request handling or Go client marshalling")
        sys.exit(0)
    else:
        print("\n❌ SOME TESTS FAILED - Pydantic validators are not working as expected")
        sys.exit(1)


if __name__ == "__main__":
    main()
