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
Incident Analysis LLM Integration

Business Requirements:
- BR-HAPI-002 (Incident Analysis)
- BR-HAPI-197 (needs_human_review field)
- BR-HAPI-211 (LLM Input Sanitization)
- BR-HAPI-212 (Mock LLM Mode)
- BR-HAPI-250 (Workflow Catalog Toolset)
- BR-AUDIT-005 (Audit Trail)

Design Decisions:
- DD-HAPI-002 v1.2 (LLM Self-Correction Loop)
- DD-HOLMESGPT-014 (MinimalDAL Stateless Architecture)
- DD-WORKFLOW-001 v1.7 (DetectedLabels validation)
- ADR-038 (Async Buffered Audit Ingestion)

This module contains the core LLM integration logic for incident analysis,
including HolmesGPT SDK integration, self-correction loop, and audit trail.
"""

import os
import logging
from typing import Dict, Any, Optional, List

from src.models.config_models import AppConfig
from datetime import datetime, timezone
from fastapi import HTTPException, status

# HolmesGPT SDK imports
from holmes.config import Config
from holmes.core.models import InvestigateRequest
from holmes.core.investigation import investigate_issues

# Audit imports (BR-AUDIT-005, ADR-038, DD-AUDIT-002)
from src.audit import (
    get_audit_store,
    create_validation_attempt_event,
)

from .constants import MAX_VALIDATION_ATTEMPTS
from .prompt_builder import (
    create_incident_investigation_prompt,
    build_validation_error_feedback
)
from .result_parser import parse_and_validate_investigation_result
from src.extensions.investigation_helpers import (
    audit_llm_request,
    audit_llm_response_and_tools,
    handle_validation_exhaustion,
)

# Import models for type handling
from src.models.incident_models import DetectedLabels

logger = logging.getLogger(__name__)


# ========================================
# MINIMAL DAL (DD-HOLMESGPT-014)
# ========================================

class MinimalDAL:
    """
    Minimal DAL for HolmesGPT SDK integration (no Robusta Platform)

    Architecture Decision (DD-HOLMESGPT-014):
    Kubernaut does NOT integrate with Robusta Platform.

    Kubernaut Provides Equivalent Features Via:
    - Workflow catalog → PostgreSQL with Data Storage Service (not Robusta Platform)
    - Historical data → Context API (not Supabase)
    - Custom investigation logic → Rego policies in RemediationExecution Controller
    - LLM credentials → Kubernetes Secrets (not database)
    - Remediation state → CRDs (RemediationRequest, AIAnalysis, RemediationExecution)

    Result: No Robusta Platform database integration needed.

    This MinimalDAL satisfies HolmesGPT SDK's DAL interface requirements
    without connecting to any Robusta Platform database.

    Note: We still install supabase/postgrest dependencies (~50MB) because
    the SDK requires them, but this class ensures they're never used at runtime.

    See: docs/decisions/DD-HOLMESGPT-014-MinimalDAL-Stateless-Architecture.md
    """
    def __init__(self, cluster_name: str = "unknown"):
        self.cluster_name = cluster_name
        self.enabled = True  # Always enabled for Kubernaut (no Robusta Platform toggle)

    def get_issues(self, *args, **kwargs):
        """Return empty list - no historical issues from Robusta Platform"""
        return []

    def get_issue(self, *args, **kwargs):
        """Return None - no issue data from Robusta Platform"""
        return None

    def get_issue_data(self, *args, **kwargs):
        """Return None - no issue data from Robusta Platform"""
        return None

    def get_resource_instructions(self, *args, **kwargs):
        """Return None - no resource-specific instructions from Robusta Platform"""
        return None

    def get_global_instructions_for_account(self, *args, **kwargs):
        """Return None - no global account instructions from Robusta Platform"""
        return None

    def get_account_id(self, *args, **kwargs):
        """Return None - no Robusta Platform account"""
        return None

    def get_cluster_name(self, *args, **kwargs):
        """Return cluster name from initialization"""
        return self.cluster_name


def create_data_storage_client(app_config: Optional[AppConfig]):
    """
    Create Data Storage client for workflow validation.

    DD-HAPI-002 v1.2: Client used to validate workflow existence, container image, and parameters.

    Returns:
        WorkflowCatalogAPIApi or None if configuration is missing
    """
    try:
        from datastorage.api.workflow_catalog_api_api import WorkflowCatalogAPIApi
        from datastorage.api_client import ApiClient
        from datastorage.configuration import Configuration

        # Get Data Storage URL from config or environment
        ds_url = None
        if app_config:
            ds_url = app_config.get("data_storage_url") or app_config.get("DATA_STORAGE_URL")
        if not ds_url:
            ds_url = os.getenv("DATA_STORAGE_URL", "http://data-storage:8080")

        # DD-AUTH-014: Use ServiceAccount authentication
        # Performance Fix: Use singleton pool manager to reuse HTTP connections
        from datastorage_pool_manager import get_shared_datastorage_pool_manager
        auth_pool = get_shared_datastorage_pool_manager()
        configuration = Configuration(host=ds_url)
        api_client = ApiClient(configuration)
        api_client.rest_client.pool_manager = auth_pool  # Inject ServiceAccount token
        return WorkflowCatalogAPIApi(api_client)
    except Exception as e:
        logger.warning({
            "event": "data_storage_client_creation_failed",
            "error": str(e),
            "message": "Workflow validation will be skipped"
        })
        return None


async def analyze_incident(
    request_data: Dict[str, Any],
    mcp_config: Optional[Dict[str, Any]] = None,
    app_config: Optional[AppConfig] = None,
    metrics=None  # Injectable HAMetrics instance (Go pattern)
) -> Dict[str, Any]:
    """
    Core incident analysis logic with LLM self-correction loop.

    Business Requirements:
    - BR-HAPI-002 (Incident analysis)
    - BR-HAPI-011 (Investigation metrics)
    - BR-HAPI-197 (needs_human_review field)
    - BR-HAPI-211 (LLM Input Sanitization)
    - BR-HAPI-212 (Mock LLM Mode)
    - BR-HAPI-250 (Workflow Catalog Toolset)
    - BR-HAPI-301 (LLM observability metrics)

    Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)

    Self-Correction Loop:
    1. Call HolmesGPT SDK for RCA and workflow selection
    2. Validate workflow response (existence, image, parameters)
    3. If invalid, feed errors back to LLM for self-correction
    4. Retry up to MAX_VALIDATION_ATTEMPTS times
    5. If all attempts fail, set needs_human_review=True
    
    Args:
        request_data: Incident request data dict
        mcp_config: Optional MCP configuration
        app_config: Optional application configuration
        metrics: Optional HAMetrics instance (injected by caller, uses global if None)
    """
    import time
    
    # Start timing for BR-HAPI-011 (Investigation metrics)
    start_time = time.time()
    
    incident_id = request_data.get("incident_id", "unknown")

    logger.info({
        "event": "incident_analysis_started",
        "incident_id": incident_id,
        "signal_type": request_data.get("signal_type")
    })

    # BR-AUDIT-005: Initialize audit store
    # Per ADR-032 §1: Audit is MANDATORY for ALL LLM interactions
    audit_store = get_audit_store()
    remediation_id = request_data.get("remediation_id", "")

    # Use HolmesGPT SDK for AI-powered analysis (calls standalone Mock LLM in E2E)
    try:
        # BR-HAPI-016: Query remediation history from DataStorage for prompt enrichment
        # Graceful degradation: if DS unavailable or module not yet deployed, context is None
        remediation_history_context = None
        try:
            from src.clients.remediation_history_client import (
                create_remediation_history_api,
                fetch_remediation_history_for_request,
            )
            rh_api = create_remediation_history_api(app_config)
            enrichment_results = request_data.get("enrichment_results", {}) or {}
            current_spec_hash = ""
            if isinstance(enrichment_results, dict):
                current_spec_hash = enrichment_results.get("currentSpecHash", "")
            remediation_history_context = fetch_remediation_history_for_request(
                api=rh_api,
                request_data=request_data,
                current_spec_hash=current_spec_hash,
            )
        except (ImportError, Exception) as rh_err:
            logger.warning({"event": "remediation_history_unavailable", "error": str(rh_err)})

        # Create base investigation prompt
        # BR-HAPI-211: Sanitize prompt BEFORE sending to LLM to prevent credential leakage
        from src.sanitization import sanitize_for_llm
        base_prompt = sanitize_for_llm(create_incident_investigation_prompt(
            request_data, remediation_history_context=remediation_history_context
        ))

        # Create minimal DAL
        dal = MinimalDAL(cluster_name=request_data.get("cluster_name"))

        # Create HolmesGPT config with workflow catalog toolset (BR-HAPI-250)
        # Get formatted model name for litellm (supports Ollama, OpenAI, Claude, Vertex AI)
        from src.extensions.llm_config import (
            get_model_config_for_sdk,
            prepare_toolsets_config_for_sdk,
            register_workflow_discovery_toolset
        )

        try:
            model_name, provider = get_model_config_for_sdk(app_config)
        except ValueError as e:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )

        # Prepare toolsets configuration (BR-HAPI-002: Enable toolsets by default, BR-HAPI-250: Workflow catalog)
        toolsets_config = prepare_toolsets_config_for_sdk(app_config)

        # Create HolmesGPT SDK Config
        # NOTE: api_key is obtained from OPENAI_API_KEY environment variable via SDK's model registry
        # Do NOT pass api_key to Config() - it's not a valid field and will cause Pydantic validation error
        config = Config(
            model=model_name,
            api_base=os.getenv("LLM_ENDPOINT"),
            toolsets=toolsets_config,
            mcp_servers=app_config.get("mcp_servers", {}) if app_config else {},
        )

        # BR-HAPI-250: Register workflow catalog toolset programmatically
        # BR-AUDIT-001: Pass remediation_id for audit trail correlation (DD-WORKFLOW-002 v2.2)
        # remediation_id is MANDATORY per DD-WORKFLOW-002 v2.2 - used for CORRELATION ONLY
        remediation_id = request_data.get("remediation_id")
        if not remediation_id:
            logger.warning({
                "event": "missing_remediation_id",
                "incident_id": incident_id,
                "message": "remediation_id not provided - audit trail will be incomplete"
            })

        # DD-HAPI-001: Extract custom_labels from enrichment_results for auto-append
        # Custom labels are passed to WorkflowCatalogToolset and auto-appended to all MCP calls
        # The LLM does NOT see or provide these - they are operational metadata
        enrichment_results = request_data.get("enrichment_results", {}) or {}
        if hasattr(enrichment_results, 'customLabels'):
            # Pydantic model - access attribute directly
            custom_labels = enrichment_results.customLabels
        elif isinstance(enrichment_results, dict):
            # Dict - access via key (camelCase from K8s)
            custom_labels = enrichment_results.get("customLabels")
        else:
            custom_labels = None

        if custom_labels:
            logger.info({
                "event": "custom_labels_extracted",
                "incident_id": incident_id,
                "subdomains": list(custom_labels.keys()),
                "message": f"DD-HAPI-001: {len(custom_labels)} custom label subdomains will be auto-appended to workflow search"
            })

        # DD-WORKFLOW-001 v1.7: Extract detected_labels for workflow matching (100% safe)
        detected_labels_for_toolset = None
        if enrichment_results:
            if hasattr(enrichment_results, 'detectedLabels') and enrichment_results.detectedLabels:
                dl = enrichment_results.detectedLabels
                # Keep as DetectedLabels Pydantic model (or convert dict to model)
                if isinstance(dl, DetectedLabels):
                    detected_labels_for_toolset = dl
                elif isinstance(dl, dict):
                    detected_labels_for_toolset = DetectedLabels(**dl)
            elif isinstance(enrichment_results, dict):
                dl = enrichment_results.get('detectedLabels', {})
                if dl:
                    # Convert dict to DetectedLabels Pydantic model
                    if isinstance(dl, dict):
                        detected_labels_for_toolset = DetectedLabels(**dl)
                    elif isinstance(dl, DetectedLabels):
                        detected_labels_for_toolset = dl

        # DD-WORKFLOW-001 v1.7: Extract source_resource for DetectedLabels validation
        # This is the original signal's resource - compared against LLM's rca_resource
        source_resource = {
            "namespace": request_data.get("resource_namespace", ""),
            "kind": request_data.get("resource_kind", ""),
            "name": request_data.get("resource_name", "")
        }

        # DD-WORKFLOW-001 v1.7: Extract owner_chain from enrichment_results
        # This is the K8s ownership chain from SignalProcessing (via ownerReferences)
        # Format: [{"namespace": "prod", "kind": "ReplicaSet", "name": "..."}, {"kind": "Deployment", ...}]
        # Used for PROVEN relationship validation (100% safe)
        owner_chain = None
        if enrichment_results:
            if hasattr(enrichment_results, 'ownerChain'):
                owner_chain = enrichment_results.ownerChain
            elif isinstance(enrichment_results, dict):
                owner_chain = enrichment_results.get('ownerChain')

        if detected_labels_for_toolset:
            # Get non-None fields from DetectedLabels model for logging
            label_fields = [f for f, v in detected_labels_for_toolset.model_dump(exclude_none=True).items() if f != "failedDetections"]
            logger.info({
                "event": "detected_labels_extracted",
                "incident_id": incident_id,
                "fields": label_fields,
                "source_resource": f"{source_resource.get('kind')}/{source_resource.get('namespace') or 'cluster'}",
                "owner_chain_length": len(owner_chain) if owner_chain else 0,
                "message": f"DD-WORKFLOW-001 v1.7: {len(label_fields)} detected labels (100% safe validation)"
            })

        # DD-HAPI-017: Register three-step workflow discovery toolset
        # (replaces single search_workflow_catalog tool from DD-WORKFLOW-002)
        config = register_workflow_discovery_toolset(
            config,
            app_config,
            remediation_id=remediation_id,
            custom_labels=custom_labels,
            detected_labels=detected_labels_for_toolset,
            severity=request_data.get("severity", ""),
            component=request_data.get("resource_kind", ""),
            environment=request_data.get("environment", ""),
            priority=request_data.get("priority", ""),
        )

        # DD-HAPI-002 v1.2: Create Data Storage client for workflow validation
        data_storage_client = create_data_storage_client(app_config)

        # BR-AUDIT-005: audit_store and remediation_id already initialized at function start
        # (Moved before mock check to support audit in mock mode - BR-HAPI-212 + BR-AUDIT-005)

        # ========================================
        # LLM SELF-CORRECTION LOOP (DD-HAPI-002 v1.2)
        # With full audit trail (BR-AUDIT-005)
        # ========================================
        validation_errors_history: List[List[str]] = []
        validation_attempts_history: List[Dict[str, Any]] = []  # For response
        last_schema_hint: Optional[str] = None  # BR-HAPI-191: schema hint for self-correction
        result = None
        workflow_id = None

        for attempt in range(MAX_VALIDATION_ATTEMPTS):
            attempt_timestamp = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

            # Build prompt with error feedback for retries
            if validation_errors_history:
                investigation_prompt = base_prompt + build_validation_error_feedback(
                    validation_errors_history[-1],
                    attempt,
                    schema_hint=last_schema_hint  # BR-HAPI-191: include parameter schema
                )
            else:
                investigation_prompt = base_prompt

            # Log the prompt
            print("\n" + "="*80)
            print(f"🔍 INCIDENT ANALYSIS PROMPT TO LLM (Attempt {attempt + 1}/{MAX_VALIDATION_ATTEMPTS})")
            print("="*80)
            print(investigation_prompt)
            print("="*80 + "\n")

            # Create investigation request
            investigation_request = InvestigateRequest(
                source="kubernaut",
                title=f"Incident analysis for {request_data.get('signal_type')}",
                description=investigation_prompt,
                subject={
                    "type": "incident",
                    "incident_id": incident_id,
                    "signal_type": request_data.get("signal_type")
                },
                context={
                    "incident_id": incident_id,
                    "issue_type": "incident",
                    "attempt": attempt + 1
                },
                source_instance_id="holmesgpt-api"
            )

            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            # AUDIT: LLM REQUEST (BR-AUDIT-005, ADR-032 §1)
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            audit_llm_request(audit_store, incident_id, remediation_id, config, investigation_prompt)

            # Call HolmesGPT SDK
            logger.info({
                "event": "calling_aiagent_sdk",
                "incident_id": incident_id,
                "attempt": attempt + 1,
                "max_attempts": MAX_VALIDATION_ATTEMPTS
            })
            investigation_result = investigate_issues(
                investigate_request=investigation_request,
                dal=dal,
                config=config
            )

            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            # AUDIT: LLM RESPONSE + TOOL CALLS (BR-AUDIT-005, ADR-032 §1)
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            audit_llm_response_and_tools(audit_store, incident_id, remediation_id, investigation_result)

            # Parse and validate investigation result
            result, validation_result = parse_and_validate_investigation_result(
                investigation_result,
                request_data,
                owner_chain=owner_chain,
                data_storage_client=data_storage_client
            )

            # Get workflow_id for audit
            workflow_id = result.get("selected_workflow", {}).get("workflow_id") if result.get("selected_workflow") else None

            # Check if validation passed or no workflow to validate
            is_valid = validation_result is None or validation_result.is_valid
            validation_errors = validation_result.errors if validation_result and not validation_result.is_valid else []

            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            # AUDIT: VALIDATION ATTEMPT (BR-AUDIT-005, DD-HAPI-002 v1.2)
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            audit_store.store_audit(create_validation_attempt_event(
                incident_id=incident_id,
                remediation_id=remediation_id,
                attempt=attempt + 1,
                max_attempts=MAX_VALIDATION_ATTEMPTS,
                is_valid=is_valid,
                errors=validation_errors,
                workflow_id=workflow_id
            ))

            # Build validation attempt record for response (BR-HAPI-197)
            validation_attempts_history.append({
                "attempt": attempt + 1,
                "workflow_id": workflow_id,
                "is_valid": is_valid,
                "errors": validation_errors,
                "timestamp": attempt_timestamp
            })

            if is_valid:
                logger.info({
                    "event": "workflow_validation_passed",
                    "incident_id": incident_id,
                    "attempt": attempt + 1,
                    "has_workflow": result.get("selected_workflow") is not None
                })
                break
            else:
                # Validation failed - log and prepare for retry
                validation_errors_history.append(validation_errors)
                # BR-HAPI-191: Capture schema_hint for next retry's feedback prompt
                if validation_result and validation_result.schema_hint:
                    last_schema_hint = validation_result.schema_hint
                logger.warning({
                    "event": "workflow_validation_retry",
                    "incident_id": incident_id,
                    "attempt": attempt + 1,
                    "max_attempts": MAX_VALIDATION_ATTEMPTS,
                    "errors": validation_errors,
                    "message": "DD-HAPI-002 v1.2: Workflow validation failed, retrying with error feedback"
                })

        # After loop: Check if we exhausted all attempts
        handle_validation_exhaustion(
            result, validation_errors_history, MAX_VALIDATION_ATTEMPTS,
            audit_store, incident_id, remediation_id, workflow_id
        )

        # Add validation history to response (BR-HAPI-197)
        # E2E-HAPI-003: Only override if LLM didn't provide a history (for max_retries_exhausted simulation)
        logger.info({
            "event": "validation_history_decision",
            "incident_id": incident_id,
            "has_key": "validation_attempts_history" in result,
            "llm_provided_count": len(result.get("validation_attempts_history", [])),
            "hapi_loop_count": len(validation_attempts_history)
        })
        if "validation_attempts_history" not in result or not result["validation_attempts_history"]:
            result["validation_attempts_history"] = validation_attempts_history
            logger.info({
                "event": "validation_history_using_hapi_loop",
                "incident_id": incident_id,
                "count": len(validation_attempts_history)
            })
        else:
            logger.info({
                "event": "validation_history_using_llm",
                "incident_id": incident_id,
                "count": len(result["validation_attempts_history"])
            })

        logger.info({
            "event": "incident_analysis_completed",
            "incident_id": incident_id,
            "has_workflow": result.get("selected_workflow") is not None,
            "target_in_owner_chain": result.get("target_in_owner_chain", True),
            "warnings_count": len(result.get("warnings", [])),
            "needs_human_review": result.get("needs_human_review", False),
            "validation_attempts": len(validation_errors_history) + 1 if validation_errors_history else 1
        })
        
        # Record metrics (BR-HAPI-011: Investigation metrics)
        if metrics:
            # Determine status for metrics
            if result.get("needs_human_review", False):
                status = "needs_review"
            else:
                status = "success"
            
            logger.info(f"🔍 METRICS DEBUG (analyze_incident): About to record metrics - status={status}, metrics={metrics}")
            metrics.record_investigation_complete(start_time, status)
            logger.info(f"🔍 METRICS DEBUG (analyze_incident): Metrics recorded successfully")
        else:
            logger.warning(f"🔍 METRICS DEBUG (analyze_incident): metrics is None - NOT recording")

        return result

    except Exception as e:
        logger.error({
            "event": "incident_analysis_failed",
            "incident_id": incident_id,
            "error": str(e)
        }, exc_info=True)
        
        # Record error metrics (BR-HAPI-011)
        if metrics:
            metrics.record_investigation_complete(start_time, "error")
        
        raise

