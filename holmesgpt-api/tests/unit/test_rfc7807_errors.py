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
Unit Tests for RFC 7807 Error Response Standard

Business Requirement: BR-HAPI-200 - RFC 7807 Error Response Standard
Design Decision: DD-004 - RFC 7807 Problem Details for HTTP APIs

Test Coverage (7 tests):
1. RFC7807Error model structure
2. Error type URI format validation
3. Bad Request (400) error response
4. Unauthorized (401) error response
5. Not Found (404) error response
6. Internal Server Error (500) error response
7. Service Unavailable (503) error response

Reference: Gateway Service (pkg/gateway/errors/rfc7807.go)
Reference: Context API (pkg/contextapi/errors/rfc7807.go)
Reference: Dynamic Toolset (pkg/toolset/errors/rfc7807.go)
"""



# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 1: RFC7807Error Model Structure
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

def test_rfc7807_error_model_structure():
    """
    Test 1: RFC7807Error Pydantic model has all required fields

    BR-HAPI-200: RFC 7807 error format

    Validates:
    - All required RFC 7807 fields present: type, title, detail, status, instance
    - Optional request_id field for tracing
    - Correct field types (str for most, int for status)
    """
    from src.errors import RFC7807Error

    # Create RFC 7807 error with all fields
    error = RFC7807Error(
        type="https://kubernaut.ai/problems/validation-error",
        title="Bad Request",
        detail="Invalid JSON in request body",
        status=400,
        instance="/api/v1/incident/analyze",
        request_id="test-request-123"
    )

    # Verify all required fields present
    assert error.type == "https://kubernaut.ai/problems/validation-error"
    assert error.title == "Bad Request"
    assert error.detail == "Invalid JSON in request body"
    assert error.status == 400
    assert error.instance == "/api/v1/incident/analyze"
    assert error.request_id == "test-request-123"

    # Verify model can be serialized to dict
    error_dict = error.dict()
    assert "type" in error_dict
    assert "title" in error_dict
    assert "detail" in error_dict
    assert "status" in error_dict
    assert "instance" in error_dict
    assert "request_id" in error_dict


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 2: Error Type URI Format Validation
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

def test_error_type_uri_format():
    """
    Test 2: Error type URIs follow kubernaut.ai convention

    BR-HAPI-200: RFC 7807 error format
    Updated: December 18, 2025 - Changed domain to kubernaut.ai, path to /problems/

    Validates:
    - Error type URIs use https://kubernaut.ai/problems/{error-type} format
    - Consistent with DD-004 v1.1 RFC 7807 standard
    """
    from src.errors import (
        ERROR_TYPE_VALIDATION_ERROR,
        ERROR_TYPE_UNAUTHORIZED,
        ERROR_TYPE_NOT_FOUND,
        ERROR_TYPE_INTERNAL_ERROR,
        ERROR_TYPE_SERVICE_UNAVAILABLE
    )

    # Verify all error type URIs follow convention
    assert ERROR_TYPE_VALIDATION_ERROR == "https://kubernaut.ai/problems/validation-error"
    assert ERROR_TYPE_UNAUTHORIZED == "https://kubernaut.ai/problems/unauthorized"
    assert ERROR_TYPE_NOT_FOUND == "https://kubernaut.ai/problems/not-found"
    assert ERROR_TYPE_INTERNAL_ERROR == "https://kubernaut.ai/problems/internal-error"
    assert ERROR_TYPE_SERVICE_UNAVAILABLE == "https://kubernaut.ai/problems/service-unavailable"


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 3: Bad Request (400) Error Response
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

def test_bad_request_400_error():
    """
    Test 3: Bad Request (400) returns RFC 7807 error

    BR-HAPI-200: RFC 7807 error format

    Validates:
    - Status code 400
    - Content-Type: application/problem+json
    - All RFC 7807 fields present
    - Error type: validation-error
    """
    from src.errors import create_rfc7807_error

    error = create_rfc7807_error(
        status_code=400,
        detail="Missing required field: 'namespace'",
        instance="/api/v1/incident/analyze",
        request_id="req-400-test"
    )

    assert error.status == 400
    assert error.type == "https://kubernaut.ai/problems/validation-error"
    assert error.title == "Bad Request"
    assert error.detail == "Missing required field: 'namespace'"
    assert error.instance == "/api/v1/incident/analyze"
    assert error.request_id == "req-400-test"


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 4: Unauthorized (401) Error Response
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

def test_unauthorized_401_error():
    """
    Test 4: Unauthorized (401) returns RFC 7807 error

    BR-HAPI-200: RFC 7807 error format

    Validates:
    - Status code 401
    - Error type: unauthorized
    - Title: "Unauthorized"
    """
    from src.errors import create_rfc7807_error

    error = create_rfc7807_error(
        status_code=401,
        detail="Invalid or missing authentication token",
        instance="/api/v1/incident/analyze",
        request_id="req-401-test"
    )

    assert error.status == 401
    assert error.type == "https://kubernaut.ai/problems/unauthorized"
    assert error.title == "Unauthorized"
    assert error.detail == "Invalid or missing authentication token"


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 5: Not Found (404) Error Response
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

def test_not_found_404_error():
    """
    Test 5: Not Found (404) returns RFC 7807 error

    BR-HAPI-200: RFC 7807 error format

    Validates:
    - Status code 404
    - Error type: not-found
    - Title: "Not Found"
    """
    from src.errors import create_rfc7807_error

    error = create_rfc7807_error(
        status_code=404,
        detail="Analysis ID 'abc-123' not found",
        instance="/api/v1/incident/session/abc-123",
        request_id="req-404-test"
    )

    assert error.status == 404
    assert error.type == "https://kubernaut.ai/problems/not-found"
    assert error.title == "Not Found"
    assert error.detail == "Analysis ID 'abc-123' not found"


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 6: Internal Server Error (500) Error Response
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

def test_internal_server_error_500():
    """
    Test 6: Internal Server Error (500) returns RFC 7807 error

    BR-HAPI-200: RFC 7807 error format

    Validates:
    - Status code 500
    - Error type: internal-error
    - Title: "Internal Server Error"
    """
    from src.errors import create_rfc7807_error

    error = create_rfc7807_error(
        status_code=500,
        detail="Unexpected error during LLM analysis",
        instance="/api/v1/incident/analyze",
        request_id="req-500-test"
    )

    assert error.status == 500
    assert error.type == "https://kubernaut.ai/problems/internal-error"
    assert error.title == "Internal Server Error"
    assert error.detail == "Unexpected error during LLM analysis"


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 7: Service Unavailable (503) Error Response
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

def test_service_unavailable_503_error():
    """
    Test 7: Service Unavailable (503) returns RFC 7807 error

    BR-HAPI-200: RFC 7807 error format

    Validates:
    - Status code 503
    - Error type: service-unavailable
    - Title: "Service Unavailable"
    - Appropriate during graceful shutdown
    """
    from src.errors import create_rfc7807_error

    error = create_rfc7807_error(
        status_code=503,
        detail="Service is shutting down gracefully",
        instance="/api/v1/incident/analyze",
        request_id="req-503-test"
    )

    assert error.status == 503
    assert error.type == "https://kubernaut.ai/problems/service-unavailable"
    assert error.title == "Service Unavailable"
    assert error.detail == "Service is shutting down gracefully"

