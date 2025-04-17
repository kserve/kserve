ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install system dependencies and uv
RUN apt-get update && \
    apt-get install -y --no-install-recommends curl build-essential python3-dev && \
    curl -Ls https://astral.sh/uv/install.sh | sh && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Add uv to PATH (uv installs to ~/.cargo/bin)
ENV PATH="$HOME/.cargo/bin:$PATH"

# Create Python virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# ------------------ Install kserve ------------------
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv pip install -r uv.lock
COPY kserve kserve

# ------------------ Install custom_transformer ------------------
COPY custom_transformer/pyproject.toml custom_transformer/uv.lock custom_transformer/
RUN cd custom_transformer && uv pip install -r uv.lock
COPY custom_transformer custom_transformer

# ------------------ Final Production Image ------------------
FROM ${BASE_IMAGE} AS prod

ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY third_party third_party

# Create non-root user
RUN useradd kserve -m -u 1000 -d /home/kserve

# Copy the virtualenv and project source from builder
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder custom_transformer custom_transformer

USER 1000
ENTRYPOINT ["python", "-m", "custom_transformer.model_grpc"]
