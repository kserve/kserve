# Build the mcv binary
FROM golang:1.25-bookworm AS deps

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgpgme-dev \
    libbtrfs-dev \
    build-essential \
    pkg-config \
    libassuan-dev \
    libgpg-error-dev \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /go/src/github.com/kserve/mcv
COPY go.mod  go.mod
COPY go.sum  go.sum
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# ---- Build stage (parallel with license on BuildKit) ----
FROM deps AS builder

ARG CMD=mcv
ARG GOTAGS=""
COPY cmd/   cmd/
COPY pkg/   pkg/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux GOFLAGS=-mod=readonly go build -a -tags "${GOTAGS}" -o ${CMD} ./cmd/


# ---- License stage (parallel with build on BuildKit) ----
FROM deps AS license

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/google/go-licenses@v1.6.0

COPY cmd/   cmd/
COPY pkg/   pkg/
COPY LICENSE LICENSE
RUN --mount=type=cache,target=/go/pkg/mod \
    go-licenses save --save_path /third_party/library ./cmd/

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgpgme11 \
    libbtrfs0 \
    ca-certificates \
    pciutils \
    hwdata \
    buildah \
    rsync \
    wget \
    fuse-overlayfs fuse3 \
 && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /etc/containers && \
 printf '[storage]\ndriver="overlay"\nrunroot="/run/containers/storage"\ngraphroot="/var/lib/containers/storage"\n[storage.options]\nmount_program="/usr/bin/fuse-overlayfs"\n' \
   > /etc/containers/storage.conf

# Install ROCm apt repo
ARG ROCM_VERSION=7.0.1
ARG AMDGPU_VERSION=7.0.1.70001
ARG OPT_ROCM_VERSION=7.0.1

# Install ROCm apt repo
RUN wget https://repo.radeon.com/amdgpu-install/${ROCM_VERSION}/ubuntu/jammy/amdgpu-install_${AMDGPU_VERSION}-1_all.deb
RUN apt install -y ./*.deb
RUN apt update &&  DEBIAN_FRONTEND=noninteractive apt install -y amd-smi-lib rocm-smi-lib
RUN apt-get clean && rm -rf /var/lib/apt/lists/* && rm -rf ./*.deb
RUN ln -s /opt/rocm-${OPT_ROCM_VERSION}/bin/amd-smi /usr/bin/amd-smi
RUN ln -s /opt/rocm-${OPT_ROCM_VERSION}/bin/rocm-smi /usr/bin/rocm-smi

COPY --from=builder /go/src/github.com/kserve/mcv/mcv /mcv

ENTRYPOINT ["/mcv"]
