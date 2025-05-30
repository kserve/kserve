ARG CUDA_VERSION=12.4.1
ARG VENV_PATH=prod_venv
ARG PYTHON_VERSION=3.12
ARG WORKSPACE_DIR=/kserve-workspace

#################### BASE BUILD IMAGE ####################
# prepare basic build environment
FROM nvidia/cuda:${CUDA_VERSION}-devel-ubuntu22.04 AS base

ARG WORKSPACE_DIR
ARG CUDA_VERSION
ARG PYTHON_VERSION

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update -y \
    && apt-get install -y software-properties-common ccache curl git sudo gcc g++ kmod \
    && add-apt-repository ppa:deadsnakes/ppa \
    && apt-get update -y \
    && apt-get install -y python${PYTHON_VERSION} python${PYTHON_VERSION}-dev python${PYTHON_VERSION}-venv \
    && update-alternatives --install /usr/bin/python3 python3 /usr/bin/python${PYTHON_VERSION} 1 \
    && update-alternatives --set python3 /usr/bin/python${PYTHON_VERSION} \
    && ln -sf /usr/bin/python${PYTHON_VERSION}-config /usr/bin/python3-config \
    && ln -sf /usr/bin/python${PYTHON_VERSION} /usr/bin/python \
    && curl -sS https://bootstrap.pypa.io/get-pip.py | python${PYTHON_VERSION} \
    && python3 --version && python3 -m pip --version \
    && gcc -v && g++ -v \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.8.3
RUN --mount=type=cache,target=/root/.cache/pip curl -sSL https://install.python-poetry.org | python3 - --version $POETRY_VERSION -y
ENV PATH="$PATH:${POETRY_HOME}/bin"

# Workaround for https://github.com/openai/triton/issues/2507 and
# https://github.com/pytorch/pytorch/issues/107960 -- hopefully
# this won't be needed for future versions of this docker image
# or future versions of triton.
RUN ldconfig /usr/local/cuda-$(echo $CUDA_VERSION | cut -d. -f1,2)/compat/

# cuda arch list used by torch
# can be useful for both `dev` and `test`
# explicitly set the list to avoid issues with torch 2.2
# see https://github.com/pytorch/pytorch/pull/123243
ARG torch_cuda_arch_list='7.0 7.5 8.0 8.6 8.9 9.0+PTX'
ENV TORCH_CUDA_ARCH_LIST=${torch_cuda_arch_list}
# Override the arch list for flash-attn to reduce the binary size
ARG vllm_fa_cmake_gpu_arches='80-real;90-real'
ENV VLLM_FA_CMAKE_GPU_ARCHES=${vllm_fa_cmake_gpu_arches}

WORKDIR ${WORKSPACE_DIR}
#################### BASE BUILD IMAGE ####################

#################### WHEEL BUILD IMAGE ####################
FROM base AS build

ARG TARGETPLATFORM
ARG WORKSPACE_DIR
ARG VLLM_VERSION=0.8.5
ARG LMCACHE_VERSION=0.2.1

WORKDIR ${WORKSPACE_DIR}

