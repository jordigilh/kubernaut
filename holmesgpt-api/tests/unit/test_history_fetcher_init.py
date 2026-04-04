# Copyright 2026 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
Issue #616: Regression gate tests for history_fetcher init with TypedDict config.

BR-HAPI-016: Remediation history context for LLM prompt enrichment.
TP-616-v1.1: Validates create_remediation_history_api works with plain dict
(TypedDict at runtime), confirming the .to_dict() call at the bug site is wrong.

These are regression gates — they pass immediately because the function itself
is correct. The bug is at the CALL SITE (llm_integration.py:820) which calls
app_config.to_dict() on a TypedDict.
"""

import os
from unittest.mock import MagicMock, patch

import pytest

from src.clients.remediation_history_client import create_remediation_history_api


class TestHistoryFetcherInit:
    """UT-HAPI-616-001 through UT-HAPI-616-003: TypedDict config compatibility."""

    @patch("datastorage_pool_manager.get_shared_datastorage_pool_manager")
    def test_create_api_with_typeddict_config(self, mock_pool):
        """UT-HAPI-616-001: create_remediation_history_api works with TypedDict/dict config."""
        mock_pool.return_value = MagicMock()

        config = {"data_storage_url": "http://data-storage:8080", "service_name": "test"}

        result = create_remediation_history_api(config)

        assert result is not None, (
            "create_remediation_history_api should succeed with a plain dict "
            "(TypedDict at runtime). Bug: llm_integration.py:820 calls "
            "app_config.to_dict() which raises AttributeError on dict/TypedDict."
        )

    @patch("datastorage_pool_manager.get_shared_datastorage_pool_manager")
    def test_create_api_returns_valid_instance(self, mock_pool):
        """UT-HAPI-616-002: create_remediation_history_api returns API instance when DS URL configured."""
        mock_pool.return_value = MagicMock()

        config = {"data_storage_url": "http://data-storage:8080"}

        result = create_remediation_history_api(config)

        assert result is not None
        assert hasattr(result, "get_remediation_history_context"), (
            "Returned API should have the get_remediation_history_context method"
        )

    def test_create_api_returns_none_without_ds_url(self):
        """UT-HAPI-616-003: create_remediation_history_api returns None when DS URL not configured."""
        with patch.dict(os.environ, {}, clear=True):
            os.environ.pop("DATA_STORAGE_URL", None)

            result = create_remediation_history_api({})

            assert result is None, (
                "Should return None when no data_storage_url in config and no "
                "DATA_STORAGE_URL env var"
            )
