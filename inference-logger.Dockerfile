# Build the inference-logger binary
FROM golang:1.10.3 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kubeflow/kfserving
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o inference-logger ./cmd/inference-logger

# Copy the inference-logger into a thin image
FROM alpine:latest
WORKDIR /
COPY --from=builder /go/src/github.com/kubeflow/kfserving/inference-logger .
ENTRYPOINT ["/inference-logger"]
