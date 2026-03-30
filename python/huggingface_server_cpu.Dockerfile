ARG BASE_IMAGE=ubuntu:22.04
ARG VENV_PATH=/prod_venv

# ---- Runtime base: only what the production image needs ----
FROM ${BASE_IMAGE} AS base-runtime

ARG PYTHON=python3

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install --no-install-recommends --fix-missing -y \
        libgl1 \
        libglib2.0-0 \
        libjemalloc2 \
        libnuma1 \
        numactl \
        python3.10-dev \
        python3.10-venv \
        python3-pip && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN ln -sf "$(which ${PYTHON})" /usr/bin/python

# ---- Build base: adds compilers and build tools ----
FROM base-runtime AS base-build

RUN --mount=type=cache,sharing=locked,target=/var/cache/apt --mount=type=cache,sharing=locked,target=/var/lib/apt/lists \
    apt-get update && \
    apt-get install --no-install-recommends --fix-missing -y \
        build-essential \
        ccache \
        g++-12 \
        gcc-12 \
        git \
        libnuma-dev

RUN update-alternatives --install /usr/bin/gcc gcc /usr/bin/gcc-12 10 --slave /usr/bin/g++ g++ /usr/bin/g++-12

# ---- Builder stage ----
FROM base-build AS builder

COPY --from=ghcr.io/astral-sh/uv:0.7 /uv /usr/local/bin/uv

ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

ARG TORCH_EXTRA_INDEX_URL="https://download.pytorch.org/whl/cpu"
ARG TORCH_VERSION=2.10.0

# ---- Install torch (needed by both vllm build and huggingfaceserver) ----
RUN --mount=type=cache,target=/root/.cache/uv \
    uv pip install --index-url ${TORCH_EXTRA_INDEX_URL} \
        torch==${TORCH_VERSION} \
        torchvision \
        torchaudio

# ---- Build vllm FIRST (changes rarely, most expensive step) ----
ARG VLLM_VERSION=0.17.1
ARG VLLM_CPU_DISABLE_AVX512=true
ENV VLLM_CPU_DISABLE_AVX512=${VLLM_CPU_DISABLE_AVX512}
ARG VLLM_CPU_AVX512BF16=1
ENV VLLM_CPU_AVX512BF16=${VLLM_CPU_AVX512BF16}
ARG VLLM_TARGET_DEVICE=cpu
ENV VLLM_TARGET_DEVICE=${VLLM_TARGET_DEVICE}

RUN git clone --single-branch --branch v${VLLM_VERSION} https://github.com/vllm-project/vllm.git

RUN --mount=type=cache,target=/root/.cache/uv cd vllm && \
    uv pip install -v --index-strategy unsafe-best-match --extra-index-url ${TORCH_EXTRA_INDEX_URL} -r requirements/cpu-build.txt

RUN --mount=type=cache,target=/root/.cache/uv cd vllm && \
    uv pip install -v --index-strategy unsafe-best-match --extra-index-url ${TORCH_EXTRA_INDEX_URL} -r requirements/cpu.txt

ENV PATH="/usr/lib/ccache:$PATH"
RUN --mount=type=cache,target=/root/.ccache \
    cd vllm && CCACHE_DIR=/root/.ccache python setup.py bdist_wheel

RUN --mount=type=cache,target=/root/.cache/uv uv pip install vllm/dist/vllm-${VLLM_VERSION}*.whl

RUN rm -rf vllm /tmp/*

# ---- Install kserve (changes often, fast to reinstall) ----
COPY storage/pyproject.toml storage/uv.lock storage/
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN --mount=type=cache,target=/root/.cache/uv cd kserve && uv sync --active --inexact

COPY kserve kserve
RUN --mount=type=cache,target=/root/.cache/uv cd kserve && uv sync --active --inexact

# ---- Install storage ----
COPY storage storage
RUN --mount=type=cache,target=/root/.cache/uv cd storage && uv pip install .

# ---- Install huggingfaceserver ----
COPY huggingfaceserver/pyproject.toml huggingfaceserver/uv.lock huggingfaceserver/
RUN --mount=type=cache,target=/root/.cache/uv cd huggingfaceserver && uv sync --active --inexact

COPY huggingfaceserver huggingfaceserver
RUN --mount=type=cache,target=/root/.cache/uv cd huggingfaceserver && uv sync --active --inexact

# Restore CPU-optimized torch - uv sync resolves torch to the generic PyPI version
# (pinned in huggingfaceserver's lockfile), replacing the CPU-index wheels installed earlier.
RUN --mount=type=cache,target=/root/.cache/uv \
    uv pip install --index-url ${TORCH_EXTRA_INDEX_URL} \
        torch==${TORCH_VERSION} \
        torchvision \
        torchaudio

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN --mount=type=cache,target=/root/.cache/pip pip install tomli
RUN mkdir -p third_party/library && python3 pip-licenses.py

# ---- Production image ----
FROM base-runtime AS prod

ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder --chown=kserve:kserve huggingfaceserver huggingfaceserver
COPY --from=builder --chown=kserve:kserve kserve kserve
COPY --from=builder --chown=kserve:kserve storage storage

ENV HF_HOME="/tmp/huggingface"
ENV HF_HUB_DISABLE_TELEMETRY="1"

# Use jemalloc for better memory management (matches the GPU image).
# The original Dockerfile loaded both tcmalloc and jemalloc, but LD_PRELOAD
# resolves left-to-right so only the first allocator is used - the second is dead weight.
ENV LD_PRELOAD=/usr/lib/x86_64-linux-gnu/libjemalloc.so.2

USER 1000
ENV PYTHONPATH=/huggingfaceserver
ENTRYPOINT ["python", "-m", "huggingfaceserver"]
