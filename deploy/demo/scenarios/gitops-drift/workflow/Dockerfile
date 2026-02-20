# GitOps Revert - Runnable Job Container (BR-WE-014)
# Authority: ADR-043, DD-WORKFLOW-017, DD-WORKFLOW-003
# Scenario: #125 -- GitOps drift remediation
#
# Dual purpose:
#   1. DataStorage OCI extractor reads /workflow-schema.yaml for catalog registration
#   2. WorkflowExecution Job executor runs this image as a K8s Job container
#
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

ARG TARGETARCH
ARG KUBECTL_VERSION=v1.31.4

RUN microdnf install -y git && microdnf clean all

RUN curl -fsSL -o /usr/local/bin/kubectl \
    "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x /usr/local/bin/kubectl

COPY workflow-schema.yaml /workflow-schema.yaml

COPY remediate.sh /scripts/remediate.sh
RUN chmod +x /scripts/remediate.sh

USER 1001

ENTRYPOINT ["/scripts/remediate.sh"]
