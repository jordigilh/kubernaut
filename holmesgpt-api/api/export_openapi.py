#!/usr/bin/env python3
"""
Export OpenAPI specification from FastAPI application.

Usage:
    python3 api/export_openapi.py

Output:
    api/openapi.json - OpenAPI 3.1.0 specification (native FastAPI output)

ADR-045: AIAnalysis ↔ HolmesGPT-API Service Contract
- This script exports the auto-generated OpenAPI spec from FastAPI/Pydantic models
- Uses native OpenAPI 3.1.0 format (FastAPI default)
- AIAnalysis team uses `ogen` to generate Go client (supports 3.1.0 natively)

Client Generation:
    # Install ogen (supports OpenAPI 3.1.0)
    go install github.com/ogen-go/ogen/cmd/ogen@latest

    # Generate Go client
    ogen -package holmesgpt -target pkg/clients/holmesgpt \
        holmesgpt-api/api/openapi.json

Source Models:
- src/models/incident_models.py - IncidentRequest, IncidentResponse, DetectedLabels
- src/models/postexec_models.py - Post-execution analysis models

Related:
- DD-HAPI-001: DetectedLabels for workflow filtering
"""

import json
import sys
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent))

from src.main import app


def export_openapi():
    """Export OpenAPI spec to JSON file (native 3.1.0 format)."""
    output_path = Path(__file__).parent / "openapi.json"

    # Get OpenAPI schema from FastAPI app (generates 3.1.0)
    openapi_schema = app.openapi()

    # Write to file
    with open(output_path, "w") as f:
        json.dump(openapi_schema, f, indent=2)

    print(f"✅ OpenAPI spec exported to: {output_path}")
    print(f"   Title: {openapi_schema.get('info', {}).get('title')}")
    print(f"   Version: {openapi_schema.get('info', {}).get('version')}")
    print(f"   OpenAPI: {openapi_schema.get('openapi')}")
    print(f"   Paths: {len(openapi_schema.get('paths', {}))}")
    print(f"   Schemas: {len(openapi_schema.get('components', {}).get('schemas', {}))}")
    print()
    print("📦 Generate Go client with ogen (supports 3.1.0):")
    print("   go install github.com/ogen-go/ogen/cmd/ogen@latest")
    print("   ogen -package holmesgpt -target pkg/clients/holmesgpt api/openapi.json")

    return output_path


if __name__ == "__main__":
    export_openapi()
