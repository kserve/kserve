ARG BASE_IMAGE=ubuntu:22.04
ARG VENV_PATH=prod_venv
ARG PYTHON_VERSION=3.12
ARG WORKSPACE_DIR=/kserve-workspace

#################### BASE BUILD IMAGE ####################
# prepare basic build environment
FROM ${BASE_IMAGE} AS base

ARG WORKSPACE_DIR
ARG PYTHON_VERSION

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update -y \
    && apt-get upgrade -y \
    && apt-get install --no-install-recommends -y \
    software-properties-common \
    gcc-12 \
    g++-12 \
    google-perftools \
    libgl1 \
    libglib2.0-0 \
    libjemalloc2 \
    libnuma1 \
    numactl \
    python-is-python3 \
    gnupg \
    && add-apt-repository ppa:deadsnakes/ppa \
    && apt-get update -y \
    && apt-get install -y python${PYTHON_VERSION} python${PYTHON_VERSION}-dev python${PYTHON_VERSION}-venv \
    && update-alternatives --install /usr/bin/python3 python3 /usr/bin/python${PYTHON_VERSION} 1 \
    && update-alternatives --set python3 /usr/bin/python${PYTHON_VERSION} \
    && ln -sf /usr/bin/python${PYTHON_VERSION}-config /usr/bin/python3-config \
    && curl -sS https://bootstrap.pypa.io/get-pip.py | python${PYTHON_VERSION} \
    && python3 --version && python3 -m pip --version \
    && apt-get clean && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*


RUN update-alternatives --install /usr/bin/gcc gcc /usr/bin/gcc-12 10 --slave /usr/bin/g++ g++ /usr/bin/g++-12

WORKDIR ${WORKSPACE_DIR}
#################### BASE BUILD IMAGE ####################

#################### WHEEL BUILD IMAGE ####################
FROM base AS build

ARG TARGETPLATFORM
ARG WORKSPACE_DIR
ARG VLLM_VERSION=0.9.0.1

WORKDIR ${WORKSPACE_DIR}

RUN --mount=type=cache,target=/var/cache/apt \
    apt-get update && \
    apt-get install --no-install-recommends --fix-missing -y \
    build-essential \
    git \
    curl \
    libnuma-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.8.3
RUN --mount=type=cache,target=/root/.cache/pip curl -sSL https://install.python-poetry.org | python3 - --version $POETRY_VERSION -y
ENV PATH="$PATH:${POETRY_HOME}/bin"

ARG TORCH_EXTRA_INDEX_URL="https://download.pytorch.org/whl/cpu"
ARG IPEX_EXTRA_INDEX_URL="https://pytorch-extension.intel.com/release-whl/stable/cpu/us/"
ARG TORCH_VERSION=2.7.0
ARG TORCHVISION_VERSION=0.22.0

ARG VLLM_CPU_DISABLE_AVX512=true
ENV VLLM_CPU_DISABLE_AVX512=${VLLM_CPU_DISABLE_AVX512}
ARG VLLM_CPU_AVX512BF16=1
ENV VLLM_CPU_AVX512BF16=${VLLM_CPU_AVX512BF16}
ARG VLLM_TARGET_DEVICE=cpu
ENV VLLM_TARGET_DEVICE=${VLLM_TARGET_DEVICE}

RUN --mount=type=cache,target=/root/.cache/pip git clone --single-branch --branch v${VLLM_VERSION} https://github.com/vllm-project/vllm.git && \
    cd vllm && \
    pip install -r requirements/build.txt && \
    pip install -r requirements/cpu.txt --extra-index-url ${TORCH_EXTRA_INDEX_URL} && \
    python setup.py bdist_wheel --dist-dir dist

# From this point, all Python packages will be installed in the virtual environment and copied to the final image.
# Make sure build dependencies are not installed in the final image.

ARG VENV_PATH
RUN python3 -m venv ${VENV_PATH}
# Activate virtual env by setting VIRTUAL_ENV
ENV VIRTUAL_ENV=${WORKSPACE_DIR}/${VENV_PATH}
ENV PATH="${WORKSPACE_DIR}/${VENV_PATH}/bin:$PATH"

# Install kserve
COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN --mount=type=cache,target=/root/.cache/pypoetry cd kserve && poetry install --no-root --no-interaction
COPY kserve kserve
RUN --mount=type=cache,target=/root/.cache/pypoetry cd kserve && poetry install --no-interaction

# Install huggingfaceserver
COPY huggingfaceserver/pyproject.toml huggingfaceserver/poetry.lock huggingfaceserver/
# Remove vllm and torch from the huggingface and kserve dependencies as wheels is not available for CPU
RUN --mount=type=cache,target=/root/.cache/pypoetry \
    cd huggingfaceserver; \
    sed -i -E 's/(kserve\s*=\s*\{[^\}]*extras\s*=\s*\[)[^]]*\]/\1"storage"\]/' pyproject.toml && \
    sed -i '/^\s*vllm\s*=/d' pyproject.toml; \
    sed -i '/^\s*torch\s*=/d' pyproject.toml; \
    poetry lock --no-update;

RUN --mount=type=cache,target=/root/.cache/pypoetry cd huggingfaceserver && poetry install --no-root --no-interaction
COPY huggingfaceserver huggingfaceserver
RUN --mount=type=cache,target=/root/.cache/pypoetry cd huggingfaceserver && poetry install --no-interaction --only-root

RUN --mount=type=cache,target=/root/.cache/pip \
    if [ "$TARGETPLATFORM" = "linux/amd64" ]; then \
      pip install --extra-index-url ${TORCH_EXTRA_INDEX_URL} --extra-index-url ${IPEX_EXTRA_INDEX_URL} \
      'intel_extension_for_pytorch~='${TORCH_VERSION} intel-openmp; \
    fi

# Install vllm
RUN --mount=type=cache,target=/root/.cache/pip pip install --no-cache-dir vllm/dist/vllm-${VLLM_VERSION}*.whl --extra-index-url ${TORCH_EXTRA_INDEX_URL}

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
RUN mkdir -p third_party/library && python3 pip-licenses.py

# Build the final image
FROM base AS prod

RUN echo 'ulimit -c 0' >> ~/.bashrc

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=build --chown=kserve:kserve ${WORKSPACE_DIR}/third_party third_party
COPY --from=build --chown=kserve:kserve ${WORKSPACE_DIR}/$VENV_PATH $VENV_PATH
COPY --from=build ${WORKSPACE_DIR}/kserve kserve
COPY --from=build ${WORKSPACE_DIR}/huggingfaceserver huggingfaceserver

# Set a writable Hugging Face home folder to avoid permission issue. See https://github.com/kserve/kserve/issues/3562
ENV HF_HOME="/tmp/huggingface"
# https://huggingface.co/docs/huggingface_hub/en/package_reference/environment_variables#hfhubdisabletelemetry
ENV HF_HUB_DISABLE_TELEMETRY="1"

# Use TCMalloc and jemalloc for better memory management
ENV LD_PRELOAD=/usr/lib/x86_64-linux-gnu/libtcmalloc.so.4:/usr/lib/x86_64-linux-gnu/libjemalloc.so.2:${LD_PRELOAD}

USER 1000
ENTRYPOINT ["python", "-m", "huggingfaceserver"]
