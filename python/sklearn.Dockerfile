ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install build tools and uv
RUN apt-get update && \
    apt-get install -y --no-install-recommends curl build-essential python3-dev && \
    curl -Ls https://astral.sh/uv/install.sh | sh && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Add uv to PATH
ENV PATH="$HOME/.cargo/bin:$PATH"

# Create virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# ========== Install kserve dependencies ==========
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv pip install -r uv.lock

# Copy kserve source code after installing deps (for layer caching)
COPY kserve kserve

# ========== Install sklearnserver dependencies ==========
COPY sklearnserver/pyproject.toml sklearnserver/uv.lock sklearnserver/
RUN cd sklearnserver && uv pip install -r uv.lock

# Copy sklearnserver source code after installing deps (for layer caching)
COPY sklearnserver sklearnserver

# =================== Final stage ===================
FROM ${BASE_IMAGE} AS prod

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

# Copy vendored dependencies, source, and third-party files
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder sklearnserver sklearnserver
COPY third_party third_party

USER 1000
ENTRYPOINT ["python", "-m", "sklearnserver"]
