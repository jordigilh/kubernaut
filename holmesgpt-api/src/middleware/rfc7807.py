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
RFC 7807 Error Response Middleware for FastAPI

Business Requirement: BR-HAPI-200 - RFC 7807 Error Response Standard
Design Decision: DD-004 - RFC 7807 Problem Details

This middleware converts all FastAPI exceptions to RFC 7807 Problem Details format.

REFACTOR phase enhancements:
- Prometheus metrics for error responses
- Enhanced structured logging
"""

import logging
import uuid
from fastapi import Request, status
from fastapi.responses import JSONResponse
from fastapi.exceptions import RequestValidationError, HTTPException
from starlette.exceptions import HTTPException as StarletteHTTPException
from pydantic import ValidationError

from src.errors import create_rfc7807_error
from src.middleware.metrics import rfc7807_errors_total

logger = logging.getLogger(__name__)


async def rfc7807_exception_handler(request: Request, exc: Exception) -> JSONResponse:
    """
    Convert exceptions to RFC 7807 Problem Details format

    Business Requirement: BR-HAPI-200

    Handles:
    - RequestValidationError (Pydantic validation) → 400
    - HTTPException (FastAPI) → status from exception
    - StarletteHTTPException → status from exception
    - All other exceptions → 500
    """
    # Get or generate request ID
    request_id = request.headers.get("X-Request-ID", str(uuid.uuid4()))

    # Get request path for instance field
    instance = request.url.path

    # Determine status code and detail based on exception type
    # 🔍 DEBUG: Check for Pydantic ValidationError first (before Starlette wraps it)
    if isinstance(exc, ValidationError):
        # Pure Pydantic validation error (caught before Starlette wrapping)
        status_code = status.HTTP_400_BAD_REQUEST
        # Extract field name and message from first error
        # exc.errors() returns: [{'loc': ('body', 'field_name'), 'msg': '...', 'type': '...'}]
        first_error = exc.errors()[0]
        field_name = first_error['loc'][-1] if first_error.get('loc') else 'unknown'
        error_msg = first_error['msg']
        detail = f"Pydantic validation error: Field '{field_name}': {error_msg}"
        logger.error({
            "event": "pydantic_validation_error_raw",
            "request_id": request_id,
            "path": instance,
            "errors": exc.errors(),
            "error_count": len(exc.errors()),
        })
    elif isinstance(exc, RequestValidationError):
        # FastAPI RequestValidationError (wraps Pydantic ValidationError)
        status_code = status.HTTP_400_BAD_REQUEST
        # Extract field name and message from first error
        # exc.errors() returns: [{'loc': ('body', 'field_name'), 'msg': '...', 'type': '...'}]
        first_error = exc.errors()[0]
        field_name = first_error['loc'][-1] if first_error.get('loc') else 'unknown'
        error_msg = first_error['msg']
        detail = f"Validation error: Field '{field_name}': {error_msg}"
        logger.warning({
            "event": "validation_error",
            "request_id": request_id,
            "path": instance,
            "errors": exc.errors()
        })
    elif isinstance(exc, HTTPException):
        # FastAPI HTTPException
        status_code = exc.status_code
        detail = exc.detail
        logger.warning({
            "event": "http_exception",
            "request_id": request_id,
            "path": instance,
            "status_code": status_code,
            "detail": detail
        })
    elif isinstance(exc, StarletteHTTPException):
        # Starlette HTTPException
        status_code = exc.status_code
        detail = exc.detail
        logger.warning({
            "event": "starlette_http_exception",
            "request_id": request_id,
            "path": instance,
            "status_code": status_code,
            "detail": detail
        })
    else:
        # Unexpected exception → 500 Internal Server Error
        status_code = status.HTTP_500_INTERNAL_SERVER_ERROR
        detail = "An unexpected error occurred"
        logger.error({
            "event": "unexpected_error",
            "request_id": request_id,
            "path": instance,
            "error_type": type(exc).__name__,
            "error": str(exc)
        }, exc_info=True)

    # Create RFC 7807 error response
    error = create_rfc7807_error(
        status_code=status_code,
        detail=detail,
        instance=instance,
        request_id=request_id
    )

    # REFACTOR phase: Record error metrics
    # BR-HAPI-200: Track RFC 7807 error responses
    rfc7807_errors_total.labels(
        status_code=str(status_code),
        error_type=error.type.split('/')[-1]  # Extract error type from URI
    ).inc()

    # Return JSON response with RFC 7807 format
    response = JSONResponse(
        status_code=status_code,
        content=error.dict(),
        headers={
            "Content-Type": "application/problem+json",
            "X-Request-ID": request_id
        }
    )
    
    # 🔍 DEBUG: Log response creation (troubleshooting 400→401 conversion)
    logger.info({
        "event": "rfc7807_response_created",
        "request_id": request_id,
        "status_code": status_code,
        "response_type": type(response).__name__,
        "detail": detail[:100] if len(detail) > 100 else detail,
    })
    
    return response


def add_rfc7807_exception_handlers(app):
    """
    Register RFC 7807 exception handlers with FastAPI app

    Business Requirement: BR-HAPI-200
    """
    # 🔍 DEBUG: Register ValidationError handler FIRST (highest priority)
    app.add_exception_handler(ValidationError, rfc7807_exception_handler)
    app.add_exception_handler(RequestValidationError, rfc7807_exception_handler)
    app.add_exception_handler(HTTPException, rfc7807_exception_handler)
    app.add_exception_handler(StarletteHTTPException, rfc7807_exception_handler)
    app.add_exception_handler(Exception, rfc7807_exception_handler)
    
    logger.info("RFC 7807 exception handlers registered (with ValidationError debug)")

