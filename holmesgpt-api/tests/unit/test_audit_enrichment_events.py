"""
Copyright 2026 Jordi Gil.

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
Unit Tests for Phase 2 Enrichment Audit Events (Issue #533)

Business Requirement: BR-AUDIT-005 v2.0 (SOC2 CC8.1)
Design Decision: ADR-034 (Unified Audit Table Design)

Validates that enrichment audit event factory functions produce
ADR-034 compliant events with correct envelope and payload fields.
"""

from src.audit.events import (
    create_enrichment_completed_event,
    create_enrichment_failed_event,
)


class TestEnrichmentCompletedEvent:
    """Unit tests for create_enrichment_completed_event (UT-HAPI-533-001 to 006, 011, 012)."""

    def test_ut_hapi_533_001_completed_event_adr034_envelope(self):
        """UT-HAPI-533-001: Enrichment completed event has ADR-034 envelope."""
        event = create_enrichment_completed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            root_owner={"kind": "Deployment", "name": "api-server", "namespace": "production"},
            owner_chain_length=3,
            detected_labels={"gitOpsManaged": True, "pdbProtected": False, "hpaEnabled": True},
            failed_detections=None,
            remediation_history_fetched=True,
        )

        assert event.version == "1.0"
        assert event.event_category == "aiagent"
        assert event.event_type == "aiagent.enrichment.completed"
        assert event.event_timestamp is not None
        assert event.correlation_id == "rem-456"
        assert event.event_action == "enrichment_completed"
        assert event.event_outcome == "success"
        assert event.event_data is not None
        assert event.event_data.actual_instance is not None
        assert event.actor_type == "Service"
        assert event.actor_id == "holmesgpt-api"

    def test_ut_hapi_533_002_completed_event_root_owner_accuracy(self):
        """UT-HAPI-533-002: Enrichment completed event captures root_owner correctly."""
        event = create_enrichment_completed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            root_owner={"kind": "Deployment", "name": "api-server", "namespace": "production"},
            owner_chain_length=3,
            detected_labels=None,
            failed_detections=None,
            remediation_history_fetched=False,
        )

        data = event.event_data.actual_instance
        assert data.root_owner_kind == "Deployment"
        assert data.root_owner_name == "api-server"
        assert data.root_owner_namespace == "production"

    def test_ut_hapi_533_003_completed_event_detected_labels_summary(self):
        """UT-HAPI-533-003: Enrichment completed event captures detected_labels summary."""
        labels = {"gitOpsManaged": True, "pdbProtected": False, "hpaEnabled": True}
        event = create_enrichment_completed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            root_owner={"kind": "Deployment", "name": "api", "namespace": "default"},
            owner_chain_length=2,
            detected_labels=labels,
            failed_detections=None,
            remediation_history_fetched=True,
        )

        data = event.event_data.actual_instance
        assert data.detected_labels_summary == labels

    def test_ut_hapi_533_004_completed_event_failed_detections(self):
        """UT-HAPI-533-004: Enrichment completed event captures failed_detections list."""
        event = create_enrichment_completed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            root_owner={"kind": "Deployment", "name": "api", "namespace": "default"},
            owner_chain_length=2,
            detected_labels={"gitOpsManaged": True},
            failed_detections=["networkIsolated", "serviceMesh"],
            remediation_history_fetched=True,
        )

        data = event.event_data.actual_instance
        assert data.failed_detections == ["networkIsolated", "serviceMesh"]

    def test_ut_hapi_533_005_completed_event_remediation_history_fetched(self):
        """UT-HAPI-533-005: Enrichment completed event captures remediation_history_fetched."""
        event = create_enrichment_completed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            root_owner={"kind": "Deployment", "name": "api", "namespace": "default"},
            owner_chain_length=2,
            detected_labels=None,
            failed_detections=None,
            remediation_history_fetched=True,
        )

        data = event.event_data.actual_instance
        assert data.remediation_history_fetched is True

    def test_ut_hapi_533_006_completed_event_owner_chain_length(self):
        """UT-HAPI-533-006: Enrichment completed event captures owner_chain_length."""
        event = create_enrichment_completed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            root_owner={"kind": "Deployment", "name": "api", "namespace": "default"},
            owner_chain_length=3,
            detected_labels=None,
            failed_detections=None,
            remediation_history_fetched=False,
        )

        data = event.event_data.actual_instance
        assert data.owner_chain_length == 3

    def test_ut_hapi_533_011_completed_event_partial_data(self):
        """UT-HAPI-533-011: Enrichment completed event handles partial data gracefully."""
        event = create_enrichment_completed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            root_owner={"kind": "Deployment", "name": "api", "namespace": "default"},
            owner_chain_length=2,
            detected_labels=None,
            failed_detections=None,
            remediation_history_fetched=False,
        )

        assert event.event_outcome == "success"
        data = event.event_data.actual_instance
        assert data.detected_labels_summary is None
        assert data.failed_detections is None
        assert data.remediation_history_fetched is False
        assert data.root_owner_kind == "Deployment"
        assert data.root_owner_name == "api"

    def test_ut_hapi_533_012_completed_event_cluster_scoped_resource(self):
        """UT-HAPI-533-012: Enrichment completed event handles cluster-scoped resource."""
        event = create_enrichment_completed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            root_owner={"kind": "Node", "name": "worker-1"},
            owner_chain_length=1,
            detected_labels=None,
            failed_detections=None,
            remediation_history_fetched=False,
        )

        data = event.event_data.actual_instance
        assert data.root_owner_namespace == ""


