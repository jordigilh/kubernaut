"""
Root-level pytest configuration.

This file is loaded BEFORE tests/conftest.py and BEFORE test collection.
It configures the Python path to include the OpenAPI-generated DataStorage client.
"""

import sys
from pathlib import Path

# Add src/ to PYTHONPATH so bare imports like 'from clients.xxx' resolve correctly.
# Also add the OpenAPI-generated DataStorage client sub-package.
project_root = Path(__file__).parent
src_path = project_root / "src"
datastorage_client_path = src_path / "clients" / "datastorage"

for p in (str(src_path), str(datastorage_client_path)):
    if p not in sys.path:
        sys.path.insert(0, p)

