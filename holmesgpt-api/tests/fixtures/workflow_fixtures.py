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
Workflow Test Fixtures - DD-API-001 Compliant

Provides reusable test workflow data that can be used across:
- Integration tests
- E2E tests
- Unit tests (for mocking)

Uses OpenAPI generated client for DD-API-001 compliance.
"""

import os
import hashlib
from typing import List, Dict, Any, Optional
from dataclasses import dataclass

# DD-API-001: Use OpenAPI generated client
import sys
# Add holmesgpt-api root to path so src.clients.datastorage can be imported
hapi_root = os.path.join(os.path.dirname(__file__), '..', '..')
sys.path.insert(0, hapi_root)
from datastorage import ApiClient, Configuration
from datastorage.api import WorkflowCatalogAPIApi, WorkflowDiscoveryAPIApi
from datastorage.models import CreateWorkflowFromOCIRequest, RemediationWorkflow, MandatoryLabels


@dataclass
class WorkflowFixture:
    """Test workflow data structure"""
    workflow_name: str
    version: str
    display_name: str
    description: str
    action_type: str  # DD-WORKFLOW-016: FK to action_type_taxonomy
    signal_type: str
    severity: str
    component: str
    environment: str
    priority: str
    risk_tolerance: str
    container_image: str

    @property
    def primary_environment(self) -> str:
        """First environment value (for API calls that need a single string)."""
        return self.environment[0] if isinstance(self.environment, list) else self.environment

    def to_yaml_content(self) -> str:
        """Generate workflow YAML content"""
        return f"""apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: {self.workflow_name}
  version: "{self.version}"
  description: {self.description}
labels:
  signal_type: {self.signal_type}
  severity: {self.severity}
  component: {self.component}
  environment: {self.primary_environment}
  priority: {self.priority}
  risk_tolerance: {self.risk_tolerance}
parameters:
  - name: NAMESPACE
    type: string
    required: true
    description: Target namespace for the operation
  - name: TARGET_NAME
    type: string
    required: true
    description: Target resource name
