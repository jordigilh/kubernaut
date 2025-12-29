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
to avoid duplication across incident.py, recovery.py, and postexec.py.

Per ADR-032 §2: Audit is MANDATORY for LLM interactions (P1 service).
Service MUST crash if audit store cannot be initialized.
"""

import os
import sys
import logging

from . import BufferedAuditStore, AuditConfig
from src.config.constants import (
    AUDIT_BUFFER_SIZE,
    AUDIT_BATCH_SIZE,
    AUDIT_FLUSH_INTERVAL_SECONDS,
)

logger = logging.getLogger(__name__)

# Global singleton instance (ADR-038)
_audit_store: BufferedAuditStore = None


def get_audit_store() -> BufferedAuditStore:
    """
    Get or initialize the audit store singleton.

    Business Requirement: BR-AUDIT-005 (Audit Trail)
    Design Decisions:
      - ADR-032 §1: Audit writes are MANDATORY, not best-effort
      - ADR-032 §2: Service MUST crash if audit cannot be initialized
      - ADR-038: Async Buffered Audit Ingestion

    Per ADR-032 §3: HAPI is P0 service - audit is MANDATORY for LLM interactions.

    Returns:
        BufferedAuditStore singleton

    Raises:
        SystemExit: If audit store cannot be initialized (ADR-032 §2)
    """
    global _audit_store
    if _audit_store is None:
        data_storage_url = os.getenv("DATA_STORAGE_URL", "http://data-storage:8080")
        try:
            _audit_store = BufferedAuditStore(
                data_storage_url=data_storage_url,
                config=AuditConfig(
                    buffer_size=AUDIT_BUFFER_SIZE,
                    batch_size=AUDIT_BATCH_SIZE,
                    flush_interval_seconds=AUDIT_FLUSH_INTERVAL_SECONDS
                )
            )
            logger.info(f"BR-AUDIT-005: Initialized audit store - url={data_storage_url}")
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

