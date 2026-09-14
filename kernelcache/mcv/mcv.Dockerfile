# Build the mcv binary
FROM golang:1.26.8-bookworm AS deps

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgpgme-dev \
    libbtrfs-dev \
    build-essential \
    pkg-config \
    libassuan-dev \
    libgpg-error-dev \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /go/src/github.com/kserve/kernelcache/mcv
COPY kernelcache/mcv/go.mod  go.mod
COPY kernelcache/mcv/go.sum  go.sum
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# ---- Build stage (parallel with license on BuildKit) ----
FROM deps AS builder

ARG CMD=mcv
ARG GOTAGS=""
COPY kernelcache/mcv/cmd/   cmd/
COPY kernelcache/mcv/pkg/   pkg/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux GOFLAGS=-mod=readonly go build -a -tags "${GOTAGS}" -o ${CMD} ./cmd/


# ---- License stage (parallel with build on BuildKit) ----
FROM deps AS license

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/google/go-licenses@v1.6.0

COPY kernelcache/mcv/cmd/   cmd/
COPY kernelcache/mcv/pkg/   pkg/
COPY LICENSE LICENSE
RUN --mount=type=cache,target=/go/pkg/mod \
    go-licenses save --save_path /third_party/library ./cmd/


# ============================================================================
# MINIMAL TARGET: For cache creation/extraction with --no-gpu
# No CUDA/ROCm libraries - uses cache metadata only
# Build: docker build --target mcv-minimal -t mcv:minimal -f mcv.Dockerfile .
# Usage: mcv --create --image foo --dir /cache --no-gpu
#        mcv --extract --image foo --no-gpu
# ============================================================================
FROM debian:bookworm-slim AS mcv-minimal

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgpgme11 \
    libbtrfs0 \
    libffi8 \
    libc6 \
    ca-certificates \
    buildah \
    netavark aardvark-dns \
    hwdata \
    python3 \
 && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /etc/containers && \
 printf '[storage]\ndriver="vfs"\nrunroot="/home/appuser/.local/share/containers/runroot"\ngraphroot="/home/appuser/.local/share/containers/storage"\n' \
   > /etc/containers/storage.conf

COPY --from=builder /go/src/github.com/kserve/kernelcache/mcv/mcv /mcv
COPY --from=license /third_party/library /third_party/library
COPY kernelcache/mcv/entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

RUN groupadd -g 1000 appgroup && \
    useradd -u 1000 -g 1000 -m -s /bin/bash appuser
RUN test "$(id -u appuser)" = "1000"
RUN chown appuser:1000 /mcv /entrypoint.sh
RUN mkdir -p /home/appuser/.local/share/containers/storage \
             /home/appuser/.local/share/containers/runroot \
             /home/appuser/.config/containers && \
    chown -R appuser:1000 /home/appuser/.local /home/appuser/.config
WORKDIR /app
RUN chown -R appuser:1000 /app
USER appuser

LABEL description="MCV minimal - cache creation/extraction without GPU libraries (use --no-gpu flag)"
LABEL variant="minimal"

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/mcv"]


# ============================================================================
# AMD TARGET: For cache validation with AMD GPU hardware
# Includes ROCm libraries for GPU detection and preflight checks
# Build: docker build --target mcv-rocm -t mcv:rocm -f kernelcache/mcv/mcv.Dockerfile .
# Usage: mcv --check-compat --image foo
#        mcv --extract --image foo  (with preflight check)
# ============================================================================
FROM mcv-minimal AS mcv-rocm
USER root

ARG TARGETARCH
# ROCm only publishes amd64 packages; fail fast on other architectures.
RUN [ "$TARGETARCH" = "amd64" ] || \
    { echo "ERROR: mcv-amd requires amd64 - ROCm does not support ${TARGETARCH}"; exit 1; }

ARG ROCM_VERSION=7.0.1
ARG AMDGPU_VERSION=7.0.1.70001
ARG OPT_ROCM_VERSION=7.0.1
# SHA-256 of amdgpu-install .deb from repo.radeon.com — update when bumping AMDGPU_VERSION
ARG AMDGPU_INSTALLER_SHA256=f4cec24612039c03271e6ab494bc1e18cb5647d59188755aa8e31b6d74bb06df

RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    pciutils \
    python3 \
 && rm -rf /var/lib/apt/lists/*

RUN wget -O /tmp/amdgpu-install.deb \
        https://repo.radeon.com/amdgpu-install/${ROCM_VERSION}/ubuntu/jammy/amdgpu-install_${AMDGPU_VERSION}-1_all.deb && \
    echo "${AMDGPU_INSTALLER_SHA256}  /tmp/amdgpu-install.deb" | sha256sum -c - && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y /tmp/amdgpu-install.deb && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        amd-smi-lib \
        rocm-smi-lib \
        libdrm2 && \
    ln -s /opt/rocm-${OPT_ROCM_VERSION}/bin/amd-smi /usr/bin/amd-smi && \
    ln -s /opt/rocm-${OPT_ROCM_VERSION}/bin/rocm-smi /usr/bin/rocm-smi && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/amdgpu-install.deb

USER appuser

LABEL description="MCV ROCm - includes ROCm libraries for GPU detection and validation"
LABEL variant="rocm"


# ============================================================================
# GAUDI TARGET: For Intel Gaudi GPU validation with hl-smi
# Includes Habana Labs tools for Gaudi device detection
# Build: docker build --target mcv-gaudi -t mcv:gaudi -f kernelcache/mcv/mcv.Dockerfile .
# Usage: mcv --check-compat --image foo (on Intel Gaudi systems)
#        mcv --extract --image foo  (with Gaudi GPU preflight check)
# ============================================================================
FROM public.ecr.aws/docker/library/ubuntu:24.04 AS mcv-gaudi

ARG TARGETARCH

# Gaudi only publishes amd64 packages; fail fast on other architectures.
RUN [ "$TARGETARCH" = "amd64" ] || \
    { echo "ERROR: mcv-gaudi requires amd64 - Habana does not support ${TARGETARCH}"; exit 1; }

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgpgme11t64 \
    libbtrfs0 \
    libffi8 \
    libc6 \
    ca-certificates \
    buildah \
    netavark aardvark-dns \
    hwdata \
    wget \
    gnupg2 \
    pciutils \
    python3 \
 && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /etc/containers && \
 printf '[storage]\ndriver="vfs"\nrunroot="/home/appuser/.local/share/containers/runroot"\ngraphroot="/home/appuser/.local/share/containers/storage"\n' \
   > /etc/containers/storage.conf

# Install hl-smi via Habana apt repo (pattern from HabanaAI/Setup_and_Install).
# HABANA_SIGNING_FP must match the canonical fingerprint published in Intel/Habana
# documentation (https://docs.habana.ai) — update it whenever the signing key rotates.
ARG HABANA_SIGNING_FP="6D4D7C0F52A263F383D782791E676CE836A2DE65"
RUN wget -q -O /tmp/habana-key.asc https://vault.habana.ai/artifactory/api/gpg/key/public && \
    gpg --dearmor < /tmp/habana-key.asc > /tmp/habana-key.gpg && \
    actual=$(gpg --no-default-keyring --keyring /tmp/habana-key.gpg --fingerprint 2>/dev/null \
             | awk '/^      /{gsub(/ /,"",$0); print}' | head -1) && \
    expected=$(printf '%s' "${HABANA_SIGNING_FP}" | tr -d ' :') && \
    [ "$actual" = "$expected" ] || { echo "Habana GPG key fingerprint mismatch: got $actual expected $expected"; exit 1; } && \
    mv /tmp/habana-key.gpg /usr/share/keyrings/habana-artifactory.gpg && \
    rm /tmp/habana-key.asc && \
    chmod 644 /usr/share/keyrings/habana-artifactory.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/habana-artifactory.gpg] https://vault.habana.ai/artifactory/debian noble main" \
        > /etc/apt/sources.list.d/habana.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends habanalabs-firmware-tools && \
    rm -f /etc/apt/sources.list.d/habana.list && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /go/src/github.com/kserve/kernelcache/mcv/mcv /mcv
COPY --from=license /third_party/library /third_party/library
COPY kernelcache/mcv/entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

# Drop any pre-existing ubuntu user/group that claims UID/GID 1000 (present in
# ubuntu:24.04 base images). Without this, useradd -u 1000 fails.
RUN userdel -r ubuntu 2>/dev/null; groupdel ubuntu 2>/dev/null; \
    groupadd -g 1000 appgroup && \
    useradd -u 1000 -g 1000 -m -s /bin/bash appuser
RUN test "$(id -u appuser)" = "1000"
RUN chown appuser:1000 /mcv /entrypoint.sh
RUN mkdir -p /home/appuser/.local/share/containers/storage \
             /home/appuser/.local/share/containers/runroot \
             /home/appuser/.config/containers && \
    chown -R appuser:1000 /home/appuser/.local /home/appuser/.config
WORKDIR /app
RUN chown -R appuser:1000 /app
USER appuser

LABEL description="MCV Gaudi - includes Habana Labs tools for Intel Gaudi GPU detection and validation"
LABEL variant="gaudi"

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/mcv"]

# ============================================================================
# NVIDIA TARGET: For NVIDIA GPU validation with CUDA/NVML support
# Includes CUDA runtime and NVML libraries for GPU detection
# Build: docker build --target mcv-cuda -t mcv:cuda -f kernelcache/mcv/mcv.Dockerfile .
# Usage: mcv --check-compat --image foo (on NVIDIA GPU systems)
#        mcv --extract --image foo  (with NVIDIA GPU preflight check)
# ============================================================================
FROM nvcr.io/nvidia/cuda:12.6.3-base-ubuntu24.04 AS mcv-cuda

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgpgme11t64 \
    libbtrfs0 \
    ca-certificates \
    buildah \
    netavark aardvark-dns \
    hwdata \
    pciutils \
    python3 \
 && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /etc/containers && \
 printf '[storage]\ndriver="vfs"\nrunroot="/home/appuser/.local/share/containers/runroot"\ngraphroot="/home/appuser/.local/share/containers/storage"\n' \
   > /etc/containers/storage.conf

COPY --from=builder /go/src/github.com/kserve/kernelcache/mcv/mcv /mcv
COPY --from=license /third_party/library /third_party/library
COPY kernelcache/mcv/entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

# Drop any pre-existing ubuntu user/group that claims UID/GID 1000 (present in
# ubuntu:24.04 and nvcr.io/nvidia/cuda:*-ubuntu24.04 base images).
RUN userdel -r ubuntu 2>/dev/null; groupdel ubuntu 2>/dev/null; \
    groupadd -g 1000 appgroup && \
    useradd -u 1000 -g 1000 -m -s /bin/bash appuser
RUN test "$(id -u appuser)" = "1000"
RUN chown appuser:1000 /mcv /entrypoint.sh
RUN mkdir -p /home/appuser/.local/share/containers/storage \
             /home/appuser/.local/share/containers/runroot \
             /home/appuser/.config/containers && \
    chown -R appuser:1000 /home/appuser/.local /home/appuser/.config
WORKDIR /app
RUN chown -R appuser:1000 /app
USER appuser

LABEL description="MCV NVIDIA - includes CUDA runtime and NVML for GPU detection"
LABEL variant="cuda"

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/mcv"]


# ============================================================================
# UNIFIED TARGET: For mixed GPU environments (NVIDIA + AMD + Gaudi support)
# Includes CUDA/NVML, ROCm, and Habana libraries for auto-detection
# Build: docker build --target mcv-unified -t mcv:unified -f kernelcache/mcv/mcv.Dockerfile .
# Usage: mcv --check-compat --image foo (auto-detects GPU vendor)
#        mcv --extract --image foo  (with auto GPU preflight check)
# ============================================================================
FROM nvcr.io/nvidia/cuda:12.6.3-base-ubuntu24.04 AS mcv-unified

ARG TARGETARCH
ARG ROCM_VERSION=7.0.1
ARG AMDGPU_VERSION=7.0.1.70001
ARG OPT_ROCM_VERSION=7.0.1
# SHA-256 of amdgpu-install .deb from repo.radeon.com — update when bumping AMDGPU_VERSION
ARG AMDGPU_INSTALLER_SHA256=f4cec24612039c03271e6ab494bc1e18cb5647d59188755aa8e31b6d74bb06df
# Canonical fingerprint from Intel/Habana documentation (https://docs.habana.ai)
ARG HABANA_SIGNING_FP="6D4D7C0F52A263F383D782791E676CE836A2DE65"

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libgpgme11t64 \
    libbtrfs0 \
    buildah \
    netavark aardvark-dns \
    hwdata \
    pciutils \
    python3 \
 && rm -rf /var/lib/apt/lists/*

# Install ROCm + Habana tools for AMD/Gaudi GPU detection (amd64 only).
# On arm64 the image still provides NVIDIA/NVML support via the CUDA base.
RUN if [ "$TARGETARCH" = "amd64" ]; then \
    apt-get update && apt-get install -y --no-install-recommends \
        wget gnupg2 && \
    rm -rf /var/lib/apt/lists/* && \
    wget -O /tmp/amdgpu-install.deb \
        https://repo.radeon.com/amdgpu-install/${ROCM_VERSION}/ubuntu/jammy/amdgpu-install_${AMDGPU_VERSION}-1_all.deb && \
    echo "${AMDGPU_INSTALLER_SHA256}  /tmp/amdgpu-install.deb" | sha256sum -c - && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y /tmp/amdgpu-install.deb && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        amd-smi-lib rocm-smi-lib libdrm2 && \
    ln -s /opt/rocm-${OPT_ROCM_VERSION}/bin/amd-smi /usr/bin/amd-smi && \
    ln -s /opt/rocm-${OPT_ROCM_VERSION}/bin/rocm-smi /usr/bin/rocm-smi && \
    wget -q -O /tmp/habana-key.asc https://vault.habana.ai/artifactory/api/gpg/key/public && \
    gpg --dearmor < /tmp/habana-key.asc > /tmp/habana-key.gpg && \
    actual=$(gpg --no-default-keyring --keyring /tmp/habana-key.gpg --fingerprint 2>/dev/null \
             | awk '/^      /{gsub(/ /,"",$0); print}' | head -1) && \
    expected=$(printf '%s' "${HABANA_SIGNING_FP}" | tr -d ' :') && \
    [ "$actual" = "$expected" ] || { echo "Habana GPG key fingerprint mismatch: got $actual expected $expected"; exit 1; } && \
    mv /tmp/habana-key.gpg /usr/share/keyrings/habana-artifactory.gpg && \
    rm /tmp/habana-key.asc && \
    chmod 644 /usr/share/keyrings/habana-artifactory.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/habana-artifactory.gpg] https://vault.habana.ai/artifactory/debian noble main" \
        > /etc/apt/sources.list.d/habana.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends habanalabs-firmware-tools && \
    rm -f /etc/apt/sources.list.d/habana.list && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/amdgpu-install.deb; \
fi

RUN mkdir -p /etc/containers && \
 printf '[storage]\ndriver="vfs"\nrunroot="/home/appuser/.local/share/containers/runroot"\ngraphroot="/home/appuser/.local/share/containers/storage"\n' \
   > /etc/containers/storage.conf

COPY --from=builder /go/src/github.com/kserve/kernelcache/mcv/mcv /mcv
COPY --from=license /third_party/library /third_party/library
COPY kernelcache/mcv/entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

# Drop any pre-existing ubuntu user/group that claims UID/GID 1000 (present in
# ubuntu:24.04 and nvcr.io/nvidia/cuda:*-ubuntu24.04 base images).
RUN userdel -r ubuntu 2>/dev/null; groupdel ubuntu 2>/dev/null; \
    groupadd -g 1000 appgroup && \
    useradd -u 1000 -g 1000 -m -s /bin/bash appuser
RUN test "$(id -u appuser)" = "1000"
RUN chown appuser:1000 /mcv /entrypoint.sh
RUN mkdir -p /home/appuser/.local/share/containers/storage \
             /home/appuser/.local/share/containers/runroot \
             /home/appuser/.config/containers && \
    chown -R appuser:1000 /home/appuser/.local /home/appuser/.config
WORKDIR /app
RUN chown -R appuser:1000 /app
USER appuser

LABEL description="MCV Unified - NVIDIA (CUDA/NVML), AMD (ROCm), and Intel Gaudi support"
LABEL variant="unified"
LABEL gpu-support="cuda,rocm,gaudi"

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/mcv"]

# Build examples:
# Minimal (no GPU libs):  make docker-build-mcv-minimal
# ROCm (AMD):             make docker-build-mcv-rocm
# Gaudi (Intel):          make docker-build-mcv-gaudi
# NVIDIA (CUDA):          make docker-build-mcv-cuda
# Unified (all GPUs):     make docker-build-mcv-unified
# All variants:           make docker-build-mcv
