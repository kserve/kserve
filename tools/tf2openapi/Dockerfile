# Build the manager binary
# Upstream already is on go 1.24, however there is no gotoolset for 1.24 yet.
# TODO move to ubi9/go-toolset:1.24 when available
FROM registry.access.redhat.com/ubi9/go-toolset:1.23 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY tools/  tools/
COPY pkg/    pkg/
COPY go.mod  go.mod
COPY go.sum  go.sum

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o tf2openapi ./tools/tf2openapi/cmd

# Copy tf2openapi into a thin image
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY third_party/ third_party/
COPY --from=builder /go/src/github.com/kserve/kserve/tf2openapi .
ENTRYPOINT ["/tf2openapi"]
