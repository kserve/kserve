ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install necessary dependencies for building Python packages
RUN apt-get update && apt-get install -y --no-install-recommends python3-dev build-essential gcc libgomp1 && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install uv (Astral) for dependency management
RUN curl -Ls https://astral.sh/uv/install.sh | sh
ENV PATH="$HOME/.cargo/bin:$PATH"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv ${VIRTUAL_ENV}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy and install dependencies for kserve using uv
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv pip install -r uv.lock
COPY kserve kserve
RUN cd kserve && uv pip install -r uv.lock

# Copy and install dependencies for xgbserver using uv
COPY xgbserver/pyproject.toml xgbserver/uv.lock xgbserver/
RUN cd xgbserver && uv pip install -r uv.lock
COPY xgbserver xgbserver
RUN cd xgbserver && uv pip install -r uv.lock

# ---------- Production Image ----------
FROM ${BASE_IMAGE} AS prod

COPY third_party third_party

# Install necessary runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends libgomp1 && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Create a non-root user for running the application
RUN useradd kserve -m -u 1000 -d /home/kserve

# Copy virtual env and code from builder stage
COPY --from=builder --chown=kserve:kserve ${VIRTUAL_ENV} ${VIRTUAL_ENV}
COPY --from=builder kserve kserve
COPY --from=builder xgbserver xgbserver

USER 1000
ENTRYPOINT ["python", "-m", "xgbserver"]
