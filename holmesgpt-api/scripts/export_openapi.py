#!/usr/bin/env python3
#
# Copyright 2025 Jordi Gil.
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
#

"""
Export OpenAPI spec from HAPI without requiring a Kubernetes cluster.

Authority: ADR-045 (OpenAPI Spec Export)
Issue: #393 (replaces DISABLE_K8S_AUTH / OPENAPI_EXPORT env vars)

Creates the FastAPI app with mock auth injected via dependency injection,
then dumps the OpenAPI schema as JSON to stdout.
"""

import json

from tests.helpers.mock_auth import MockAuthenticator, MockAuthorizer
from src.main import create_app

app = create_app(
    authenticator=MockAuthenticator(),
    authorizer=MockAuthorizer(default_allow=True),
)

print(json.dumps(app.openapi(), indent=2))
