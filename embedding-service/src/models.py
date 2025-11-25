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
Pydantic models for Kubernaut Embedding Service

Provides request/response validation for the embedding API.
"""

from typing import List
from pydantic import BaseModel, Field, field_validator


class EmbedRequest(BaseModel):
    """
    Request model for embedding generation.

    Validates that text is non-empty and within reasonable length limits.
    """
    text: str = Field(
        ...,
        min_length=1,
        max_length=10000,
        description="Text to generate embedding for (1-10000 characters)"
    )

    @field_validator('text')
    @classmethod
    def text_must_not_be_whitespace(cls, v: str) -> str:
        """Validate that text is not just whitespace."""
        if not v.strip():
            raise ValueError('text must contain non-whitespace characters')
        return v.strip()


class EmbedResponse(BaseModel):
    """
    Response model for embedding generation.

    Returns 768-dimensional embedding vector with metadata.
    """
    embedding: List[float] = Field(
        ...,
        min_length=768,
        max_length=768,
        description="768-dimensional embedding vector"
    )
    dimensions: int = Field(
        default=768,
        description="Number of dimensions in embedding vector"
    )
    model: str = Field(
        default="all-mpnet-base-v2",
        description="Model used for embedding generation"
    )

    @field_validator('embedding')
    @classmethod
    def validate_dimensions(cls, v: List[float]) -> List[float]:
        """Validate embedding has exactly 768 dimensions."""
        if len(v) != 768:
            raise ValueError(f'embedding must have exactly 768 dimensions, got {len(v)}')
        return v


class HealthResponse(BaseModel):
    """
    Response model for health check endpoint.

    Provides service status and model information.
    """
    status: str = Field(
        default="healthy",
        description="Service health status"
    )
    model: str = Field(
        default="all-mpnet-base-v2",
        description="Embedding model name"
    )
    dimensions: int = Field(
        default=768,
        description="Embedding vector dimensions"
    )


class ErrorResponse(BaseModel):
    """
    Response model for error responses.

    Provides structured error information.
    """
    error: str = Field(
        ...,
        description="Error message"
    )
    detail: str = Field(
        default="",
        description="Detailed error information"
    )

