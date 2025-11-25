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
Kubernaut Embedding Service

Text-to-vector embedding service using sentence-transformers.
Provides semantic search capabilities for the Data Storage service.

Model: all-mpnet-base-v2 (768 dimensions, 92% accuracy)
Framework: FastAPI with Pydantic validation
Deployment: Sidecar container in Data Storage pod
"""

__version__ = "1.0.0"

