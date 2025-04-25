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

# Install uv
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    ln -s /root/.local/bin/uv /usr/local/bin/uv

# Set up and activate virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# ------------------ kserve deps ------------------
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv sync
COPY kserve kserve

# ------------------ custom_model deps ------------------
COPY custom_model/pyproject.toml custom_model/uv.lock custom_model/
RUN cd custom_model && uv sync
COPY custom_model custom_model

# ------------------ Final stage ------------------
FROM ${BASE_IMAGE} AS prod

ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy third-party non-Python dependencies
COPY third_party third_party

# Create and switch to a non-root user
RUN useradd kserve -m -u 1000 -d /home/kserve

# Copy venv and application code from builder stage
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder custom_model custom_model

USER 1000
ENTRYPOINT ["python", "-m", "custom_model.model_grpc"]
