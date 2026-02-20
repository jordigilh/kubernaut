# OOMKill Increase Memory - Execution Image (BR-WE-014)
# Authority: ADR-043, DD-WORKFLOW-017, DD-WORKFLOW-003
#
# Execution-only image for the WE Job executor. Does NOT contain workflow-schema.yaml.
# The schema is delivered separately via a schema-only OCI image (FROM scratch).
# See: test/fixtures/workflows/Dockerfile (shared schema image builder)
#
# The remediation script follows the Validate -> Action -> Verify pattern
# from DD-WORKFLOW-003. It uses kubectl to patch the target resource's
# memory limits.
#
# Build: podman build -t quay.io/kubernaut-cicd/test-workflows/oomkill-increase-memory-job:v1.0.0-exec .
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install kubectl for Kubernetes API access
ARG TARGETARCH
ARG KUBECTL_VERSION=v1.31.4
RUN curl -fsSL -o /usr/local/bin/kubectl \
    "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x /usr/local/bin/kubectl

# DD-WORKFLOW-003: Remediation script (Validate -> Action -> Verify)
COPY remediate.sh /scripts/remediate.sh
RUN chmod +x /scripts/remediate.sh

USER 1001

ENTRYPOINT ["/scripts/remediate.sh"]
