# Build the inference-router binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

# Install license tool (cached independently of source changes)
RUN go install github.com/google/go-licenses@v1.6.0
COPY LICENSE LICENSE

ARG CMD=router
COPY cmd/${CMD}/ cmd/${CMD}/
COPY pkg/    pkg/

<<<<<<< HEAD
# Check and generate third-party licenses (fast, fail-fast on violations)
RUN go-licenses check ./cmd/${CMD} ./pkg/... --disallowed_types="forbidden,unknown" && \
    go-licenses save --save_path third_party/library ./cmd/${CMD}

# Build
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=readonly go build -a -o router ./cmd/${CMD}
=======
# Build
USER root
RUN CGO_ENABLED=0  go build -a -o router ./cmd/router

# Generate third-party licenses
COPY LICENSE LICENSE
RUN go install github.com/google/go-licenses@latest
# Forbidden Licenses: https://github.com/google/licenseclassifier/blob/e6a9bb99b5a6f71d5a34336b8245e305f5430f99/license_type.go#L341
RUN /opt/app-root/src/go/bin/go-licenses check ./cmd/... ./pkg/... --disallowed_types="forbidden,unknown"
RUN /opt/app-root/src/go/bin/go-licenses save --save_path third_party/library ./cmd/router
>>>>>>> odh-master

# Copy the inference-router into a thin image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
RUN microdnf install -y --disablerepo=* --enablerepo=ubi-9-baseos-rpms shadow-utils && \
    microdnf clean all && \
    useradd kserve -m -u 1000
RUN microdnf remove -y shadow-utils

COPY --from=builder /go/src/github.com/kserve/kserve/third_party /third_party

WORKDIR /ko-app

COPY --from=builder /go/src/github.com/kserve/kserve/router /ko-app/
USER 1000:1000

ENTRYPOINT ["/ko-app/router"]
