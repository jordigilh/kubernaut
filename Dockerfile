FROM golang:1.25 AS builder

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o apifrontend ./cmd/apifrontend/

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/apifrontend .
USER 65532:65532

ENTRYPOINT ["/apifrontend"]
