ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install system deps and uv
RUN apt-get update && \
    apt-get install -y --no-install-recommends curl build-essential python3-dev && \
    curl -Ls https://astral.sh/uv/install.sh | sh && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Add uv to PATH (default installed at ~/.cargo/bin)
ENV PATH="$HOME/.cargo/bin:$PATH"

# Set up Python virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# ------------------ Install kserve ------------------
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv pip install -r uv.lock
COPY kserve kserve

# ------------------ Install custom_model ------------------
COPY custom_model/pyproject.toml custom_model/uv.lock custom_model/
RUN cd custom_model && uv pip install -r uv.lock
COPY custom_model custom_model

# ------------------ Final production image ------------------
FROM ${BASE_IMAGE} AS prod

ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy third-party dependencies
COPY third_party third_party

# Create non-root user
RUN useradd kserve -m -u 1000 -d /home/kserve

# Copy environment and source code from builder stage
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder custom_model custom_model

USER 1000
ENTRYPOINT ["python", "-m", "custom_model.model"]
