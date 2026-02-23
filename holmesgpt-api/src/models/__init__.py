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
HolmesGPT API Models
"""

from .recovery_models import RecoveryRequest, RecoveryResponse, RecoveryStrategy
from .incident_models import IncidentRequest, IncidentResponse, AlternativeWorkflow
# DD-017: PostExec models deferred to V1.1 — EM Level 1 (V1.0, DD-017 v2.0) does not use PostExec; Level 2 (V1.1) is the consumer
# from .postexec_models import PostExecRequest, PostExecResponse, EffectivenessAssessment

__all__ = [
    # Incident Analysis
    "IncidentRequest",
    "IncidentResponse",
    "AlternativeWorkflow",

    # Recovery
    "RecoveryRequest",
    "RecoveryResponse",
    "RecoveryStrategy",

    # DD-017: PostExec models deferred to V1.1 — EM Level 1 (V1.0, DD-017 v2.0) does not use PostExec; Level 2 (V1.1) is the consumer
    # "PostExecRequest",
    # "PostExecResponse",
    # "EffectivenessAssessment",
]
