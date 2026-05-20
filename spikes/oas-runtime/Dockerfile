FROM golang:1.23 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /oas-runtime .

FROM ubuntu:24.04
RUN useradd -m -u 1000 sandbox && \
    apt-get update && apt-get install -y iproute2 ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /oas-runtime /usr/local/bin/oas-runtime
USER sandbox
EXPOSE 8080
