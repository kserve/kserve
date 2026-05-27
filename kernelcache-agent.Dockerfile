# Build the manager binary
FROM golang:1.25 AS deps

# Install required system packages for CGO builds (MCV dependencies)
RUN apt-get update && \
    apt-get install -y \
        libgpgme-dev \
        btrfs-progs \
        libbtrfs-dev \
        libgpgme11-dev \
        libseccomp-dev \
        pkg-config \
        build-essential && \
    apt-get clean

WORKDIR /go/src/github.com/kserve/kserve
# Copy MCV source from build context for local replace directive
COPY --from=mcv . /go/src/github.com/redhat-et/GKM/mcv/
COPY go.mod  go.mod
COPY go.sum  go.sum
# Update replace directive to point to copied MCV source
RUN sed -i 's|replace github.com/redhat-et/GKM/mcv => /home/bmcfall/src/GKM/mcv|replace github.com/redhat-et/GKM/mcv => /go/src/github.com/redhat-et/GKM/mcv|' go.mod
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# ---- Build stage (parallel with license on BuildKit) ----
FROM deps AS builder

ARG CMD=kernelcachenode
ARG GOTAGS=""
COPY cmd/${CMD}/ cmd/${CMD}/
COPY pkg/    pkg/
# Enable CGO for NVML dependency (required by MCV)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux GOFLAGS=-mod=readonly go build -tags "${GOTAGS}" -a -o kernelcachenode-agent ./cmd/${CMD}

# ---- License stage (parallel with build on BuildKit) ----
FROM deps AS license

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/google/go-licenses@v1.6.0

ARG CMD=kernelcachenode
COPY cmd/${CMD}/ cmd/${CMD}/
COPY pkg/    pkg/
COPY LICENSE LICENSE
# Temporarily skip license check for MCV dependencies (FIXME: resolve license issues)
RUN --mount=type=cache,target=/go/pkg/mod \
    mkdir -p /third_party/library && \
    echo "License check skipped for MCV integration" > /third_party/library/README.txt

# Use Ubuntu base image for CGO support (required by MCV/NVML)
FROM public.ecr.aws/docker/library/ubuntu:24.04
# Install runtime libraries for CGO (required by MCV dependencies)
RUN apt-get update && \
    apt-get install -y \
        ca-certificates \
        libgpgme11 \
        libbtrfs0 \
        libseccomp2 \
        libc6 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
# Create MCV config directory with proper permissions for non-root user
RUN mkdir -p /tmp/mcv && chmod 777 /tmp/mcv
COPY --from=license /third_party /third_party
COPY --from=builder /go/src/github.com/kserve/kserve/kernelcachenode-agent /manager
# Run as non-root user
USER 65532:65532
ENTRYPOINT ["/manager"]
