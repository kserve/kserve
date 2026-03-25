# Build the inference-router binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 as builder
# distro: UBI go-toolset does not add GOPATH/bin to PATH
ENV PATH="$PATH:/opt/app-root/src/go/bin"

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

USER root

# Check and generate third-party licenses (fast, fail-fast on violations)
RUN /opt/app-root/src/go/bin/go-licenses check ./cmd/${CMD} ./pkg/... --disallowed_types="forbidden,unknown" && \
    /opt/app-root/src/go/bin/go-licenses save --save_path third_party/library ./cmd/${CMD}

# Build
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=readonly go build -a -o router ./cmd/${CMD}

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