class TestEnrichmentFailedEvent:
    """Unit tests for create_enrichment_failed_event (UT-HAPI-533-007 to 010, 013)."""

    def test_ut_hapi_533_007_failed_event_adr034_envelope(self):
        """UT-HAPI-533-007: Enrichment failed event has ADR-034 envelope with outcome=failure."""
        event = create_enrichment_failed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            reason="rca_incomplete",
            detail="resolve_owner_chain failed after 3 retries",
            affected_resource={"kind": "Pod", "name": "api-xyz", "namespace": "prod"},
        )

        assert event.version == "1.0"
        assert event.event_category == "aiagent"
        assert event.event_type == "aiagent.enrichment.failed"
        assert event.event_action == "enrichment_failed"
        assert event.event_outcome == "failure"
        assert event.actor_type == "Service"
        assert event.actor_id == "holmesgpt-api"

    def test_ut_hapi_533_008_failed_event_reason_and_detail(self):
        """UT-HAPI-533-008: Enrichment failed event captures reason and detail."""
        event = create_enrichment_failed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            reason="rca_incomplete",
            detail="resolve_owner_chain failed after 3 retries: ConnectionError",
            affected_resource={"kind": "Pod", "name": "api-xyz", "namespace": "prod"},
        )

        data = event.event_data.actual_instance
        assert data.reason == "rca_incomplete"
        assert data.detail == "resolve_owner_chain failed after 3 retries: ConnectionError"

    def test_ut_hapi_533_009_failed_event_affected_resource(self):
        """UT-HAPI-533-009: Enrichment failed event captures affected_resource."""
        event = create_enrichment_failed_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            reason="rca_incomplete",
            detail="timeout",
            affected_resource={"kind": "Pod", "name": "api-xyz", "namespace": "prod"},
        )

        data = event.event_data.actual_instance
        assert data.affected_resource_kind == "Pod"
        assert data.affected_resource_name == "api-xyz"
        assert data.affected_resource_namespace == "prod"

    def test_ut_hapi_533_010_failed_event_correlation_id(self):
        """UT-HAPI-533-010: Enrichment failed event uses remediation_id as correlation."""
        event = create_enrichment_failed_event(
            incident_id="inc-123",
            remediation_id="rem-abc-123",
            reason="rca_incomplete",
            detail="timeout",
            affected_resource={"kind": "Pod", "name": "api-xyz", "namespace": "prod"},
        )

        assert event.correlation_id == "rem-abc-123"

    def test_ut_hapi_533_013_failed_event_missing_remediation_id(self):
        """UT-HAPI-533-013: Enrichment failed event handles missing remediation_id."""
        event = create_enrichment_failed_event(
            incident_id="inc-123",
            remediation_id=None,
            reason="rca_incomplete",
            detail="timeout",
            affected_resource={"kind": "Pod", "name": "api-xyz", "namespace": "prod"},
        )

        assert event.correlation_id == "unknown"
