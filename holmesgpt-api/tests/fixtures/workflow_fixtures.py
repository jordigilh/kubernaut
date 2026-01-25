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
from typing import List, Dict, Any
from dataclasses import dataclass

# DD-API-001: Use OpenAPI generated client
import sys
# Add holmesgpt-api root to path so src.clients.datastorage can be imported
hapi_root = os.path.join(os.path.dirname(__file__), '..', '..')
sys.path.insert(0, hapi_root)
from datastorage import ApiClient, Configuration
from datastorage.api import WorkflowCatalogAPIApi
from datastorage.models import RemediationWorkflow, MandatoryLabels


@dataclass
class WorkflowFixture:
    """Test workflow data structure"""
    workflow_name: str
    version: str
    display_name: str
    description: str
    signal_type: str
    severity: str
    component: str
    environment: str
    priority: str
    risk_tolerance: str
    container_image: str

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
  environment: {self.environment}
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

    def to_remediation_workflow(self) -> RemediationWorkflow:
        """Convert to RemediationWorkflow model (DD-API-001 compliant)"""
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
        return RemediationWorkflow(
            workflow_name=self.workflow_name,
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
TEST_WORKFLOWS = [
    WorkflowFixture(
        workflow_name="oomkill-increase-memory-limits",
        version="1.0.0",
        display_name="OOMKill Remediation - Increase Memory Limits",
        description="Increases memory limits for pods experiencing OOMKilled events",
        signal_type="OOMKilled",
        severity="critical",
        component="pod",
        environment="production",
        priority="P0",
        risk_tolerance="low",
        container_image="ghcr.io/kubernaut/workflows/oomkill-increase-memory:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000001"
    ),
    WorkflowFixture(
        workflow_name="oomkill-scale-down-replicas",
        version="1.0.0",
        display_name="OOMKill Remediation - Scale Down Replicas",
        description="Reduces replica count for deployments experiencing OOMKilled",
        signal_type="OOMKilled",
        severity="high",
        component="deployment",
        environment="staging",
        priority="P1",
        risk_tolerance="medium",
        container_image="ghcr.io/kubernaut/workflows/oomkill-scale-down:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000002"
    ),
    WorkflowFixture(
        workflow_name="crashloop-fix-configuration",
        version="1.0.0",
        display_name="CrashLoopBackOff - Fix Configuration",
        description="Identifies and fixes configuration issues causing CrashLoopBackOff",
        signal_type="CrashLoopBackOff",
        severity="high",
        component="pod",
        environment="production",
        priority="P1",
        risk_tolerance="low",
        container_image="ghcr.io/kubernaut/workflows/crashloop-fix-config:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000003"
    ),
    WorkflowFixture(
        workflow_name="node-not-ready-drain-and-reboot",
        version="1.0.0",
        display_name="NodeNotReady - Drain and Reboot",
        description="Safely drains and reboots nodes in NotReady state",
        signal_type="NodeNotReady",
        severity="critical",
        component="node",
        environment="production",
        priority="P0",
        risk_tolerance="low",
        container_image="ghcr.io/kubernaut/workflows/node-drain-reboot:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000004"
    ),
    WorkflowFixture(
        workflow_name="image-pull-backoff-fix-credentials",
        version="1.0.0",
        display_name="ImagePullBackOff - Fix Registry Credentials",
        description="Fixes ImagePullBackOff errors by updating registry credentials",
        signal_type="ImagePullBackOff",
        severity="high",
        component="pod",
        environment="production",
        priority="P1",
        risk_tolerance="medium",
        container_image="ghcr.io/kubernaut/workflows/imagepull-fix-creds:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000005"
    ),
]


def bootstrap_workflows(data_storage_url: str, workflows: List[WorkflowFixture] = None) -> Dict[str, Any]:
    """
    Bootstrap workflow test data into Data Storage.

    DD-API-001 COMPLIANCE: Uses OpenAPI generated client.

    Args:
        data_storage_url: Data Storage service URL
        workflows: List of workflow fixtures (defaults to TEST_WORKFLOWS)

    Returns:
        Dict with 'created', 'existing', 'failed' counts and workflow IDs
    """
    if workflows is None:
        workflows = TEST_WORKFLOWS

    config = Configuration(host=data_storage_url)
    results = {
        "created": [],
        "existing": [],
        "failed": [],
        "total": len(workflows)
    }

    with ApiClient(config) as api_client:
        api = WorkflowCatalogAPIApi(api_client)

        for workflow in workflows:
            try:
                # DD-API-001: Use OpenAPI client to create workflow
                remediation_workflow = workflow.to_remediation_workflow()
                response = api.create_workflow(
                    remediation_workflow=remediation_workflow,
                    _request_timeout=10
                )
                results["created"].append(workflow.workflow_name)

            except Exception as e:
                error_msg = str(e)
                # 409 Conflict means workflow already exists (acceptable)
                if "409" in error_msg or "already exists" in error_msg.lower():
                    results["existing"].append(workflow.workflow_name)
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


def get_oomkilled_workflows() -> List[WorkflowFixture]:
    """Get OOMKilled workflows (commonly used in tests)"""
    return get_workflow_by_signal_type("OOMKilled")


def get_crashloop_workflows() -> List[WorkflowFixture]:
    """Get CrashLoopBackOff workflows"""
    return get_workflow_by_signal_type("CrashLoopBackOff")

