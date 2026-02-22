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
Validation Module for HolmesGPT-API

Business Requirements:
- BR-AI-023: Hallucination detection (workflow existence validation)
- BR-HAPI-191: Parameter validation in chat session
- BR-HAPI-196: Container image consistency validation

Design Decision: DD-HAPI-002 v1.2 - Workflow Response Validation Architecture
"""

from .workflow_response_validator import (
    WorkflowResponseValidator,
    ValidationResult,
)

__all__ = [
    "WorkflowResponseValidator",
    "ValidationResult",
]


