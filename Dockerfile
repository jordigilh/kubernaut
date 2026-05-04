FROM golang:1.25.6 AS builder

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -a -o apifrontend ./cmd/apifrontend/

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/apifrontend .
USER 65532:65532

# Serves plaintext HTTP on port 8443; TLS is terminated by the ingress/mesh.
EXPOSE 8443
ENTRYPOINT ["/apifrontend"]
