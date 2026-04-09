# CrashLoop Config Fix - Execution Image (BR-WE-014)
# Authority: ADR-043, DD-WORKFLOW-017, DD-WORKFLOW-003
#
# Execution-only image for the WE Job executor. Does NOT contain workflow-schema.yaml.
# The schema is delivered separately via a schema-only OCI image (FROM scratch).
# See: test/fixtures/workflows/Dockerfile (shared schema image builder)
#
# The remediation script validates required parameters (NAMESPACE, DEPLOYMENT_NAME)
# and simulates a deployment restart. Parameter validation ensures that regressions
# in the LLM -> KA -> AIAnalysis -> WFE -> Job parameter pipeline are caught.
#
# Build: podman build -t quay.io/kubernaut-cicd/test-workflows/crashloop-config-fix-job:v1.0.0-exec .
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest

# DD-WORKFLOW-003: Remediation script (Validate -> Action)
COPY remediate.sh /scripts/remediate.sh
RUN chmod +x /scripts/remediate.sh

USER 1001

ENTRYPOINT ["/scripts/remediate.sh"]