execution:
  engine: tekton
  bundle: {self.container_image}"""

    def to_oci_request(self) -> CreateWorkflowFromOCIRequest:
        """
        Convert to CreateWorkflowFromOCIRequest (DD-WORKFLOW-017 compliant).

        DD-WORKFLOW-017: Workflow registration accepts only an OCI image pullspec.
        DataStorage pulls the image, extracts workflow-schema.yaml, validates it,
        and populates all catalog fields from the extracted schema.
        """
        return CreateWorkflowFromOCIRequest(
            container_image=self.container_image
        )

    def to_remediation_workflow(self) -> RemediationWorkflow:
        """
        Convert to RemediationWorkflow model for test assertions.

        NOTE: This is no longer used for workflow creation (see to_oci_request()).
        Retained for tests that need to build expected RemediationWorkflow values
        for response assertions.
        """
        content = self.to_yaml_content()
        content_hash = hashlib.sha256(content.encode()).hexdigest()

        # Extract container_digest if present
        container_digest = None
        if "@sha256:" in self.container_image:
            container_digest = self.container_image.split("@")[1]

        # Create MandatoryLabels instance
        labels = MandatoryLabels(
            signal_type=self.signal_type,
            severity=self.severity,
            component=self.component,
            environment=self.environment,
            priority=self.priority
        )

        # Create RemediationWorkflow instance
        # DD-WORKFLOW-016: action_type is now required (FK to action_type_taxonomy)
        return RemediationWorkflow(
            workflow_name=self.workflow_name,
            action_type=self.action_type,
            version=self.version,
            name=self.display_name,
            description=self.description,
            content=content,
            content_hash=content_hash,
            labels=labels,
            execution_engine="tekton",
            status="active",
            container_image=self.container_image,
            container_digest=container_digest
        )


# Test Workflow Fixtures
# MUST match Mock LLM server.py and AIAnalysis test_workflows.go
# DD-WORKFLOW-016: action_type values must exist in action_type_taxonomy (seeded by migration 025)
TEST_WORKFLOWS = [
    WorkflowFixture(
        workflow_name="oomkill-increase-memory-v1",  # Aligned with Mock LLM and AIAnalysis
        version="1.0.0",
        display_name="OOMKill Remediation - Increase Memory Limits",
        description="Increases memory limits for pods experiencing OOMKilled events",
        action_type="AdjustResources",  # DD-WORKFLOW-016: Modify resource requests/limits
        signal_type="OOMKilled",
        severity="critical",
        component="pod",
        environment=["production"],
        priority="P0",
        risk_tolerance="low",
        container_image="ghcr.io/kubernaut/workflows/oomkill-increase-memory:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000001"
    ),
    WorkflowFixture(
        workflow_name="memory-optimize-v1",  # Aligned with Mock LLM and AIAnalysis
        version="1.0.0",
        display_name="OOMKill Remediation - Scale Down Replicas",
        description="Reduces replica count for deployments experiencing OOMKilled",
        action_type="ScaleReplicas",  # DD-WORKFLOW-016: Horizontally scale workload
        signal_type="OOMKilled",
        severity="high",
        component="deployment",
        environment=["staging"],
        priority="P1",
        risk_tolerance="medium",
        container_image="ghcr.io/kubernaut/workflows/oomkill-scale-down:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000002"
    ),
    WorkflowFixture(
        workflow_name="crashloop-config-fix-v1",  # Aligned with Mock LLM and AIAnalysis
        version="1.0.0",
        display_name="CrashLoopBackOff - Fix Configuration",
        description="Identifies and fixes configuration issues causing CrashLoopBackOff",
        action_type="ReconfigureService",  # DD-WORKFLOW-016: Update ConfigMap/Secret values
        signal_type="CrashLoopBackOff",
        severity="high",
        component="pod",
        environment=["production"],
        priority="P1",
        risk_tolerance="low",
        container_image="ghcr.io/kubernaut/workflows/crashloop-fix-config:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000003"
    ),
    WorkflowFixture(
        workflow_name="node-drain-reboot-v1",  # Aligned with Mock LLM and AIAnalysis
        version="1.0.0",
        display_name="NodeNotReady - Drain and Reboot",
        description="Safely drains and reboots nodes in NotReady state",
        action_type="RestartPod",  # DD-WORKFLOW-016: Delete and recreate to recover
        signal_type="NodeNotReady",
        severity="critical",
        component="node",
        environment=["production"],
        priority="P0",
        risk_tolerance="low",
        container_image="ghcr.io/kubernaut/workflows/node-drain-reboot:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000004"
    ),
    WorkflowFixture(
        workflow_name="image-pull-backoff-fix-credentials",
        version="1.0.0",
        display_name="ImagePullBackOff - Fix Registry Credentials",
        description="Fixes ImagePullBackOff errors by updating registry credentials",
        action_type="ReconfigureService",  # DD-WORKFLOW-016: Update credentials = configuration
        signal_type="ImagePullBackOff",
        severity="high",
        component="pod",
        environment=["production"],
        priority="P1",
        risk_tolerance="medium",
        container_image="ghcr.io/kubernaut/workflows/imagepull-fix-creds:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000005"
    ),
]


def _create_authenticated_ds_client(data_storage_url: str) -> ApiClient:
    """
    Create an authenticated Data Storage API client.

    DD-AUTH-014: Injects ServiceAccount token via shared pool manager.
    Callers should use the returned client within a `with` block or
    call `.close()` when done.

    Args:
        data_storage_url: Data Storage service URL

    Returns:
        ApiClient with ServiceAccount token authentication injected
    """
    import sys
    from pathlib import Path
    sys.path.insert(0, str(Path(__file__).parent.parent / "src"))
    from clients.datastorage_pool_manager import get_shared_datastorage_pool_manager

    config = Configuration(host=data_storage_url)
    api_client = ApiClient(config)
    auth_pool = get_shared_datastorage_pool_manager()
    api_client.rest_client.pool_manager = auth_pool
    return api_client


def bootstrap_workflows(data_storage_url: str, workflows: List[WorkflowFixture] = None) -> Dict[str, Any]:
    """
    Bootstrap workflow test data into Data Storage.

    DD-WORKFLOW-017: Uses OCI pullspec-only registration. DataStorage pulls the OCI image,
    extracts workflow-schema.yaml, validates it, and populates all catalog fields.
    DD-AUTH-014: Uses shared pool manager with ServiceAccount token injection.
    DD-TEST-011 v2.0: Captures workflow UUIDs for Mock LLM configuration.

    Args:
        data_storage_url: Data Storage service URL
        workflows: List of workflow fixtures (defaults to TEST_WORKFLOWS)

    Returns:
        Dict with 'created', 'existing', 'failed' counts and workflow_id_map
        {
            "created": ["workflow1", ...],
            "existing": ["workflow2", ...],
            "failed": [...],
            "total": N,
            "workflow_id_map": {"workflow_name:environment": "uuid", ...}
        }
    """
    if workflows is None:
        workflows = TEST_WORKFLOWS

    results = {
        "created": [],
        "existing": [],
        "failed": [],
        "total": len(workflows),
        "workflow_id_map": {}  # DD-TEST-011: Map workflow_name:environment → UUID
    }

    with _create_authenticated_ds_client(data_storage_url) as api_client:
        catalog_api = WorkflowCatalogAPIApi(api_client)
        discovery_api = WorkflowDiscoveryAPIApi(api_client)

        for workflow in workflows:
            try:
                # DD-WORKFLOW-017: Register workflow via OCI pullspec only
                oci_request = workflow.to_oci_request()
                response = catalog_api.create_workflow(
                    create_workflow_from_oci_request=oci_request,
                    _request_timeout=10
                )

                # DD-TEST-011: Capture workflow_id from response
                workflow_id = response.workflow_id if hasattr(response, 'workflow_id') else None
                if workflow_id:
                    key = f"{workflow.workflow_name}:{workflow.primary_environment}"
                    results["workflow_id_map"][key] = workflow_id

                results["created"].append(workflow.workflow_name)

            except Exception as e:
                error_msg = str(e)
                # 409 Conflict means workflow already exists (acceptable)
                if "409" in error_msg or "already exists" in error_msg.lower():
                    results["existing"].append(workflow.workflow_name)

                    # DD-TEST-011: Query for existing workflow UUID via discovery API
                    # DD-WORKFLOW-016: Use list_workflows_by_action_type (replaces removed search_workflows)
                    try:
                        discovery_response = discovery_api.list_workflows_by_action_type(
                            action_type=workflow.action_type,
                            severity=workflow.severity,
                            component=workflow.component,
                            environment=workflow.primary_environment,
                            priority=workflow.priority,
                            _request_timeout=5
                        )
                        # Match by workflow_name in the discovery results
                        for entry in discovery_response.workflows:
                            if entry.workflow_name == workflow.workflow_name:
                                key = f"{workflow.workflow_name}:{workflow.primary_environment}"
                                results["workflow_id_map"][key] = entry.workflow_id
                                break
                    except Exception as query_err:
                        print(f"Warning: Failed to query existing workflow UUID for {workflow.workflow_name}: {query_err}")
                else:
                    results["failed"].append({
                        "workflow": workflow.workflow_name,
                        "error": error_msg
                    })

    return results


def get_test_workflows() -> List[WorkflowFixture]:
    """Get all test workflow fixtures"""
    return TEST_WORKFLOWS


def get_workflow_by_signal_type(signal_type: str) -> List[WorkflowFixture]:
    """Get workflows for specific signal type"""
    return [w for w in TEST_WORKFLOWS if w.signal_type == signal_type]


def get_workflows_by_action_type(action_type: str, workflows: Optional[List[WorkflowFixture]] = None) -> List[WorkflowFixture]:
    """
    Get workflows for a specific action type (DD-WORKFLOW-016).

    Args:
        action_type: Action type from taxonomy (e.g., 'ScaleReplicas', 'RestartPod')
        workflows: Workflow list to filter (defaults to TEST_WORKFLOWS)

    Returns:
        List of WorkflowFixture matching the action_type
    """
    source = workflows if workflows is not None else TEST_WORKFLOWS
    return [w for w in source if w.action_type == action_type]


def get_oomkilled_workflows() -> List[WorkflowFixture]:
    """Get OOMKilled workflows (commonly used in tests)"""
    return get_workflow_by_signal_type("OOMKilled")


def get_crashloop_workflows() -> List[WorkflowFixture]:
    """Get CrashLoopBackOff workflows"""
    return get_workflow_by_signal_type("CrashLoopBackOff")


# ========================================
# DD-WORKFLOW-016: Action Type Taxonomy Constants
# ========================================
# These must match the values seeded by migration 025_action_type_taxonomy.sql

ACTION_TYPE_SCALE_REPLICAS = "ScaleReplicas"
ACTION_TYPE_RESTART_POD = "RestartPod"
ACTION_TYPE_ROLLBACK_DEPLOYMENT = "RollbackDeployment"
ACTION_TYPE_ADJUST_RESOURCES = "AdjustResources"
ACTION_TYPE_RECONFIGURE_SERVICE = "ReconfigureService"

ALL_ACTION_TYPES = [
    ACTION_TYPE_SCALE_REPLICAS,
    ACTION_TYPE_RESTART_POD,
    ACTION_TYPE_ROLLBACK_DEPLOYMENT,
    ACTION_TYPE_ADJUST_RESOURCES,
    ACTION_TYPE_RECONFIGURE_SERVICE,
]


def bootstrap_action_type_taxonomy(data_storage_url: str) -> Dict[str, Any]:
    """
    Verify action type taxonomy is available in Data Storage.

    DD-WORKFLOW-016: Taxonomy is seeded by SQL migration 025_action_type_taxonomy.sql
    (executed by Go infrastructure). This function verifies the taxonomy
    is accessible via the discovery API.

    Args:
        data_storage_url: Data Storage service URL

    Returns:
        Dict with 'available' (bool), 'action_types' (list of str),
        and 'total' (int) from DS response
    """
    result = {
        "available": False,
        "action_types": [],
        "total": 0,
    }

    try:
        with _create_authenticated_ds_client(data_storage_url) as api_client:
            discovery_api = WorkflowDiscoveryAPIApi(api_client)

            # Query all action types with permissive filters
            # (use broad context so we can see all taxonomy entries)
            response = discovery_api.list_available_actions(
                severity="critical",
                component="pod",
                environment="production",
                priority="P0",
                limit=100,
                _request_timeout=10
            )

            action_types = [entry.action_type for entry in response.action_types]
            result["available"] = True
            result["action_types"] = action_types
            result["total"] = response.pagination.total_count

            print(f"✅ DD-WORKFLOW-016: Taxonomy available — {len(action_types)} action types")
            for at in action_types:
                print(f"   - {at}")

    except Exception as e:
        print(f"⚠️  DD-WORKFLOW-016: Taxonomy check failed — {e}")
        print(f"   Ensure Go infrastructure is running with migration 025 applied")

    return result


# ========================================
# PAGINATION FIXTURES
# ========================================
# IT-HAPI-017-001-004: 25+ workflows for pagination testing
# Spread across multiple action types to test both list_available_actions
# and list_workflows_by_action_type pagination.


def _generate_pagination_workflows() -> List[WorkflowFixture]:
    """
    Generate 25+ workflow fixtures for pagination testing.

    DD-WORKFLOW-016: Distributes workflows across all 5 action types
    to enable pagination testing on both list_available_actions and
    list_workflows_by_action_type endpoints.

    Distribution:
      - ScaleReplicas: 6 workflows
      - RestartPod: 5 workflows
      - RollbackDeployment: 5 workflows
      - AdjustResources: 5 workflows
      - ReconfigureService: 5 workflows
      Total: 26 workflows (plus 5 from TEST_WORKFLOWS = 31)
    """
    pagination_fixtures = []

    definitions = [
        # (action_type, prefix, signal_type, component, severity, env, priority, count)
        (ACTION_TYPE_SCALE_REPLICAS, "scale", "OOMKilled", "deployment", "high", "production", "P1", 6),
        (ACTION_TYPE_RESTART_POD, "restart", "CrashLoopBackOff", "pod", "high", "production", "P1", 5),
        (ACTION_TYPE_ROLLBACK_DEPLOYMENT, "rollback", "DeploymentFailed", "deployment", "critical", "production", "P0", 5),
        (ACTION_TYPE_ADJUST_RESOURCES, "adjust", "OOMKilled", "pod", "critical", "staging", "P0", 5),
        (ACTION_TYPE_RECONFIGURE_SERVICE, "reconfig", "CrashLoopBackOff", "pod", "high", "staging", "P1", 5),
    ]

    for action_type, prefix, signal_type, component, severity, env, priority, count in definitions:
        for i in range(1, count + 1):
            workflow_name = f"pagination-{prefix}-{i:02d}-v1"
            # Generate a deterministic hash suffix for container image
            hash_suffix = hashlib.sha256(workflow_name.encode()).hexdigest()[:64]

            pagination_fixtures.append(
                WorkflowFixture(
                    workflow_name=workflow_name,
                    version="1.0.0",
                    display_name=f"Pagination Test - {action_type} #{i:02d}",
                    description=f"Pagination test workflow for {action_type} action type ({i} of {count})",
                    action_type=action_type,
                    signal_type=signal_type,
                    severity=severity,
                    component=component,
                    environment=[env],
                    priority=priority,
                    risk_tolerance="medium",
                    container_image=f"ghcr.io/kubernaut/workflows/{workflow_name}:v1.0.0@sha256:{hash_suffix}",
                )
            )

    return pagination_fixtures


PAGINATION_WORKFLOWS = _generate_pagination_workflows()

# All workflows combined (base + pagination)
ALL_TEST_WORKFLOWS = TEST_WORKFLOWS + PAGINATION_WORKFLOWS

