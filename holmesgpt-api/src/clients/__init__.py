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
External service clients for HolmesGPT API

NOTE: mcp_client.py has been REMOVED per DD-WORKFLOW-002 v2.4.
Workflow catalog search is now handled by WorkflowCatalogToolset
which calls the Data Storage Service REST API directly.
"""

# Only DataStorage client is exported - MCPClient removed per DD-WORKFLOW-002 v2.4
from src.clients.datastorage.client import DataStorageClient

__all__ = ["DataStorageClient"]

