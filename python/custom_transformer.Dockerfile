ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install system dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    curl \
    build-essential \
    python3-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install uv and ensure it's in PATH
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    ln -s /root/.local/bin/uv /usr/local/bin/uv

# Create virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv ${VIRTUAL_ENV}
ENV PATH="${VIRTUAL_ENV}/bin:$PATH"

# Install kserve dependencies
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv sync --no-cache
COPY kserve kserve

# Install custom_transformer dependencies
COPY custom_transformer/pyproject.toml custom_transformer/uv.lock custom_transformer/
RUN cd custom_transformer && uv sync --no-cache
COPY custom_transformer custom_transformer

# ---------- Production image ----------
FROM ${BASE_IMAGE} AS prod

ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="${VIRTUAL_ENV}/bin:$PATH"

# Copy any external shared resources
COPY third_party third_party

# Create non-root user
RUN useradd kserve -m -u 1000 -d /home/kserve

# Copy venv and code from builder stage
COPY --from=builder --chown=kserve:kserve ${VENV_PATH} ${VENV_PATH}
COPY --from=builder kserve kserve
COPY --from=builder custom_transformer custom_transformer

USER 1000
ENTRYPOINT ["python", "-m", "custom_transformer.model"]
