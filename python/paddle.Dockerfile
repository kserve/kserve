# syntax=docker/dockerfile:1.4

ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install system dependencies
RUN --mount=type=cache,sharing=locked,target=/var/cache/apt --mount=type=cache,sharing=locked,target=/var/lib/apt/lists \
    apt-get update && apt-get install -y --no-install-recommends python3-dev build-essential

# Install uv and ensure it's in PATH
COPY --from=ghcr.io/astral-sh/uv:0.7 /uv /usr/local/bin/uv

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy storage metadata for editable dependency resolution
COPY storage/pyproject.toml storage/uv.lock storage/

# Install kserve dependencies using uv
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN --mount=type=cache,target=/root/.cache/uv cd kserve && uv sync --active

COPY kserve kserve
RUN --mount=type=cache,target=/root/.cache/uv cd kserve && uv sync --active

# Copy and install dependencies for kserve-storage using uv
COPY storage/pyproject.toml storage/uv.lock storage/
RUN --mount=type=cache,target=/root/.cache/uv cd storage && uv sync --active

COPY storage storage
RUN --mount=type=cache,target=/root/.cache/uv cd storage && uv pip install .

# Install paddleserver dependencies using uv
COPY paddleserver/pyproject.toml paddleserver/uv.lock paddleserver/
RUN --mount=type=cache,target=/root/.cache/uv cd paddleserver && uv sync --active

COPY paddleserver paddleserver
RUN --mount=type=cache,target=/root/.cache/uv cd paddleserver && uv sync --active

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
RUN mkdir -p third_party/library && python3 pip-licenses.py

# ---------- Production image ----------
FROM ${BASE_IMAGE} AS prod

RUN apt-get update && \
    apt-get install -y --no-install-recommends libgomp1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="${VIRTUAL_ENV}/bin:$PATH"

# Create non-root user
RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder storage storage
COPY --from=builder paddleserver paddleserver

USER 1000
ENV PYTHONPATH=/paddleserver
ENTRYPOINT ["python", "-m", "paddleserver"]
