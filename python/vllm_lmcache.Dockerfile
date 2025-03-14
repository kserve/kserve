ARG CUDA_VERSION=12.4.1
ARG VENV_PATH=/prod_venv

#################### BASE IMAGE ####################
# TODO: Restore to base image after FlashInfer AOT wheel fixed
FROM nvidia/cuda:${CUDA_VERSION}-devel-ubuntu22.04 AS base
ARG CUDA_VERSION=12.4.1
ARG PYTHON_VERSION=3.12
ENV DEBIAN_FRONTEND=noninteractive

# Install Python and other dependencies
RUN apt-get update -y \
    && apt-get install -y ccache software-properties-common git curl sudo \
    && apt-get install -y libibverbs1 ibverbs-providers \
    && add-apt-repository ppa:deadsnakes/ppa \
    && apt-get update -y \
    && apt-get install -y python${PYTHON_VERSION} python${PYTHON_VERSION}-dev python${PYTHON_VERSION}-venv \
    && update-alternatives --install /usr/bin/python3 python3 /usr/bin/python${PYTHON_VERSION} 1 \
    && update-alternatives --set python3 /usr/bin/python${PYTHON_VERSION} \
    && ln -sf /usr/bin/python${PYTHON_VERSION}-config /usr/bin/python3-config \
    && curl -sS https://bootstrap.pypa.io/get-pip.py | python${PYTHON_VERSION} \
    && python3 --version && python3 -m pip --version \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*


# Workaround for https://github.com/openai/triton/issues/2507 and
# https://github.com/pytorch/pytorch/issues/107960 -- hopefully
# this won't be needed for future versions of this docker image
# or future versions of triton.
RUN ldconfig /usr/local/cuda-$(echo $CUDA_VERSION | cut -d. -f1,2)/compat/

WORKDIR /workspace


#################### BUILD IMAGE ####################
FROM base AS build

#################### LMCache WHEEL BUILD ####################
# cuda arch list used by torch
# can be useful for both `dev` and `test`
# explicitly set the list to avoid issues with torch 2.2
# see https://github.com/pytorch/pytorch/pull/123243
ARG torch_cuda_arch_list='7.0 7.5 8.0 8.6 8.9 9.0+PTX'
ENV TORCH_CUDA_ARCH_LIST=${torch_cuda_arch_list}
# Override the arch list for flash-attn to reduce the binary size
ARG vllm_fa_cmake_gpu_arches='80-real;90-real'
ENV VLLM_FA_CMAKE_GPU_ARCHES=${vllm_fa_cmake_gpu_arches}


# max jobs used by Ninja to build extensions
ARG max_jobs=2
ENV MAX_JOBS=${max_jobs}
# number of threads used by nvcc
ARG nvcc_threads=8
ENV NVCC_THREADS=$nvcc_threads

ARG LMCACHE_COMMIT_ID=1

RUN git clone https://github.com/LMCache/LMCache.git
RUN git clone https://github.com/LMCache/torchac_cuda.git

# install build dependencies
RUN --mount=type=cache,target=/root/.cache/pip \
    python3 -m pip install -r LMCache/docker/requirements-build.txt


WORKDIR /workspace/LMCache
RUN --mount=type=cache,target=/root/.cache/ccache \
    --mount=type=cache,target=/root/.cache/pip \
    python3 setup.py bdist_wheel --dist-dir=dist_lmcache

WORKDIR /workspace/torchac_cuda
RUN --mount=type=cache,target=/root/.cache/ccache \
    --mount=type=cache,target=/root/.cache/pip \
    python3 setup.py bdist_wheel --dist-dir=/workspace/LMCache/dist_lmcache

#################### vLLM installation ####################

WORKDIR /

ARG POETRY_HOME=/opt/poetry
RUN --mount=type=cache,target=/root/.cache/pip curl -sSL https://install.python-poetry.org | python3 - --version 1.8.3 -y
ENV PATH="$PATH:${POETRY_HOME}/bin"

# Install vllm
ARG VLLM_VERSION=0.7.3

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

COPY huggingfaceserver/pyproject.toml huggingfaceserver/poetry.lock huggingfaceserver/health_check.py huggingfaceserver/
RUN cd huggingfaceserver && poetry install --no-root --no-interaction --no-cache
COPY huggingfaceserver huggingfaceserver
RUN cd huggingfaceserver && poetry install --no-interaction --no-cache

# https://docs.vllm.ai/en/latest/models/extensions/runai_model_streamer.html, https://docs.vllm.ai/en/latest/models/extensions/tensorizer.html
RUN --mount=type=cache,target=/root/.cache/pip pip install --upgrade pip && pip install vllm[runai,tensorizer]==${VLLM_VERSION}

# Install FlashInfer Attention backend
RUN --mount=type=cache,target=/root/.cache/pip pip install https://github.com/flashinfer-ai/flashinfer/releases/download/v0.2.1.post1/flashinfer_python-0.2.1.post1+cu124torch2.5-cp38-abi3-linux_x86_64.whl

# Although we build Flashinfer with AOT mode, there's still
# some issues w.r.t. JIT compilation. Therefore we need to
# install build dependencies for JIT compilation.
# TODO: Remove this once FlashInfer AOT wheel is fixed
RUN --mount=type=cache,target=/root/.cache/pip curl -sSLo requirements-build.txt https://github.com/vllm-project/vllm/raw/refs/tags/v0.7.3/requirements-build.txt \
    && pip install -r requirements-build.txt

RUN --mount=type=cache,target=/root/.cache/pip pip install accelerate hf_transfer 'modelscope!=1.15.0' 'bitsandbytes>=0.45.0' 'timm==0.9.10' boto3

# Install torchac_cuda and lmcache wheel
RUN --mount=type=cache,target=/root/.cache/pip pip install /workspace/LMCache/dist_lmcache/*.whl --verbose

# TODO: Remove this patch once the next vLLM release is available
RUN git clone https://github.com/vllm-project/vllm.git
RUN cd vllm && git checkout 6d7f037 # https://github.com/vllm-project/vllm/commit/6d7f037748b2e7df64f3318e54101a1c80016f3c

# Copy lmc_connector patch into vllm
RUN cp vllm/vllm/distributed/kv_transfer/kv_connector/factory.py \
    ${VENV_PATH}/lib/python3.12/site-packages/vllm/distributed/kv_transfer/kv_connector/
RUN cp vllm/vllm/distributed/kv_transfer/kv_connector/lmcache_connector.py \
    ${VENV_PATH}/lib/python3.12/site-packages/vllm/distributed/kv_transfer/kv_connector/

COPY vllm_parallel_state.patch parallel_state.patch
RUN patch ${VENV_PATH}/lib/python3.12/site-packages/vllm/distributed/parallel_state.py parallel_state.patch

#################### PRODUCTION IMAGE ####################
FROM base AS prod

WORKDIR /

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=build --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=build kserve kserve
COPY --from=build huggingfaceserver huggingfaceserver

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