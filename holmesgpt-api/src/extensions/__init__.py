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
FastAPI extension routers for HolmesGPT API
"""

from . import health
# DD-017: PostExec extension deferred to V1.1 - Effectiveness Monitor not in V1.0
# from . import postexec

__all__ = [
    # "postexec",  # DD-017: Deferred to V1.1
    "health",
]
