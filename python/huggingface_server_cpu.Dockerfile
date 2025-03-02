ARG BASE_IMAGE=ubuntu:22.04
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS base

ARG PYTHON=python3

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install --no-install-recommends --fix-missing -y \
        libgl1 \
        libglib2.0-0 \
        libnuma1 \
        numactl \
        python3-pip \
        python3.10-venv && \
    apt-get clean && \
    apt-get autoclean && \
    apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*

RUN ln -sf "$(which ${PYTHON})" /usr/bin/python

FROM base AS builder

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.8.3

RUN --mount=type=cache,target=/var/cache/apt \
    apt-get update && \
    apt-get install --no-install-recommends --fix-missing -y \
        build-essential \
        g++-12 \
        gcc-12 \
        git \
        libnuma-dev \
        python3.10-dev && \        
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN update-alternatives --install /usr/bin/gcc gcc /usr/bin/gcc-12 10 --slave /usr/bin/g++ g++ /usr/bin/g++-12

RUN python -m venv ${POETRY_HOME} && \
    ${POETRY_HOME}/bin/pip install --no-cache-dir --upgrade pip && \
    ${POETRY_HOME}/bin/pip install --no-cache-dir poetry==${POETRY_VERSION}
ENV PATH="$PATH:${POETRY_HOME}/bin"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"
ARG TORCH_EXTRA_INDEX_URL="https://download.pytorch.org/whl/cpu"
ARG IPEX_EXTRA_INDEX_URL="https://pytorch-extension.intel.com/release-whl/stable/cpu/us/"
ARG TORCH_VERSION=2.5.0
ARG TORCHVISION_VERSION=0.20.1

# Install kserve
COPY kserve kserve
RUN cd kserve && \
    poetry install --no-interaction --no-cache && rm -rf ~/.cache/pypoetry

# Install huggingfaceserver
COPY huggingfaceserver huggingfaceserver
RUN cd huggingfaceserver && \
    poetry source add --priority=supplemental pytorch-cpu ${TORCH_EXTRA_INDEX_URL} && \
    poetry add --source pytorch-cpu \
        'torch~='${TORCH_VERSION} \
        'torchaudio~='${TORCH_VERSION} \
        'torchvision~='${TORCHVISION_VERSION} && \
    poetry lock && \
    poetry install --no-interaction --no-cache && rm -rf ~/.cache/pypoetry

RUN pip install --no-cache-dir --extra-index-url ${TORCH_EXTRA_INDEX_URL} --extra-index-url ${IPEX_EXTRA_INDEX_URL} \
    'intel_extension_for_pytorch~='${TORCH_VERSION} \
    intel-openmp

# install vllm
ARG VLLM_VERSION=0.7.3
ARG VLLM_CPU_DISABLE_AVX512=true
ENV VLLM_CPU_DISABLE_AVX512=${VLLM_CPU_DISABLE_AVX512}
ARG VLLM_CPU_AVX512BF16=1
ENV VLLM_CPU_AVX512BF16=${VLLM_CPU_AVX512BF16}
ARG VLLM_TARGET_DEVICE=cpu
ENV VLLM_TARGET_DEVICE=${VLLM_TARGET_DEVICE}
RUN git clone --single-branch --branch v${VLLM_VERSION} https://github.com/vllm-project/vllm.git && \
    cd vllm && \
    pip install --no-cache-dir -v -r requirements-build.txt && \
    pip install --no-cache-dir -v -r requirements-cpu.txt && \
    python setup.py bdist_wheel && \
    pip install --no-cache-dir dist/vllm-${VLLM_VERSION}*.whl

# Build the final image
FROM base AS prod

RUN echo 'ulimit -c 0' >> ~/.bashrc

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder --chown=kserve:kserve huggingfaceserver huggingfaceserver
COPY --from=builder --chown=kserve:kserve kserve kserve

# Set a writable Hugging Face home folder to avoid permission issue. See https://github.com/kserve/kserve/issues/3562
ENV HF_HOME="/tmp/huggingface"
# https://huggingface.co/docs/huggingface_hub/en/package_reference/environment_variables#hfhubdisabletelemetry
ENV HF_HUB_DISABLE_TELEMETRY="1"
# https://github.com/vllm-project/vllm/issues/6152
# Set the multiprocess method to spawn to avoid issues with cuda initialization for `mp` executor backend.
ENV VLLM_WORKER_MULTIPROC_METHOD="spawn"

USER 1000
ENTRYPOINT ["python", "-m", "huggingfaceserver"]
