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
Authentication and Authorization package for HolmesGPT API

Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)

This package provides authentication and authorization interfaces and implementations
for Kubernetes-based REST API services using dependency injection.

The interfaces allow for:
- Production: Real Kubernetes TokenReview + SubjectAccessReview (SAR)
- Integration tests: Mock implementations (auth still enforced)
- E2E tests: Real Kubernetes APIs in Kind clusters

Security: No runtime disable flags - auth is always enforced via interface implementations.
"""

from .interfaces import Authenticator, Authorizer
from .k8s_auth import K8sAuthenticator, K8sAuthorizer
from .mock_auth import MockAuthenticator, MockAuthorizer

__all__ = [
    "Authenticator",
    "Authorizer",
    "K8sAuthenticator",
    "K8sAuthorizer",
    "MockAuthenticator",
    "MockAuthorizer",
]
