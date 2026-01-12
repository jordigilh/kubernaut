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
    create_llm_request_event,
    create_llm_response_event,
    create_tool_call_event,
)
from src.mock_responses import generate_mock_recovery_response
from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool
from src.validation.workflow_response_validator import WorkflowResponseValidator
from .constants import MinimalDAL
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
        register_workflow_catalog_toolset
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
    config_data = {
        "model": model_name,
        "api_base": os.getenv("LLM_ENDPOINT"),
        "toolsets": toolsets_config,
        "mcp_servers": mcp_servers_config,
    }

    try:
        config = Config(**config_data)

        # BR-HAPI-250: Register workflow catalog toolset programmatically
        # BR-AUDIT-001: Pass remediation_id for audit trail correlation (DD-WORKFLOW-002 v2.2)
        # DD-HAPI-001: Pass custom_labels for auto-append to workflow search
        # DD-WORKFLOW-001 v1.7: Pass detected_labels with source_resource and owner_chain (100% safe)
        config = register_workflow_catalog_toolset(
            config,
            app_config,
            remediation_id=remediation_id,
            custom_labels=custom_labels,
            detected_labels=detected_labels,
            source_resource=source_resource,
            owner_chain=owner_chain
        )

        # Log labels count for debugging
        custom_labels_info = f", custom_labels={len(custom_labels)} subdomains" if custom_labels else ""
        # Count non-None fields from DetectedLabels model (excluding failedDetections meta field)
        detected_labels_count = len([f for f in detected_labels.model_dump(exclude_none=True).keys() if f != "failedDetections"]) if detected_labels else 0
        detected_labels_info = f", detected_labels={detected_labels_count} fields" if detected_labels else ""
        source_info = f", source={source_resource.get('kind')}/{source_resource.get('namespace', 'cluster')}" if source_resource else ""
        owner_info = f", owner_chain={len(owner_chain)} owners" if owner_chain else ""
        logger.info(f"Initialized HolmesGPT SDK config: model={model_name}, toolsets={list(config.toolset_manager.toolsets.keys())}{custom_labels_info}{detected_labels_info}{source_info}{owner_info}")
        return config
    except Exception as e:
        logger.error(f"Failed to initialize HolmesGPT config: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"LLM configuration error: {str(e)}"
        )


