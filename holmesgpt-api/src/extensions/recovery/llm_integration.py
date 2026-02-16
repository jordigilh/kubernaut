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
Recovery Analysis LLM Integration

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
Design Decision: DD-RECOVERY-003 (Recovery LLM Integration)

This module contains the main recovery analysis logic, Holmes SDK integration,
and configuration management.
"""

import asyncio
import os
import logging
from typing import Dict, Any, Optional, List
from fastapi import HTTPException, status

from src.models.config_models import AppConfig
from holmes.config import Config
from holmes.core.models import InvestigateRequest, InvestigationResult
from holmes.core.investigation import investigate_issues
from src.audit import (
    get_audit_store,
    create_validation_attempt_event,
)
from src.validation.workflow_response_validator import WorkflowResponseValidator, ValidationResult
from .constants import MinimalDAL, MAX_VALIDATION_ATTEMPTS
from src.extensions.incident.llm_integration import create_data_storage_client
from src.extensions.incident.prompt_builder import build_validation_error_feedback
from src.extensions.investigation_helpers import (
    audit_llm_request,
    audit_llm_response_and_tools,
    handle_validation_exhaustion,
)
from .prompt_builder import (
    _create_recovery_investigation_prompt,
    _create_investigation_prompt
)
from .result_parser import (
    _parse_investigation_result,
    _parse_recovery_specific_result
)

# Import models for type handling
from src.models.incident_models import DetectedLabels

logger = logging.getLogger(__name__)

def _get_holmes_config(
    app_config: Dict[str, Any] = None,
    remediation_id: Optional[str] = None,
    custom_labels: Optional[Dict[str, List[str]]] = None,
    detected_labels: Optional[DetectedLabels] = None,
    source_resource: Optional[Dict[str, str]] = None,
    owner_chain: Optional[List[Dict[str, str]]] = None
) -> Config:
    """
    Initialize HolmesGPT SDK Config from environment variables and app config

    Args:
        app_config: Application configuration dictionary
        remediation_id: Remediation request ID for audit correlation (DD-WORKFLOW-002 v2.2)
                       MANDATORY per DD-WORKFLOW-002 v2.2. This ID is for CORRELATION/AUDIT ONLY -
                       do NOT use for RCA analysis or workflow matching.
        custom_labels: Custom labels for auto-append to workflow search (DD-HAPI-001)
                      Format: map[string][]string (subdomain → list of values)
                      Example: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
                      Auto-appended to all MCP workflow search calls - invisible to LLM.
        detected_labels: Auto-detected labels for workflow matching (DD-WORKFLOW-001 v1.7)
                        Format: {"gitOpsManaged": true, "gitOpsTool": "argocd", ...}
                        Only included when relationship to RCA resource is PROVEN.
        source_resource: Original signal's resource for DetectedLabels validation
                        Format: {"namespace": "production", "kind": "Pod", "name": "api-xyz"}
                        Compared against LLM's rca_resource.
        owner_chain: K8s ownership chain from SignalProcessing enrichment
                    Format: [{"namespace": "prod", "kind": "ReplicaSet", "name": "..."}, ...]
                    Used for PROVEN relationship validation (100% safe).

    Required environment variables:
    - LLM_MODEL: Full litellm-compatible model identifier (e.g., "provider/model-name")
    - LLM_ENDPOINT: Optional LLM API endpoint

    Note: LLM_MODEL should include the litellm provider prefix if needed
    Examples:
    - "gpt-4" (OpenAI - no prefix needed)
    - "provider_name/model-name" (other providers)
    """
    # Get formatted model name for litellm (supports Ollama, OpenAI, Claude, Vertex AI)
    from src.extensions.llm_config import (
        get_model_config_for_sdk,
        prepare_toolsets_config_for_sdk,
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

    # Get MCP servers configuration from app_config
    # MCP servers are registered as toolsets by the SDK's ToolsetManager
    mcp_servers_config = None
    if app_config and "mcp_servers" in app_config:
        mcp_servers_config = app_config["mcp_servers"]
        logger.info(f"Registering MCP servers: {list(mcp_servers_config.keys())}")

    # Create HolmesGPT SDK Config
    # NOTE: api_key is obtained from OPENAI_API_KEY environment variable via SDK's model registry
    # Do NOT pass api_key to Config() - it's not a valid field and will cause Pydantic validation error
    config_data = {
        "model": model_name,
        "api_base": os.getenv("LLM_ENDPOINT"),
        "toolsets": toolsets_config,
        "mcp_servers": mcp_servers_config,
    }

    try:
        config = Config(**config_data)

        # Log labels count for debugging
        custom_labels_info = f", custom_labels={len(custom_labels)} subdomains" if custom_labels else ""
        # Count non-None fields from DetectedLabels model (excluding failedDetections meta field)
        detected_labels_count = len([f for f in detected_labels.model_dump(exclude_none=True).keys() if f != "failedDetections"]) if detected_labels else 0
        detected_labels_info = f", detected_labels={detected_labels_count} fields" if detected_labels else ""
        source_info = f", source={source_resource.get('kind')}/{source_resource.get('namespace', 'cluster')}" if source_resource else ""
        owner_info = f", owner_chain={len(owner_chain)} owners" if owner_chain else ""
        logger.info(f"Initialized HolmesGPT SDK config: model={model_name}, toolsets={list(config.toolset_manager.toolsets.keys()) if hasattr(config, 'toolset_manager') else 'N/A'}{custom_labels_info}{detected_labels_info}{source_info}{owner_info}")
        return config
    except Exception as e:
        logger.error(f"Failed to initialize HolmesGPT config: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"LLM configuration error: {str(e)}"
        )


async def analyze_recovery(request_data: Dict[str, Any], app_config: Optional[AppConfig] = None, metrics=None) -> Dict[str, Any]:
    """
    Core recovery analysis logic.

    Business Requirements:
    - BR-HAPI-001 to 050 (Recovery analysis)
    - BR-HAPI-011 (Investigation metrics)
    - BR-HAPI-301 (LLM metrics)
    
    Design Decision: DD-WORKFLOW-002 v2.4 - WorkflowCatalogToolset via SDK

    Uses HolmesGPT SDK for AI-powered recovery analysis.
    Workflow search is handled by WorkflowCatalogToolset registered with the SDK.
    LLM endpoint is configured via environment variables (LLM_ENDPOINT, LLM_MODEL, LLM_PROVIDER).
    
    Args:
        request_data: Recovery request data dict
        app_config: Optional application configuration
        metrics: Optional HAMetrics instance (injected by caller, uses global if None)
    """
    import time
    
    # Start timing for BR-HAPI-011 (Investigation metrics)
    start_time = time.time()
    
    incident_id = request_data.get("incident_id")
    is_recovery = request_data.get("is_recovery_attempt", False)

    # Support both legacy and new format (DD-RECOVERY-003)
    failed_action = request_data.get("failed_action", {}) or {}
    failure_context = request_data.get("failure_context", {}) or {}
    previous_execution = request_data.get("previous_execution", {}) or {}

    # Determine action type for logging
    if is_recovery and previous_execution:
        failure = previous_execution.get("failure", {}) or {}
        action_type = f"recovery_from_{failure.get('reason', 'unknown')}"
    else:
        action_type = failed_action.get("type", "unknown")

    logger.info({
        "event": "recovery_analysis_started",
        "incident_id": incident_id,
        "action_type": action_type,
        "is_recovery_attempt": is_recovery
    })

    # BR-AUDIT-005: Initialize audit store BEFORE mock check
    # Per ADR-032 §1: Audit is MANDATORY for ALL LLM interactions
    audit_store = get_audit_store()
    
    # BR-AUDIT-001: Extract remediation_id for audit trail correlation (DD-WORKFLOW-002 v2.2)
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
    custom_labels = enrichment_results.get("customLabels")  # camelCase from K8s

    if custom_labels:
        logger.info({
            "event": "custom_labels_extracted",
            "incident_id": incident_id,
            "subdomains": list(custom_labels.keys()),
            "message": f"DD-HAPI-001: {len(custom_labels)} custom label subdomains will be auto-appended to workflow search"
        })

    # DD-WORKFLOW-001 v1.7: Extract detected_labels for workflow matching (100% safe)
    detected_labels_dict = enrichment_results.get("detectedLabels", {}) or {}
    detected_labels = DetectedLabels(**detected_labels_dict) if detected_labels_dict else None

    # DD-WORKFLOW-001 v1.7: Extract source_resource for DetectedLabels validation
    # This is the original signal's resource - compared against LLM's rca_resource
    source_resource = {
        "namespace": request_data.get("resource_namespace", ""),
        "kind": request_data.get("resource_kind", ""),
        "name": request_data.get("resource_name", "")
    }

    # DD-WORKFLOW-001 v1.7: Extract owner_chain from enrichment_results
    # K8s ownership chain from SignalProcessing (via ownerReferences)
    # Used for PROVEN relationship validation (100% safe)
    owner_chain = enrichment_results.get("ownerChain")

    if detected_labels:
        # Get non-None fields from DetectedLabels model for logging
        label_fields = [f for f, v in detected_labels.model_dump(exclude_none=True).items() if f != "failedDetections"]
        logger.info({
            "event": "detected_labels_extracted",
            "incident_id": incident_id,
            "fields": label_fields,
            "source_resource": f"{source_resource.get('kind')}/{source_resource.get('namespace') or 'cluster'}",
            "owner_chain_length": len(owner_chain) if owner_chain else 0,
            "message": f"DD-WORKFLOW-001 v1.7: {len(label_fields)} detected labels (100% safe validation)"
        })

    config = _get_holmes_config(
        app_config,
        remediation_id=remediation_id,
        custom_labels=custom_labels,
        detected_labels=detected_labels,
        source_resource=source_resource,
        owner_chain=owner_chain
    )

    # DD-HAPI-017: Register three-step workflow discovery toolset
    # (replaces single search_workflow_catalog tool from DD-WORKFLOW-002)
    # NOTE: Registration is done here (not in _get_holmes_config) because
    # context filters come from request_data which is only available in the caller.
    from src.extensions.llm_config import register_workflow_discovery_toolset
    config = register_workflow_discovery_toolset(
        config,
        app_config,
        remediation_id=remediation_id,
        custom_labels=custom_labels,
        detected_labels=detected_labels,
        severity=request_data.get("severity", ""),
        component=request_data.get("resource_kind", ""),
        environment=request_data.get("environment", ""),
        priority=request_data.get("priority", ""),
    )

    # Use HolmesGPT SDK with enhanced error handling
    # NOTE: Workflow discovery is handled by WorkflowDiscoveryToolset registered via
    # register_workflow_discovery_toolset() - LLM calls three-step tools during investigation
    # per DD-HAPI-017
    # Issue #97: root_owner is declared here so it's accessible in the outer scope
    root_owner = {}
    try:
        # BR-HAPI-016: Query remediation history from DataStorage for prompt enrichment
        # Graceful degradation: if DS unavailable or module not yet deployed, context is None
        remediation_history_context = None
        try:
            from src.clients.remediation_history_client import (
                create_remediation_history_api,
                fetch_remediation_history_for_request,
            )
            from src.clients.k8s_client import get_k8s_client, resolve_root_owner

            rh_api = create_remediation_history_api(app_config)

            # Issue #97: Resolve root owner from owner chain (conditional)
            owner_chain_for_history = None
            if isinstance(enrichment_results, dict):
                owner_chain_for_history = enrichment_results.get("ownerChain")
            elif hasattr(enrichment_results, "ownerChain"):
                owner_chain_for_history = enrichment_results.ownerChain

            signal_target = {
                "kind": request_data.get("resource_kind", ""),
                "name": request_data.get("resource_name", ""),
                "namespace": request_data.get("resource_namespace", ""),
            }
            root_owner = resolve_root_owner(owner_chain_for_history, signal_target)

            # Issue #97: Compute spec hash from root owner via K8s API
            current_spec_hash = ""
            if root_owner.get("kind") and root_owner.get("name"):
                try:
                    k8s = get_k8s_client()
                    current_spec_hash = await k8s.compute_spec_hash(
                        kind=root_owner["kind"],
                        name=root_owner["name"],
                        namespace=root_owner.get("namespace", ""),
                    )
                except Exception as hash_err:
                    logger.warning({
                        "event": "spec_hash_computation_failed",
                        "incident_id": incident_id,
                        "error": str(hash_err),
                    })

            # Issue #97: Pass root owner identity for remediation history query
            history_request_data = dict(request_data)
            if root_owner.get("kind") and root_owner.get("name"):
                history_request_data["resource_kind"] = root_owner["kind"]
                history_request_data["resource_name"] = root_owner["name"]
                history_request_data["resource_namespace"] = root_owner.get("namespace", "")

            remediation_history_context = fetch_remediation_history_for_request(
                api=rh_api,
                request_data=history_request_data,
                current_spec_hash=current_spec_hash,
            )
        except (ImportError, Exception) as rh_err:
            logger.warning({"event": "remediation_history_unavailable", "error": str(rh_err)})

        # Build base investigation prompt (before validation loop)
        # DD-RECOVERY-003: Use recovery-specific prompt for recovery attempts
        # BR-HAPI-211: Sanitize prompt BEFORE sending to LLM to prevent credential leakage
        from src.sanitization import sanitize_for_llm

        is_recovery = request_data.get("is_recovery_attempt", False)
        if is_recovery and request_data.get("previous_execution"):
            base_prompt = sanitize_for_llm(_create_recovery_investigation_prompt(
                request_data, remediation_history_context=remediation_history_context
            ))
            logger.info({
                "event": "using_recovery_prompt",
                "incident_id": incident_id,
                "recovery_attempt_number": request_data.get("recovery_attempt_number", 1)
            })
        else:
            base_prompt = sanitize_for_llm(_create_investigation_prompt(
                request_data, remediation_history_context=remediation_history_context
            ))

        # Create minimal DAL (no Robusta Platform database needed)
        dal = MinimalDAL(cluster_name=request_data.get("context", {}).get("cluster"))

        # Get remediation_id from request context (for audit correlation)
        remediation_id_from_context = request_data.get("context", {}).get("remediation_request_id", "")
        if remediation_id_from_context:
            remediation_id = remediation_id_from_context

        # DD-HAPI-002 v1.2: Create Data Storage client for workflow validation
        data_storage_client = create_data_storage_client(app_config)

        # DD-HAPI-017: Create validator once before the loop (avoids re-creation per attempt)
        # request_data uses model_dump(mode="json") so all values are plain strings.
        validator = None
        if data_storage_client:
            validator = WorkflowResponseValidator(
                data_storage_client,
                severity=request_data.get("severity"),
                component=request_data.get("resource_kind"),
                environment=request_data.get("environment"),
                priority=request_data.get("priority"),
            )

        # ========================================
        # LLM SELF-CORRECTION LOOP (DD-HAPI-002 v1.2, BR-HAPI-017-004)
        # With full audit trail (BR-AUDIT-005)
        # ========================================
        validation_errors_history: List[List[str]] = []
        last_schema_hint: Optional[str] = None
        result = None
        workflow_id = None

        for attempt in range(MAX_VALIDATION_ATTEMPTS):
            # Build prompt with error feedback for retries
            if validation_errors_history:
                investigation_prompt = base_prompt + build_validation_error_feedback(
                    validation_errors_history[-1],
                    attempt,
                    schema_hint=last_schema_hint
                )
            else:
                investigation_prompt = base_prompt

            # Log the prompt
            print("\n" + "="*80)
            print(f"🔍 RECOVERY ANALYSIS PROMPT TO LLM (Attempt {attempt + 1}/{MAX_VALIDATION_ATTEMPTS})")
            print("="*80)
            print(investigation_prompt)
            print("="*80 + "\n")

            # Create investigation request
            investigation_request = InvestigateRequest(
                source="kubernaut",
                title=f"Recovery analysis for {failed_action.get('type')} failure",
                description=investigation_prompt,
                subject={
                    "type": "remediation_failure",
                    "incident_id": incident_id,
                    "failed_action": failed_action
                },
                context={
                    "incident_id": incident_id,
                    "issue_type": "remediation_failure",
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
            # DD-AA-HAPI-064: Offload sync Holmes SDK call to thread pool
            # to keep the event loop responsive for session submit/poll requests
            investigation_result = await asyncio.to_thread(
                investigate_issues,
                investigate_request=investigation_request,
                dal=dal,
                config=config,
            )

            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            # AUDIT: LLM RESPONSE + TOOL CALLS (BR-AUDIT-005, ADR-032 §1)
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            audit_llm_response_and_tools(audit_store, incident_id, remediation_id, investigation_result)

            # Validate investigation result exists
            if not investigation_result or not investigation_result.analysis:
                logger.error({
                    "event": "sdk_empty_response",
                    "incident_id": incident_id,
                    "attempt": attempt + 1,
                    "message": "SDK returned empty analysis"
                })
                raise HTTPException(
                    status_code=status.HTTP_502_BAD_GATEWAY,
                    detail="LLM provider returned empty response"
                )

            # Parse result into recovery response
            result = _parse_investigation_result(investigation_result, request_data)

            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            # WORKFLOW VALIDATION (DD-HAPI-002 v1.2, BR-HAPI-017-004)
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            selected_workflow = result.get("selected_workflow")
            if not selected_workflow or not selected_workflow.get("workflow_id"):
                # No workflow selected — nothing to validate, exit loop
                logger.info({
                    "event": "no_workflow_to_validate",
                    "incident_id": incident_id,
                    "attempt": attempt + 1,
                })
                break

            workflow_id = selected_workflow["workflow_id"]

            # Validate using pre-created validator (DD-HAPI-017)
            if validator:
                validation_result = validator.validate(
                    workflow_id=workflow_id,
                    container_image=selected_workflow.get("container_image"),
                    parameters=selected_workflow.get("parameters", {}),
                )
                is_valid = validation_result.is_valid
                validation_errors = validation_result.errors if not is_valid else []
            else:
                # No DS client — skip validation
                is_valid = True
                validation_errors = []
                validation_result = None

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

            if is_valid:
                logger.info({
                    "event": "workflow_validation_passed",
                    "incident_id": incident_id,
                    "attempt": attempt + 1,
                    "workflow_id": workflow_id,
                })
                break
            else:
                # Validation failed — prepare for retry
                validation_errors_history.append(validation_errors)
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

        logger.info({
            "event": "recovery_analysis_completed",
            "incident_id": incident_id,
            "strategy_count": len(result.get("strategies", [])),
            "confidence": result.get("analysis_confidence"),
            "validation_attempts": len(validation_errors_history) + 1 if result else 0,
        })

        # Record metrics (BR-HAPI-011: Investigation metrics)
        if metrics:
            metrics.record_investigation_complete(start_time, "success")

        return result

    except ValueError as e:
        # Configuration or validation errors
        logger.error({
            "event": "sdk_validation_error",
            "incident_id": incident_id,
            "error_type": "ValueError",
            "error": str(e),
            "failed_action": failed_action.get("type")
        })
        
        # Record error metrics (BR-HAPI-011)
        if metrics:
            metrics.record_investigation_complete(start_time, "error")
        
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Configuration error: {str(e)}"
        )

    except (ConnectionError, TimeoutError) as e:
        # Network/LLM provider errors
        logger.error({
            "event": "sdk_connection_error",
            "incident_id": incident_id,
            "error_type": type(e).__name__,
            "error": str(e),
            "provider": os.getenv("LLM_MODEL", "unknown")
        })
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"LLM provider unavailable: {str(e)}"
        )

    except Exception as e:
        # Catch-all for unexpected errors
        logger.error({
            "event": "sdk_analysis_failed",
            "incident_id": incident_id,
            "error_type": type(e).__name__,
            "error": str(e),
            "error_details": {
                "failed_action_type": failed_action.get("type"),
                "cluster": request_data.get("context", {}).get("cluster"),
                "namespace": failed_action.get("namespace")
            }
        }, exc_info=True)
        raise


