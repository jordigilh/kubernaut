# CrashLoop Config Fix - Execution Image (BR-WE-014)
# Authority: ADR-043, DD-WORKFLOW-017, DD-WORKFLOW-003
# Issue: #1542 (real, verifiable fix — not a no-op simulation)
#
# Execution-only image for the WE Job executor. Does NOT contain workflow-schema.yaml.
# The schema is delivered separately via a schema-only OCI image (FROM scratch).
# See: test/fixtures/workflows/Dockerfile (shared schema image builder)
#
# The remediation script validates required parameters (NAMESPACE, DEPLOYMENT_NAME,
# CONFIGMAP_NAME, CONFIGMAP_KEY, CONFIGMAP_VALUE), patches the offending ConfigMap
# key, and restarts the deployment via kubectl. Parameter validation ensures that
# regressions in the LLM -> KA -> AIAnalysis -> WFE -> Job parameter pipeline are caught.
#
# Build: podman build -t quay.io/kubernaut-cicd/test-workflows/crashloop-config-fix-job:v1.0.0-exec .
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest@sha256:b217fa65d8c21058887b18f005f587e47a17dd1281a5196ac88d01724a273dbd

# Install kubectl for Kubernetes API access
ARG TARGETARCH
ARG KUBECTL_VERSION=v1.31.14
RUN curl -fsSL -o /usr/local/bin/kubectl \
    "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x /usr/local/bin/kubectl

# DD-WORKFLOW-003: Remediation script (Validate -> Action -> Verify)
COPY remediate.sh /scripts/remediate.sh
RUN chmod +x /scripts/remediate.sh

USER 1001

ENTRYPOINT ["/scripts/remediate.sh"]
