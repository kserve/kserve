# Build the manager binary
FROM golang:1.10.3 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kubeflow/kfserving
COPY vendor/ vendor/
COPY tools/  tools/
COPY pkg/    pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o tf2openapi ./tools/tf2openapi/cmd

# Copy tf2openapi into a thin image
FROM ubuntu:latest
WORKDIR /
COPY --from=builder /go/src/github.com/kubeflow/kfserving/tf2openapi .
ENTRYPOINT ["/tf2openapi"]
