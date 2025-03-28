ARG BASE_IMAGE=ubuntu:22.04
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder


# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.8.3

# Install vllm
ARG VLLM_VERSION=v0.8.1

RUN apt-get update -y && apt-get install -y \
    gcc python3.10-venv python3-dev python3-pip \
    ffmpeg libsm6 libxext6 libgl1 libnuma1 git \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Set up Poetry for dependency management
RUN python3 -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip3 install poetry==${POETRY_VERSION}
ENV PATH="$PATH:${POETRY_HOME}/bin"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Install KServe and Hugging Face Server dependencies
COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

COPY huggingfaceserver/pyproject.toml huggingfaceserver/poetry.lock huggingfaceserver/health_check.py huggingfaceserver/
RUN cd huggingfaceserver && poetry install --no-root --no-interaction 
COPY huggingfaceserver huggingfaceserver
RUN cd huggingfaceserver && poetry install --no-interaction --no-cache

# Clone vllm
# Install Python build tools and other dependencies then build vllm from source
# temporary fix for dependency conflict for https://github.com/vllm-project/vllm/issues/11398
WORKDIR /vllm
RUN git clone --branch $VLLM_VERSION --depth 1 https://github.com/vllm-project/vllm.git . && \
    pip install --upgrade pip && \
    pip install -r requirements/build.txt --extra-index-url https://download.pytorch.org/whl/cpu && \
    PIP_EXTRA_INDEX_URL="https://download.pytorch.org/whl/cpu" VLLM_TARGET_DEVICE="openvino" python -m pip install -v .

FROM ${BASE_IMAGE} AS prod

RUN apt-get update && apt-get upgrade -y && apt-get install python3.10-venv libnuma1 build-essential gcc python3-dev -y && apt-get clean && \
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
# https://huggingface.co/docs/huggingface_hub/en/package_reference/environment_variables#hfhubdisabletelemetry
ENV HF_HUB_DISABLE_TELEMETRY="1"
# https://github.com/vllm-project/vllm/issues/6152
# Set the multiprocess method to spawn to avoid issues with cuda initialization for `mp` executor backend.
ENV VLLM_WORKER_MULTIPROC_METHOD="spawn"

USER 1000
ENTRYPOINT ["python3", "-m", "huggingfaceserver"]
