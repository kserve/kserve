ARG BASE_IMAGE=nvidia/cuda:12.4.1-devel-ubuntu22.04
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder


# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.8.3

# Install vllm
ARG VLLM_VERSION=0.6.3

RUN apt-get update -y && apt-get install -y \
    gcc python3.10-venv python3-dev python3-pip \
    gcc-12 g++-12 libnuma-dev libnuma1 libtcmalloc-minimal4 numactl git \
    && apt-get clean && rm -rf /var/lib/apt/lists/*
    
RUN update-alternatives --install /usr/bin/gcc gcc /usr/bin/gcc-12 10 --slave /usr/bin/g++ g++ /usr/bin/g++-12

# Set up Poetry for dependency management
RUN python3 -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip install poetry==${POETRY_VERSION}
ENV PATH="$PATH:${POETRY_HOME}/bin"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# https://intel.github.io/intel-extension-for-pytorch/cpu/latest/tutorials/performance_tuning/tuning_guide.html
# intel-openmp provides additional performance improvement vs. openmp
# tcmalloc provides better memory allocation efficiency, e.g, holding memory in caches to speed up access of commonly-used objects.
RUN pip install intel-openmp

ENV LD_PRELOAD="/usr/lib/x86_64-linux-gnu/libtcmalloc_minimal.so.4:$VIRTUAL_ENV/lib/libiomp5.so"

RUN pip install intel_extension_for_pytorch==2.5.0

# Install Python build tools and other dependencies
RUN pip install --upgrade pip && \
    pip install "cmake>=3.26" ninja packaging "setuptools>=61" "setuptools-scm>=8" numpy "torch==2.5.1" wheel jinja2

# Install KServe and Hugging Face Server dependencies
COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

COPY huggingfaceserver/pyproject.toml huggingfaceserver/poetry.lock huggingfaceserver/
RUN cd huggingfaceserver && poetry install --no-root --no-interaction --no-cache
COPY huggingfaceserver huggingfaceserver
RUN cd huggingfaceserver && poetry install --no-interaction --no-cache

# Support for building with non-AVX512 vLLM: docker build --build-arg VLLM_CPU_DISABLE_AVX512="true" ...
ARG VLLM_CPU_DISABLE_AVX512
ENV VLLM_CPU_DISABLE_AVX512=${VLLM_CPU_DISABLE_AVX512}

# Clone and build vllm from source
WORKDIR /vllm
RUN git clone https://github.com/vllm-project/vllm.git . && \
    pip install -v -r requirements-cpu.txt --extra-index-url https://download.pytorch.org/whl/cpu && \
    VLLM_TARGET_DEVICE=cpu python3 setup.py bdist_wheel && \
    pip install dist/*.whl && \
    rm -rf dist

FROM nvidia/cuda:12.4.1-runtime-ubuntu22.04 AS prod

RUN apt-get update && apt-get upgrade -y && apt-get install python3.10-venv libnuma1 build-essential gcc python3-dev libtcmalloc-minimal4 -y && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

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
ENV LD_PRELOAD="/usr/lib/x86_64-linux-gnu/libtcmalloc_minimal.so.4:$VIRTUAL_ENV/lib/libiomp5.so"
ENV VLLM_CPU_DISABLE_AVX512=${VLLM_CPU_DISABLE_AVX512}

USER 1000
ENTRYPOINT ["python3", "-m", "huggingfaceserver"]