# TODO: Remove setuptools and packaging version pinning when upgrading to vLLM > 0.8.5
# Support for PEP 639 was added in setuptools 77.0.3
# https://github.com/vllm-project/vllm/issues/17464
RUN --mount=type=cache,target=/root/.cache/pip \
    if [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
        pip install setuptools==77.0.3 packaging==24.2; \
    fi

# ARM64+CUDA support for pytorch only available in nightly builds.
# ARM64 (NVIDIA GH200) build follows the practice of "use existing pytorch" build,
# we need to install torch and torchvision from the nightly builds first,
# pytorch will not appear as a vLLM dependency in all of the following steps
# after this step
RUN --mount=type=cache,target=/root/.cache/pip \
    if [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
        pip install --index-url https://download.pytorch.org/whl/nightly/cu128 "torch==2.8.0.dev20250321+cu128" "torchvision==0.22.0.dev20250322";  \
        pip install --index-url https://download.pytorch.org/whl/nightly/cu128 --pre pytorch-triton==3.3.0+git96316ce5; \
    fi

# max jobs used by Ninja to build extensions
ARG max_jobs=1
ENV MAX_JOBS=${max_jobs}
# number of threads used by nvcc
ARG nvcc_threads=1
ENV NVCC_THREADS=$nvcc_threads

# Build vLLM wheel for ARM64
ENV VLLM_TARGET_DEVICE=cuda
RUN --mount=type=cache,target=/root/.cache/pip \
    if [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
        git clone --single-branch --branch v${VLLM_VERSION} https://github.com/vllm-project/vllm.git && \
        cd vllm && python3 use_existing_torch.py && \
        pip install -v -r requirements/build.txt && \
        pip install -v -r requirements/cuda.txt && \
        python3 setup.py bdist_wheel --dist-dir=dist --py-limited-api=cp38; \
    fi

# From this point, all Python packages will be installed in the virtual environment and copied to the final image.
# Make sure build dependencies are not installed in the final image.

ARG VENV_PATH
RUN python3 -m venv ${VENV_PATH}
# Activate virtual env by setting VIRTUAL_ENV
ENV VIRTUAL_ENV=${WORKSPACE_DIR}/${VENV_PATH}
ENV PATH="${WORKSPACE_DIR}/${VENV_PATH}/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN --mount=type=cache,target=/root/.cache/pypoetry cd kserve && poetry install --no-root --no-interaction
COPY kserve kserve
RUN --mount=type=cache,target=/root/.cache/pypoetry cd kserve && poetry install --no-interaction


COPY huggingfaceserver/pyproject.toml huggingfaceserver/poetry.lock huggingfaceserver/
# Remove vllm from the huggingface and kserve dependencies as wheels is not available for ARM64
# Remove torch from the huggingface dependencies as wheels for ARM64+CUDA is not available.
RUN --mount=type=cache,target=/root/.cache/pypoetry \
    if [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
        cd huggingfaceserver; \
        sed -i -E 's/(kserve\s*=\s*\{[^\}]*extras\s*=\s*\[)[^]]*\]/\1"storage"\]/' pyproject.toml && \
        sed -i '/^\s*vllm\s*=/d' pyproject.toml; \
        sed -i '/^\s*torch\s*=/d' pyproject.toml; \
        poetry lock --no-update; \
    fi
RUN --mount=type=cache,target=/root/.cache/pypoetry cd huggingfaceserver && poetry install --no-root --no-interaction
COPY huggingfaceserver huggingfaceserver
RUN --mount=type=cache,target=/root/.cache/pypoetry cd huggingfaceserver && poetry install --no-interaction --only-root

# ARM64+CUDA support for pytorch only available in nightly builds.
# ARM64 (NVIDIA GH200) build follows the practice of "use existing pytorch" build,
# we need to install torch and torchvision from the nightly builds first,
# pytorch will not appear as a vLLM dependency in all of the following steps
# after this step.
RUN --mount=type=cache,target=/root/.cache/pip \
    if [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
        pip install --index-url https://download.pytorch.org/whl/nightly/cu128 "torch==2.8.0.dev20250318+cu128" "torchvision==0.22.0.dev20250319";  \
        pip install --index-url https://download.pytorch.org/whl/nightly/cu128 --pre pytorch_triton==3.3.0+gitab727c40; \
    fi

# Install vllm
# https://docs.vllm.ai/en/latest/models/extensions/runai_model_streamer.html, https://docs.vllm.ai/en/latest/models/extensions/tensorizer.html
# https://docs.vllm.ai/en/latest/models/extensions/fastsafetensor.html
RUN --mount=type=cache,target=/root/.cache/pip \
    if [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
        pip install vllm/dist/*.whl[runai,tensorizer,fastsafetensors] --verbose; \
    else \
        pip install vllm[runai,tensorizer,fastsafetensors]==${VLLM_VERSION}; \
    fi

# Install lmcache
RUN --mount=type=cache,target=/root/.cache/pip \
    if [ "$TARGETPLATFORM" = "linux/amd64" ]; then \
        pip install lmcache==${LMCACHE_VERSION}; \
    fi

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
RUN mkdir -p third_party/library && python3 pip-licenses.py
#################### WHEEL BUILD IMAGE ####################

#################### PROD IMAGE ####################
FROM nvidia/cuda:${CUDA_VERSION}-runtime-ubuntu22.04 AS prod

ARG WORKSPACE_DIR
ARG CUDA_VERSION
ARG PYTHON_VERSION

ENV DEBIAN_FRONTEND=noninteractive

WORKDIR ${WORKSPACE_DIR}

# Install Python and other dependencies
RUN apt-get update -y \
    && apt-get upgrade -y \
    && apt-get install -y software-properties-common curl \
    && apt-get install -y ffmpeg libsm6 libxext6 libgl1 gcc libibverbs-dev \
    && add-apt-repository ppa:deadsnakes/ppa \
    && apt-get update -y \
    && apt-get install -y python${PYTHON_VERSION} python${PYTHON_VERSION}-dev python${PYTHON_VERSION}-venv \
    && update-alternatives --install /usr/bin/python3 python3 /usr/bin/python${PYTHON_VERSION} 1 \
    && update-alternatives --set python3 /usr/bin/python${PYTHON_VERSION} \
    && ln -sf /usr/bin/python${PYTHON_VERSION}-config /usr/bin/python3-config \
    && ln -sf /usr/bin/python${PYTHON_VERSION} /usr/bin/python \
    && curl -sS https://bootstrap.pypa.io/get-pip.py | python${PYTHON_VERSION} \
    && python3 --version && python3 -m pip --version \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

ARG VENV_PATH
# Activate virtual env by setting VIRTUAL_ENV
ENV VIRTUAL_ENV=${WORKSPACE_DIR}/${VENV_PATH}
ENV PATH="${WORKSPACE_DIR}/${VENV_PATH}/bin:$PATH"
RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=build --chown=kserve:kserve ${WORKSPACE_DIR}/third_party third_party
COPY --from=build --chown=kserve:kserve ${WORKSPACE_DIR}/$VENV_PATH $VENV_PATH
COPY --from=build ${WORKSPACE_DIR}/kserve kserve
COPY --from=build ${WORKSPACE_DIR}/huggingfaceserver huggingfaceserver

# Set a writable Hugging Face home folder to avoid permission issue. See https://github.com/kserve/kserve/issues/3562
ENV HF_HOME="/tmp/huggingface"
# https://huggingface.co/docs/safetensors/en/speed#gpu-benchmark
ENV SAFETENSORS_FAST_GPU="1"
# https://huggingface.co/docs/huggingface_hub/en/package_reference/environment_variables#hfhubdisabletelemetry
ENV HF_HUB_DISABLE_TELEMETRY="1"
# NCCL Lib path for vLLM. https://github.com/vllm-project/vllm/blob/ec784b2526219cd96159a52074ab8cd4e684410a/vllm/utils.py#L598-L602
ENV VLLM_NCCL_SO_PATH="/lib/x86_64-linux-gnu/libnccl.so.2"
# https://github.com/vllm-project/vllm/issues/6152
# Set the multiprocess method to spawn to avoid issues with cuda initialization for `mp` executor backend.
ENV VLLM_WORKER_MULTIPROC_METHOD="spawn"

USER 1000
ENTRYPOINT ["python3", "-m", "huggingfaceserver"]
#################### PROD IMAGE ####################
