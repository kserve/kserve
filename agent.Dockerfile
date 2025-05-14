# Build the inference-agent binary
# Upstream already is on go 1.24, however there is no gotoolset for 1.24 yet.
# TODO move to ubi9/go-toolset:1.24 when available
FROM registry.access.redhat.com/ubi9/go-toolset:1.23 AS builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

COPY cmd/    cmd/
COPY pkg/    pkg/

# Build
USER root
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=mod go build -a -o agent ./cmd/agent

# Copy the inference-agent into a thin image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

RUN microdnf install -y --disablerepo=* --enablerepo=ubi-9-baseos-rpms shadow-utils && \
    microdnf clean all && \
    useradd kserve -m -u 1000
RUN microdnf remove -y shadow-utils
COPY third_party/ third_party/
WORKDIR /ko-app
COPY --from=builder /go/src/github.com/kserve/kserve/agent /ko-app/
USER 1000:1000

ENTRYPOINT ["/ko-app/agent"]
