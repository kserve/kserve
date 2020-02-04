# Build the manager binary
FROM golang:1.10.3 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kubeflow/kfserving
COPY cmd/    cmd/
COPY vendor/ vendor/
COPY pkg/    pkg/

# Build
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
        CGO_ENABLED=0 GOOS=linux GOARCH=ppc64le go build -a -o manager ./cmd/manager; \
    elif [ "$(uname -m)" = "aarch64" ]; then \
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -o manager ./cmd/manager; \
    else \
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager ./cmd/manager; \
    fi

# Copy the controller-manager into a thin image
FROM ubuntu:latest
WORKDIR /
RUN mkdir -p third_party/library
COPY third_party/library/ third_party/library/
COPY --from=builder /go/src/github.com/kubeflow/kfserving/manager .
ENTRYPOINT ["/manager"]
