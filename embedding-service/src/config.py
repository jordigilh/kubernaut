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
Configuration for Kubernaut Embedding Service

Provides environment-based configuration with sensible defaults.
"""

import os
from typing import Optional


class Config:
    """
    Service configuration with environment variable support.

    All settings can be overridden via environment variables.
    """

    # Server Configuration
    HOST: str = os.getenv("EMBEDDING_SERVICE_HOST", "0.0.0.0")
    PORT: int = int(os.getenv("EMBEDDING_SERVICE_PORT", "8086"))

    # Model Configuration
    MODEL_NAME: str = os.getenv("EMBEDDING_MODEL_NAME", "sentence-transformers/all-mpnet-base-v2")
    MODEL_DIMENSIONS: int = 768
    DEVICE: str = os.getenv("EMBEDDING_DEVICE", "cpu")  # "cpu" or "cuda"

    # Performance Configuration
    MAX_BATCH_SIZE: int = int(os.getenv("EMBEDDING_MAX_BATCH_SIZE", "32"))
    CACHE_DIR: Optional[str] = os.getenv("TRANSFORMERS_CACHE", None)

    # Logging Configuration
    LOG_LEVEL: str = os.getenv("LOG_LEVEL", "INFO")

    @classmethod
    def validate(cls) -> None:
        """Validate configuration values."""
        assert cls.PORT > 0 and cls.PORT < 65536, f"Invalid port: {cls.PORT}"
        assert cls.MODEL_DIMENSIONS == 768, f"Invalid dimensions: {cls.MODEL_DIMENSIONS}"
        assert cls.DEVICE in ["cpu", "cuda"], f"Invalid device: {cls.DEVICE}"
        assert cls.MAX_BATCH_SIZE > 0, f"Invalid batch size: {cls.MAX_BATCH_SIZE}"


# Validate configuration on import
Config.validate()

