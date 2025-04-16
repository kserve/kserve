ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install required system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends python3-dev build-essential curl && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install uv (Astral)
RUN curl -Ls https://astral.sh/uv/install.sh | sh

# Make uv available
ENV PATH="$HOME/.cargo/bin:$PATH"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv ${VIRTUAL_ENV}
ENV PATH="${VIRTUAL_ENV}/bin:$PATH"

# Install dependencies for kserve using uv
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv pip install -r uv.lock
COPY kserve kserve

# Install dependencies for lgbserver using uv
COPY lgbserver/pyproject.toml lgbserver/uv.lock lgbserver/
RUN cd lgbserver && uv pip install -r uv.lock
COPY lgbserver lgbserver

# ---------- Production image ----------
FROM ${BASE_IMAGE} AS prod

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="${VIRTUAL_ENV}/bin:$PATH"

# Create non-root user
RUN useradd kserve -m -u 1000 -d /home/kserve

# Copy venv and code from builder stage
COPY --from=builder --chown=kserve:kserve ${VIRTUAL_ENV} ${VIRTUAL_ENV}
COPY --from=builder kserve kserve
COPY --from=builder lgbserver lgbserver

USER 1000
ENTRYPOINT ["python", "-m", "lgbserver"]
