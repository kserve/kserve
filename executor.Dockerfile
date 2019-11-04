# Build the executor binary
FROM golang:1.10.3 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kubeflow/kfserving
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/
# Build
RUN if [ "$(uname -m)" = "aarch64" ]; then \
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -o executor ./cmd/executor; \
    else \
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o executor ./cmd/executor; \
    fi

# Copy the executor into a thin image
FROM alpine:latest
WORKDIR /
COPY --from=builder /go/src/github.com/kubeflow/kfserving/executor .
ENTRYPOINT ["/executor"]
