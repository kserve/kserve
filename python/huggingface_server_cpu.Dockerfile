ARG BASE_IMAGE=ubuntu:22.04
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS base

ARG PYTHON=python3

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install --no-install-recommends --fix-missing -y \
        g++-12 \
        gcc-12 \
        google-perftools \
        libgl1 \
        libglib2.0-0 \
        libjemalloc2 \
        libnuma1 \
        numactl \
        python3.10-dev \
        python3.10-venv \
        python3-pip \
        curl && \
    apt-get clean && \
    apt-get autoclean && \
    apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*

RUN update-alternatives --install /usr/bin/gcc gcc /usr/bin/gcc-12 10 --slave /usr/bin/g++ g++ /usr/bin/g++-12

RUN ln -sf "$(which ${PYTHON})" /usr/bin/python

FROM base AS builder

# Install uv
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
ln -s /root/.local/bin/uv /usr/local/bin/uv

# Install build dependencies
RUN --mount=type=cache,target=/var/cache/apt \
    apt-get update && \
    apt-get install --no-install-recommends --fix-missing -y \
        build-essential \
        git \
        libnuma-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

ARG TORCH_EXTRA_INDEX_URL="https://download.pytorch.org/whl/cpu"
ARG IPEX_EXTRA_INDEX_URL="https://pytorch-extension.intel.com/release-whl/stable/cpu/us/"
ARG TORCH_VERSION=2.7.0
ARG TORCHVISION_VERSION=0.22.0

# Install kserve using UV
COPY kserve kserve
RUN cd kserve && \
    uv sync --active --no-cache && \
    uv cache clean && \
    rm -rf ~/.cache/uv

# Install huggingfaceserver using UV
COPY huggingfaceserver huggingfaceserver
RUN cd huggingfaceserver && \
    uv pip install --extra-index-url ${TORCH_EXTRA_INDEX_URL} \
        'torch~='${TORCH_VERSION} \
        'torchaudio~='${TORCH_VERSION} \
        'torchvision~='${TORCHVISION_VERSION} && \
    poetry lock --no-update && \
    poetry install --no-interaction --no-cache && rm -rf ~/.cache/pypoetry

    # uv sync --active --no-cache && \
    # uv cache clean && \
    # rm -rf ~/.cache/uv

RUN pip install --no-cache --extra-index-url ${TORCH_EXTRA_INDEX_URL} --extra-index-url ${IPEX_EXTRA_INDEX_URL} \
    'intel_extension_for_pytorch~='${TORCH_VERSION} \
    intel-openmp

# install vllm
ARG VLLM_VERSION=0.9.0.1
ARG VLLM_CPU_DISABLE_AVX512=true
ENV VLLM_CPU_DISABLE_AVX512=${VLLM_CPU_DISABLE_AVX512}
ARG VLLM_CPU_AVX512BF16=1
ENV VLLM_CPU_AVX512BF16=${VLLM_CPU_AVX512BF16}
ARG VLLM_TARGET_DEVICE=cpu
ENV VLLM_TARGET_DEVICE=${VLLM_TARGET_DEVICE}
# Clone vLLM repo
RUN git clone --single-branch --branch v${VLLM_VERSION} https://github.com/vllm-project/vllm.git

# Install vLLM build requirements
RUN cd vllm && \
    uv pip install --no-cache -v -r requirements/build.txt && \
    uv cache clean

# Install vLLM cpu requirements
RUN cd vllm && \
    uv pip install --index-strategy unsafe-best-match --no-cache -v -r requirements/cpu.txt && \
    uv cache clean

# Build vLLM wheel
RUN cd vllm && \
    python setup.py bdist_wheel

# Install built vLLM wheel
RUN uv pip install --no-cache vllm/dist/vllm-${VLLM_VERSION}*.whl

# Cleanup vllm source code and caches
RUN rm -rf /vllm /root/.cache/uv /root/.cache/pip /tmp/*

RUN df -hT

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN pip install --no-cache-dir tomli
RUN mkdir -p third_party/library && python3 pip-licenses.py

# Build the final image
FROM base AS prod

RUN echo 'ulimit -c 0' >> ~/.bashrc

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder --chown=kserve:kserve huggingfaceserver huggingfaceserver
COPY --from=builder --chown=kserve:kserve kserve kserve

RUN df -hT

# Set a writable Hugging Face home folder to avoid permission issue. See https://github.com/kserve/kserve/issues/3562
ENV HF_HOME="/tmp/huggingface"
# https://huggingface.co/docs/huggingface_hub/en/package_reference/environment_variables#hfhubdisabletelemetry
ENV HF_HUB_DISABLE_TELEMETRY="1"

# Use TCMalloc and jemalloc for better memory management
ENV LD_PRELOAD=/usr/lib/x86_64-linux-gnu/libtcmalloc.so.4:/usr/lib/x86_64-linux-gnu/libjemalloc.so.2:${LD_PRELOAD}

USER 1000
ENV PYTHONPATH=/huggingfaceserver
ENTRYPOINT ["python", "-m", "huggingfaceserver"]