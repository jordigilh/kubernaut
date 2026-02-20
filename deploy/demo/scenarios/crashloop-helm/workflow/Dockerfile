# Helm Rollback - Runnable Job Container (BR-WE-014)
# Scenario: #135 -- Helm-managed CrashLoopBackOff -> helm rollback
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

ARG TARGETARCH
ARG KUBECTL_VERSION=v1.31.4
ARG HELM_VERSION=v3.17.1

RUN microdnf install -y jq && microdnf clean all

RUN curl -fsSL -o /usr/local/bin/kubectl \
    "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x /usr/local/bin/kubectl

RUN curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" | \
    tar xz -C /usr/local/bin --strip-components=1 "linux-${TARGETARCH}/helm"

COPY workflow-schema.yaml /workflow-schema.yaml

COPY remediate.sh /scripts/remediate.sh
RUN chmod +x /scripts/remediate.sh

USER 1001

ENTRYPOINT ["/scripts/remediate.sh"]
