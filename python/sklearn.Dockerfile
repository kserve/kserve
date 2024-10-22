ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.8.3
ARG CARGO_HOME=/opt/.cargo/

# Required for building packages for arm64/ppc64le architectures
RUN apt-get update -y && apt-get install -y --no-install-recommends python3-dev build-essential && \
    if [ "$(uname -m)" = "ppc64le" ]; then \
       echo "Installing additional packages for Power architecture" && \
       apt-get install -y libopenblas-dev libssl-dev pkg-config curl libhdf5-dev cmake gfortran && \
       curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs > sh.rustup.rs && \
       export CARGO_HOME=${CARGO_HOME} && sh ./sh.rustup.rs -y && export PATH=$PATH:${CARGO_HOME}/bin && . "${CARGO_HOME}/env"; \
    fi && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

ENV PATH="$PATH:${POETRY_HOME}/bin:${CARGO_HOME}/bin"
# Set up Python virtual environment and install Poetry
RUN python3 -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip3 install poetry==${POETRY_VERSION}

# Activate virtual environment for subsequent commands
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH:${POETRY_HOME}/bin"

# Copy pyproject.toml and poetry.lock for kserve
COPY kserve/pyproject.toml kserve/poetry.lock kserve/

# Install dependencies for kserve, handle architecture-specific tasks
RUN cd kserve && \
    if [ "$(uname -m)" = "ppc64le" ]; then \
      export GRPC_PYTHON_BUILD_SYSTEM_OPENSSL=true; \
    fi && \
    poetry install --no-root --no-interaction --no-cache

# Copy full kserve source and finalize installation
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

# Copy pyproject.toml and poetry.lock for sklearnserver
COPY sklearnserver/pyproject.toml sklearnserver/poetry.lock sklearnserver/

# Install dependencies for sklearnserver
RUN cd sklearnserver && \
    poetry install --no-root --no-interaction --no-cache

# Copy full sklearnserver source and finalize installation
COPY sklearnserver sklearnserver
RUN cd sklearnserver && poetry install --no-interaction --no-cache

# Final production stage
FROM ${BASE_IMAGE} as prod

# Install required runtime libraries
# Install required runtime libraries, but only for ppc64le architectures
RUN apt-get update -y && \
    if [ "$(uname -m)" = "ppc64le" ]; then \
       echo "Installing libraries for Power architecture" && \
       apt-get install -y libopenblas-dev libgomp1; \
    fi && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy third-party dependencies
COPY third_party third_party

# Activate virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Set up a non-root user for production
RUN useradd kserve -m -u 1000 -d /home/kserve

# Copy virtual environment and application code from builder
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder sklearnserver sklearnserver

# Set the user to kserve for execution
USER 1000
ENTRYPOINT ["python", "-m", "sklearnserver"]

