"""
Root-level pytest configuration.

This file is loaded BEFORE tests/conftest.py and BEFORE test collection.
It configures the Python path to include the OpenAPI-generated DataStorage client.
"""

import sys
from pathlib import Path

# Add datastorage client to PYTHONPATH for OpenAPI-generated types
# This allows imports like: from datastorage.models.llm_request_payload import LLMRequestPayload
project_root = Path(__file__).parent
datastorage_client_path = project_root / "src" / "clients" / "datastorage"

if str(datastorage_client_path) not in sys.path:
    sys.path.insert(0, str(datastorage_client_path))