async def analyze_recovery(request_data: Dict[str, Any], app_config: Optional[AppConfig] = None) -> Dict[str, Any]:
    """
    Core recovery analysis logic

    Business Requirements: BR-HAPI-001 to 050
    Design Decision: DD-WORKFLOW-002 v2.4 - WorkflowCatalogToolset via SDK

    Uses HolmesGPT SDK for AI-powered recovery analysis.
    Workflow search is handled by WorkflowCatalogToolset registered with the SDK.
    LLM endpoint is configured via environment variables (LLM_ENDPOINT, LLM_MODEL, LLM_PROVIDER).
    """
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
    # Per ADR-032 §1: Audit is MANDATORY for ALL LLM interactions (including mocked)
    from src.audit import get_audit_store, create_llm_request_event, create_llm_response_event
    audit_store = get_audit_store()
    remediation_id = request_data.get("remediation_id", "")

    # BR-HAPI-212: Check mock mode with audit event generation
    from src.mock_responses import is_mock_mode_enabled, generate_mock_recovery_response
    if is_mock_mode_enabled():
        logger.info({
            "event": "mock_mode_active",
            "incident_id": incident_id,
            "message": "Returning deterministic mock response with audit (MOCK_LLM_MODE=true)"
        })

        # BR-AUDIT-005: Generate audit events even for mock responses (E2E testing requirement)
        # AUDIT: LLM REQUEST
        audit_store.store_audit(create_llm_request_event(
            incident_id=incident_id,
            remediation_id=remediation_id,
            model="mock://test-model",
            prompt="MOCK LLM REQUEST (BR-HAPI-212)",
            toolsets_enabled=["mock"],
            mcp_servers=None
        ))

        # Generate mock response
        result = generate_mock_recovery_response(request_data)

        # AUDIT: LLM RESPONSE
        # For recovery, analysis is the recovery actions string representation
        analysis = str(result.get("recovery_actions", []))
        audit_store.store_audit(create_llm_response_event(
            incident_id=incident_id,
            remediation_id=remediation_id,
            has_analysis=True,
            analysis_length=len(analysis),
            analysis_preview=analysis[:500] if analysis else "",
            tool_call_count=0
        ))

        logger.info({
            "event": "mock_mode_audit_complete",
            "incident_id": incident_id,
            "remediation_id": remediation_id,
            "audit_events_generated": 2
        })

        return result

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

    # Use HolmesGPT SDK with enhanced error handling
    # NOTE: Workflow search is handled by WorkflowCatalogToolset registered via
    # register_workflow_catalog_toolset() - the LLM calls the tool during investigation
    # per DD-WORKFLOW-002 v2.4
    try:
        # Create investigation prompt
        # DD-RECOVERY-003: Use recovery-specific prompt for recovery attempts
        # BR-HAPI-211: Sanitize prompt BEFORE sending to LLM to prevent credential leakage
        from src.sanitization import sanitize_for_llm

        is_recovery = request_data.get("is_recovery_attempt", False)
        if is_recovery and request_data.get("previous_execution"):
            investigation_prompt = sanitize_for_llm(_create_recovery_investigation_prompt(request_data))
            logger.info({
                "event": "using_recovery_prompt",
                "incident_id": incident_id,
                "recovery_attempt_number": request_data.get("recovery_attempt_number", 1)
            })
        else:
            investigation_prompt = sanitize_for_llm(_create_investigation_prompt(request_data))

        # Log the prompt being sent to LLM (sanitized version)
        print("\n" + "="*80)
        print("🔍 PROMPT TO LLM PROVIDER (via HolmesGPT SDK) [SANITIZED per BR-HAPI-211]")
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
                "issue_type": "remediation_failure"
            },
            source_instance_id="holmesgpt-api"
        )

        # Create minimal DAL (no Robusta Platform database needed)
        dal = MinimalDAL(cluster_name=request_data.get("context", {}).get("cluster"))

        # Debug: Log investigation details before SDK call
        logger.debug({
            "event": "calling_holmesgpt_sdk",
            "incident_id": incident_id,
            "prompt_length": len(investigation_prompt),
            "toolsets_enabled": config.toolsets if config else None,
            "prompt_preview": investigation_prompt[:300] + "..." if len(investigation_prompt) > 300 else investigation_prompt
        })

        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        # LLM INTERACTION AUDIT (BR-AUDIT-005, ADR-038, DD-AUDIT-002)
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        # AUDIT: LLM REQUEST (BR-AUDIT-005)
        # Per ADR-032 §1: Audit is MANDATORY - NO silent skip allowed
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

        # Get remediation_id from request context (for audit correlation)
        # Note: remediation_id already initialized at function start, but recovery uses context field
        remediation_id_from_context = request_data.get("context", {}).get("remediation_request_id", "")
        if remediation_id_from_context:
            remediation_id = remediation_id_from_context

        # BR-AUDIT-005: audit_store already initialized at function start
        # (Moved before mock check to support audit in mock mode - BR-HAPI-212 + BR-AUDIT-005)

        if audit_store is None:
            logger.error(
                "CRITICAL: audit_store is None - audit is MANDATORY per ADR-032 §1",
                extra={
                    "incident_id": incident_id,
                    "remediation_id": remediation_id,
                    "adr": "ADR-032 §1",
                }
            )
            raise RuntimeError("audit_store is None - audit is MANDATORY per ADR-032 §1")

        # Non-blocking fire-and-forget audit write (ADR-038 pattern)
        audit_store.store_audit(create_llm_request_event(
            incident_id=incident_id,
            remediation_id=remediation_id,
            model=config.model if config else "unknown",
            prompt=investigation_prompt,
            toolsets_enabled=list(config.toolsets.keys()) if config and config.toolsets else [],
            mcp_servers=list(config.mcp_servers.keys()) if config and hasattr(config, 'mcp_servers') and config.mcp_servers else []
        ))

        # Debug log (kept for backwards compatibility)
        logger.info({
            "event": "llm_request",
            "incident_id": incident_id,
            "model": config.model if config else "unknown",
            "prompt_length": len(investigation_prompt),
            "prompt_preview": investigation_prompt[:500] + "..." if len(investigation_prompt) > 500 else investigation_prompt,
            "toolsets_enabled": config.toolsets if config else [],
            "mcp_servers": list(config.mcp_servers.keys()) if config and hasattr(config, 'mcp_servers') and config.mcp_servers else [],
        })

        # Call HolmesGPT SDK
        logger.info("Calling HolmesGPT SDK for recovery analysis")
        investigation_result = investigate_issues(
            investigate_request=investigation_request,
            dal=dal,
            config=config
        )

        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        # DEBUG: SDK RESPONSE INSPECTION (Option A: Understand SDK format)
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        logger.info({
            "event": "sdk_response_debug",
            "incident_id": incident_id,
            "result_type": type(investigation_result).__name__ if investigation_result else "None",
            "has_result": investigation_result is not None,
            "has_analysis": hasattr(investigation_result, 'analysis') if investigation_result else False,
            "analysis_type": type(investigation_result.analysis).__name__ if investigation_result and hasattr(investigation_result, 'analysis') else "None",
            "attributes": dir(investigation_result) if investigation_result else [],
        })
        
        if investigation_result and hasattr(investigation_result, 'analysis'):
            analysis_text = investigation_result.analysis
            logger.info({
                "event": "sdk_analysis_structure",
                "incident_id": incident_id,
                "analysis_length": len(analysis_text) if analysis_text else 0,
                "has_json_codeblock": "```json" in analysis_text if analysis_text else False,
                "has_section_headers": "# selected_workflow" in analysis_text if analysis_text else False,
                "first_200_chars": analysis_text[:200] if analysis_text else "",
                "contains_selected_workflow": "selected_workflow" in analysis_text if analysis_text else False,
                "contains_root_cause": "root_cause" in analysis_text if analysis_text else False,
            })

        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        # AUDIT: LLM RESPONSE (BR-AUDIT-005)
        # Per ADR-032 §1: Audit is MANDATORY - NO silent skip allowed
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        has_analysis = bool(investigation_result and investigation_result.analysis)
        analysis_length = len(investigation_result.analysis) if investigation_result and investigation_result.analysis else 0
        analysis_preview = investigation_result.analysis[:500] + "..." if investigation_result and investigation_result.analysis and len(investigation_result.analysis) > 500 else (investigation_result.analysis if investigation_result and investigation_result.analysis else "")
        tool_call_count = len(investigation_result.tool_calls) if investigation_result and hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls else 0

        if audit_store is None:
            logger.error(
                "CRITICAL: audit_store is None - audit is MANDATORY per ADR-032 §1",
                extra={
                    "incident_id": incident_id,
                    "remediation_id": remediation_id,
                    "adr": "ADR-032 §1",
                }
            )
            raise RuntimeError("audit_store is None - audit is MANDATORY per ADR-032 §1")

        # Non-blocking fire-and-forget audit write (ADR-038 pattern)
        audit_store.store_audit(create_llm_response_event(
            incident_id=incident_id,
            remediation_id=remediation_id,
            has_analysis=has_analysis,
            analysis_length=analysis_length,
            analysis_preview=analysis_preview,
            tool_call_count=tool_call_count
        ))

        # Debug log (kept for backwards compatibility)
        logger.info({
            "event": "llm_response",
            "incident_id": incident_id,
            "has_analysis": has_analysis,
            "analysis_length": analysis_length,
            "analysis_preview": analysis_preview,
            "has_tool_calls": hasattr(investigation_result, 'tool_calls') and bool(investigation_result.tool_calls) if investigation_result else False,
            "tool_call_count": tool_call_count,
        })

        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        # AUDIT: TOOL CALLS (BR-AUDIT-005)
        # Per ADR-032 §1: Audit is MANDATORY - NO silent skip allowed
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        if investigation_result and hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls:
            for idx, tool_call in enumerate(investigation_result.tool_calls):
                tool_name = getattr(tool_call, 'name', 'unknown')
                tool_arguments = getattr(tool_call, 'arguments', {})
                tool_result = getattr(tool_call, 'result', None)

                if audit_store is None:
                    logger.error(
                        "CRITICAL: audit_store is None - audit is MANDATORY per ADR-032 §1",
                        extra={
                            "incident_id": incident_id,
                            "remediation_id": remediation_id,
                            "adr": "ADR-032 §1",
                        }
                    )
                    raise RuntimeError("audit_store is None - audit is MANDATORY per ADR-032 §1")

                # Non-blocking fire-and-forget audit write (ADR-038 pattern)
                audit_store.store_audit(create_tool_call_event(
                    incident_id=incident_id,
                    remediation_id=remediation_id,
                    tool_call_index=idx,
                    tool_name=tool_name,
                    tool_arguments=tool_arguments,
                    tool_result=tool_result
                ))

                # Debug log (kept for backwards compatibility)
                logger.info({
                    "event": "llm_tool_call",
                    "incident_id": incident_id,
                    "tool_call_index": idx,
                    "tool_name": tool_name,
                    "tool_arguments": tool_arguments,
                    "tool_result": tool_result,
                })

        # Log the raw LLM response (for debugging)
        print("\n" + "="*80)
        print("🤖 RAW LLM RESPONSE (from HolmesGPT SDK)")
        print("="*80)
        if investigation_result:
            if investigation_result.analysis:
                print(f"Analysis (full):\n{investigation_result.analysis}")
            else:
                print("No analysis returned")
            # Check if tool_calls attribute exists
            if hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls:
                print(f"\nTool Calls: {len(investigation_result.tool_calls)}")
                for idx, tool_call in enumerate(investigation_result.tool_calls):
                    print(f"  Tool {idx+1}: {getattr(tool_call, 'name', 'unknown')}")
        else:
            print("No result returned from SDK")
        print("="*80 + "\n")
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

        # Validate investigation result
        if not investigation_result or not investigation_result.analysis:
            logger.error({
                "event": "sdk_empty_response",
                "incident_id": incident_id,
                "message": "SDK returned empty analysis"
            })
            raise HTTPException(
                status_code=status.HTTP_502_BAD_GATEWAY,
                detail="LLM provider returned empty response"
            )

        # Parse result into recovery response
        result = _parse_investigation_result(investigation_result, request_data)

        logger.info({
            "event": "recovery_analysis_completed",
            "incident_id": incident_id,
            "strategy_count": len(result.get("strategies", [])),
            "confidence": result.get("analysis_confidence"),
            "analysis_length": len(investigation_result.analysis) if investigation_result.analysis else 0
        })

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


