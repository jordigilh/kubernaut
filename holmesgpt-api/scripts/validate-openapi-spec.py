#!/usr/bin/env python3
"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
OpenAPI Spec Validation Script

Business Requirement: BR-HAPI-260 (API Contract Validation)
Phase: Phase 4 - Automated Spec Validation

This script validates that the HAPI OpenAPI spec matches the Pydantic models.
It prevents spec/code drift and catches missing fields before commit.

Usage:
    python3 scripts/validate-openapi-spec.py

Exit Codes:
    0: Spec is valid
    1: Spec is invalid or out of sync

Integration:
    - Pre-commit hook: Runs automatically before commits
    - CI/CD: Runs in pipeline to catch drift
    - Manual: Run before regenerating clients

Authority: TRIAGE_HAPI_E2E_AND_CLIENT_GAPS.md (Phase 4)
"""

import json
import sys
import os
from pathlib import Path

# Add parent directory to path to import src module
script_dir = Path(__file__).parent.parent
sys.path.insert(0, str(script_dir))

from src.main import app
from src.models.incident_models import IncidentResponse


def validate_model_against_spec(model_class, schema_name, spec):
    """
    Validate that a Pydantic model matches its OpenAPI schema.

    Args:
        model_class: Pydantic model class
        schema_name: Name in OpenAPI spec components/schemas
        spec: OpenAPI spec dictionary

    Returns:
        tuple: (is_valid, errors)
    """
    errors = []

    # Get schema from spec
    if schema_name not in spec['components']['schemas']:
        errors.append(f"❌ Schema '{schema_name}' not found in OpenAPI spec")
        return False, errors

    schema = spec['components']['schemas'][schema_name]

    # Get model fields
    model_fields = set(model_class.model_fields.keys())
    spec_fields = set(schema.get('properties', {}).keys())

    # Check for missing fields in spec
    missing_in_spec = model_fields - spec_fields
    if missing_in_spec:
        errors.append(
            f"❌ {schema_name}: Fields in model but not in spec: {missing_in_spec}\n"
            f"   → Run: python3 scripts/generate-openapi-spec.py"
        )

    # Check for extra fields in spec (warning only)
    extra_in_spec = spec_fields - model_fields
    if extra_in_spec:
        errors.append(
            f"⚠️  {schema_name}: Fields in spec but not in model: {extra_in_spec}\n"
            f"   → Consider adding to Pydantic model or removing from spec"
        )

    # Check required fields
    spec_required = set(schema.get('required', []))
    model_required = {
        name for name, field in model_class.model_fields.items()
        if field.is_required()
    }

    missing_required = model_required - spec_required
    if missing_required:
        errors.append(
            f"❌ {schema_name}: Required in model but not in spec: {missing_required}"
        )

    is_valid = len([e for e in errors if e.startswith('❌')]) == 0
    return is_valid, errors


def validate_spec():
    """
    Validate HAPI OpenAPI spec matches Pydantic models.

    Returns:
        bool: True if valid, False if invalid
    """
    print("🔍 Validating HAPI OpenAPI Spec...")
    print("=" * 60)

    # Generate spec from FastAPI app
    try:
        spec = app.openapi()
    except Exception as e:
        print(f"❌ Failed to generate OpenAPI spec: {e}")
        return False

    # Validate critical models
    models_to_validate = [
        (IncidentResponse, "IncidentResponse"),
    ]

    all_valid = True
    all_errors = []

    for model_class, schema_name in models_to_validate:
        print(f"\n📋 Validating {schema_name}...")
        is_valid, errors = validate_model_against_spec(model_class, schema_name, spec)

        if errors:
            for error in errors:
                print(f"  {error}")
            all_errors.extend(errors)

        if not is_valid:
            all_valid = False
            print(f"  ❌ {schema_name}: INVALID")
        else:
            print(f"  ✅ {schema_name}: VALID")

    # Summary
    print("\n" + "=" * 60)
    if all_valid:
        print("✅ OpenAPI spec is valid and matches Pydantic models")
        print("=" * 60)
        return True
    else:
        print("❌ OpenAPI spec validation FAILED")
        print("=" * 60)
        print("\n🔧 To fix:")
        print("  1. Update Pydantic models to match spec, OR")
        print("  2. Regenerate spec: python3 scripts/generate-openapi-spec.py")
        print("  3. Commit both model and spec changes together")
        print("=" * 60)
        return False


def main():
    """Main entry point"""
    try:
        is_valid = validate_spec()
        sys.exit(0 if is_valid else 1)
    except Exception as e:
        print(f"\n❌ Unexpected error during validation: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()

