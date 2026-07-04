# AIAnalysis Controller - Runtime-Only Dockerfile (no QEMU)
#
# Prerequisites: make cross-build-aianalysis IMAGE_ARCH=arm64
# Usage: make image-runtime-aianalysis IMAGE_ARCH=arm64

ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-minimal:latest@sha256:b217fa65d8c21058887b18f005f587e47a17dd1281a5196ac88d01724a273dbd
ARG BINARY=bin/aianalysis-controller-arm64

# SECURITY: BASE_IMAGE above is pinned by digest. Dependabot (docker
# ecosystem, .github/dependabot.yml) re-resolves this digest weekly.
FROM --platform=linux/amd64 ${BASE_IMAGE} AS certs
RUN microdnf install -y ca-certificates tzdata && microdnf clean all

FROM scratch
COPY --from=certs /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem /etc/ssl/certs/ca-certificates.crt
COPY --from=certs /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=certs /etc/passwd /etc/passwd
ARG BINARY
COPY ${BINARY} /aianalysis-controller
USER 65534
EXPOSE 8080 9090
ENTRYPOINT ["/aianalysis-controller"]
CMD []

ARG APP_VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-aianalysis-controller" \
	org.opencontainers.image.vendor="Kubernaut"
