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
Audit Store Factory

Business Requirement: BR-AUDIT-005 (Audit Trail)
Design Decisions:
  - ADR-032: Mandatory Audit Requirements (v1.3)
  - ADR-038: Async Buffered Audit Ingestion

Provides centralized initialization of BufferedAuditStore singleton
to avoid duplication across incident.py and postexec.py.

Per ADR-032 §2: Audit is MANDATORY for LLM interactions (P1 service).
Service MUST crash if audit store cannot be initialized.
"""

# Standard library imports
import logging  # noqa: E402
import os  # noqa: E402
import sys  # noqa: E402
from typing import Optional  # noqa: E402

# Local imports
from . import AuditConfig, BufferedAuditStore  # noqa: E402
from src.config.constants import (  # noqa: E402
    AUDIT_BUFFER_SIZE,
    AUDIT_BATCH_SIZE,
    AUDIT_FLUSH_INTERVAL_SECONDS,
)

logger = logging.getLogger(__name__)

# Global singleton instance (ADR-038)
_audit_store: BufferedAuditStore = None


def get_audit_store(
    data_storage_url: Optional[str] = None,
    flush_interval_seconds: Optional[float] = None,
    buffer_size: Optional[int] = None,
    batch_size: Optional[int] = None
) -> BufferedAuditStore:
    """
    Get or initialize the audit store singleton.

    Business Requirement: BR-AUDIT-005 (Audit Trail)
    Design Decisions:
      - ADR-032 §1: Audit writes are MANDATORY, not best-effort
      - ADR-032 §2: Service MUST crash if audit cannot be initialized
      - ADR-038: Async Buffered Audit Ingestion

    Per ADR-032 §3: HAPI is P0 service - audit is MANDATORY for LLM interactions.

    Args:
        data_storage_url: Data Storage service URL (defaults to DATA_STORAGE_URL env var)
        flush_interval_seconds: Seconds between flushes (defaults to constants.AUDIT_FLUSH_INTERVAL_SECONDS)
        buffer_size: Max events to buffer (defaults to constants.AUDIT_BUFFER_SIZE)
        batch_size: Events per batch (defaults to constants.AUDIT_BATCH_SIZE)

    Returns:
        BufferedAuditStore singleton

    Raises:
        SystemExit: If audit store cannot be initialized (ADR-032 §2)
    """
    global _audit_store
    if _audit_store is None:
        data_storage_url = data_storage_url or os.getenv("DATA_STORAGE_URL", "http://data-storage:8080")
        
        # AGGRESSIVE LOGGING: Print to stderr as backup (logger might not be configured yet)
        import sys as _sys
        print(f"🔍 HAPI AUDIT INIT START: url={data_storage_url}", file=_sys.stderr, flush=True)
        logger.info(f"🔍 BR-AUDIT-005: Starting audit store initialization - url={data_storage_url}")
        
        try:
            _audit_store = BufferedAuditStore(
                data_storage_url=data_storage_url,
                config=AuditConfig(
                    buffer_size=buffer_size or AUDIT_BUFFER_SIZE,
                    batch_size=batch_size or AUDIT_BATCH_SIZE,
                    flush_interval_seconds=flush_interval_seconds or AUDIT_FLUSH_INTERVAL_SECONDS
                )
            )
            # AGGRESSIVE LOGGING: Print to stderr + logger
            print(f"✅ HAPI AUDIT INIT SUCCESS: url={data_storage_url}", file=_sys.stderr, flush=True)
            logger.info(f"✅ BR-AUDIT-005: Initialized audit store - url={data_storage_url}")
        except Exception as e:
            # ✅ COMPLIANT: Crash immediately per ADR-032 §2
            # Per ADR-032 §2: "Services MUST fail fast and exit(1) if audit cannot be initialized"
            logger.error(
                f"FATAL: Failed to initialize audit store - audit is MANDATORY per ADR-032 §2: {e}",
                extra={
                    "data_storage_url": data_storage_url,
                    "adr": "ADR-032 §2",
                    "service_classification": "P1",
                }
            )
            sys.exit(1)  # Crash - NO RECOVERY ALLOWED per ADR-032 §2
    return _audit_store

