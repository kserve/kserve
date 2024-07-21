ARG BASE_IMAGE=nvcr.io/nvidia/tritonserver
ARG BASE_IMAGE_TAG=24.06-py3
ARG VENV_PATH=prod_venv
ARG WORKSPACE=/kserve

FROM ${BASE_IMAGE}:${BASE_IMAGE_TAG} AS builder

RUN apt-get update -y && apt-get install gcc python3-venv python3-dev -y --no-install-recommends && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.7.1

RUN --mount=type=cache,target=/root/.cache python3 -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip3 install poetry==${POETRY_VERSION}
ENV POETRY_CACHE_DIR=/root/.cache/pypoetry
ENV PATH="$PATH:${POETRY_HOME}/bin"

ARG WORKSPACE
WORKDIR ${WORKSPACE}

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${WORKSPACE}/${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN --mount=type=cache,target=/root/.cache cd kserve && poetry install --no-root --no-interaction
COPY kserve kserve
RUN --mount=type=cache,target=/root/.cache cd kserve && poetry install --no-interaction

COPY huggingfaceserver/pyproject.toml huggingfaceserver/
RUN --mount=type=cache,target=/root/.cache cd huggingfaceserver && poetry install --no-root --no-interaction
COPY huggingfaceserver huggingfaceserver
RUN --mount=type=cache,target=/root/.cache cd huggingfaceserver && poetry install --no-interaction

# Install tritonserver In-Process API
RUN --mount=type=cache,target=/root/.cache find /opt/tritonserver/python -maxdepth 1 -type f -name \
    "tritonserver-*.whl" | xargs -I {} pip3 install --force-reinstall --upgrade {}[gpu]


FROM ${BASE_IMAGE}:${BASE_IMAGE_TAG} AS prod

RUN apt-get update -y && apt-get install python3-venv -y && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

ARG WORKSPACE
WORKDIR ${WORKSPACE}

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${WORKSPACE}/${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1001 -d /home/kserve

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder ${WORKSPACE}/kserve kserve
COPY --from=builder ${WORKSPACE}/huggingfaceserver huggingfaceserver

# Set a writable Hugging Face home folder to avoid permission issue. See https://github.com/kserve/kserve/issues/3562
ENV HF_HOME="/tmp/huggingface"
# https://huggingface.co/docs/safetensors/en/speed#gpu-benchmark
ENV SAFETENSORS_FAST_GPU="1"
# https://huggingface.co/docs/huggingface_hub/en/package_reference/environment_variables#hfhubdisabletelemetry
ENV HF_HUB_DISABLE_TELEMETRY="1"

USER 1001
ENTRYPOINT ["python3", "-m", "huggingfaceserver", "--backend", "triton"]
