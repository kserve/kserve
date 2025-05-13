ARG BASE_IMAGE=nvidia/cuda:12.4.1-devel-ubuntu20.04
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

ARG TARGETPLATFORM
ARG CUDA_VERSION=12.4.1
ARG PYTHON_VERSION=3.12
ENV DEBIAN_FRONTEND=noninteractive

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.8.3

# Install vllm
ARG VLLM_VERSION=0.8.5

RUN echo 'tzdata tzdata/Areas select America' | debconf-set-selections \
    && echo 'tzdata tzdata/Zones/America select Los_Angeles' | debconf-set-selections \
    && apt-get update -y \
    && apt-get install -y ccache software-properties-common git curl sudo \
    && add-apt-repository ppa:deadsnakes/ppa \
    && apt-get update -y \
    && apt-get install -y python${PYTHON_VERSION} python${PYTHON_VERSION}-dev python${PYTHON_VERSION}-venv \
    && update-alternatives --install /usr/bin/python3 python3 /usr/bin/python${PYTHON_VERSION} 1 \
    && update-alternatives --set python3 /usr/bin/python${PYTHON_VERSION} \
    && ln -sf /usr/bin/python${PYTHON_VERSION}-config /usr/bin/python3-config \
    && curl -sS https://bootstrap.pypa.io/get-pip.py | python${PYTHON_VERSION} \
    && python3 --version && python3 -m pip --version

RUN apt-get update && apt-get upgrade -y && apt-get install gcc-10 g++-10 -y && apt-get clean && \
    rm -rf /var/lib/apt/lists/*
RUN update-alternatives --install /usr/bin/gcc gcc /usr/bin/gcc-10 110 --slave /usr/bin/g++ g++ /usr/bin/g++-10
RUN python3 -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip3 install poetry==${POETRY_VERSION}
ENV PATH="$PATH:${POETRY_HOME}/bin"

RUN --mount=type=cache,target=/root/.cache/pip \
    if [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
    pip install -r https://github.com/vllm-project/vllm/raw/refs/heads/v0.8.5/requirements/build.txt; \
    pip install --index-url https://download.pytorch.org/whl/nightly/cu128 "torch==2.8.0.dev20250318+cu128" "torchvision==0.22.0.dev20250319";  \
    pip install --index-url https://download.pytorch.org/whl/nightly/cu128 --pre pytorch_triton==3.3.0+gitab727c40; \
    pip download --no-binary ":all:" vllm==0.8.5 && tar -xvf vllm-0.8.5.tar.gz && cd vllm-0.8.5 && python3 setup.py bdist_wheel --dist-dir=dist --py-limited-api=cp38; \
    fi

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

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
ENV VLLM_TARGET_DEVICE=cuda
COPY huggingfaceserver/ huggingfaceserver/
RUN --mount=type=cache,target=/root/.cache/pypoetry \
    if [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
    cd huggingfaceserver; \
    sed -i -E 's/(kserve\s*=\s*\{[^\}]*extras\s*=\s*\[)[^]]*\]/\1"storage"\]/' pyproject.toml &&  sed -i '/^\s*vllm\s*=/d' pyproject.toml; \
    poetry lock --no-update; \
    fi
RUN --mount=type=cache,target=/root/.cache/pypoetry cd huggingfaceserver && poetry install --no-root --no-interaction --no-cache
COPY huggingfaceserver huggingfaceserver
# RUN --mount=type=cache,target=/root/.cache/pypoetry \
#     if [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
#     cd huggingfaceserver; \
#     sed -i -E 's/(kserve\s*=\s*\{[^\}]*extras\s*=\s*\[)[^]]*\]/\1"storage"\]/' pyproject.toml &&  sed -i '/^\s*vllm\s*=/d' pyproject.toml; \
#     poetry lock --no-update; \
#     fi
RUN cd huggingfaceserver && poetry install --no-interaction --no-cache --only-root

RUN --mount=type=cache,target=/root/.cache/pip \
    if [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
    pip install --index-url https://download.pytorch.org/whl/nightly/cu128 "torch==2.8.0.dev20250318+cu128" "torchvision==0.22.0.dev20250319";  \
    pip install --index-url https://download.pytorch.org/whl/nightly/cu128 --pre pytorch_triton==3.3.0+gitab727c40; \
    fi

RUN python3 -c "import torch; print(torch.__version__); print(torch.version.cuda);"
ENV VLLM_TARGET_DEVICE=cuda
RUN --mount=type=cache,target=/root/.cache/pip pip install --no-build-isolation vllm==${VLLM_VERSION}

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN pip install --no-cache-dir tomli
RUN mkdir -p third_party/library && python3 pip-licenses.py

FROM nvidia/cuda:12.4.1-runtime-ubuntu22.04 AS prod

RUN apt-get update && apt-get upgrade -y && apt-get install python3.10-venv build-essential gcc python3-dev -y && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder huggingfaceserver huggingfaceserver

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
