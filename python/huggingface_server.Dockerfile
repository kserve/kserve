ARG BASE_IMAGE=nvidia/cuda:12.1.0-devel-ubuntu22.04
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} as builder

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.7.1

RUN apt-get update -y && apt-get install gcc python3.10-venv python3-dev -y --no-install-recommends && \
    apt-get clean && rm -rf /var/lib/apt/lists/*
RUN python3 -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip3 install poetry==${POETRY_VERSION}
ENV PATH="$PATH:${POETRY_HOME}/bin"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

COPY huggingfaceserver/pyproject.toml huggingfaceserver/poetry.lock huggingfaceserver/
RUN cd huggingfaceserver && poetry install --no-root --no-interaction --no-cache --extras "vllm"
COPY huggingfaceserver huggingfaceserver
RUN cd huggingfaceserver && poetry install --no-interaction --no-cache --extras "vllm"


FROM nvidia/cuda:12.1.0-base-ubuntu22.04 as prod

RUN apt-get update && apt-get install python3.10-venv -y --no-install-recommends && apt-get clean && \
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
USER 1000
ENTRYPOINT ["python3", "-m", "huggingfaceserver"]

