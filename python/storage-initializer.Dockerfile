ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install necessary dependencies for building Python packages
RUN apt-get update && apt-get install -y --no-install-recommends python3-dev build-essential gcc libkrb5-dev krb5-config curl && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install uv (Astral) for dependency management
RUN curl -Ls https://astral.sh/uv/install.sh | sh
ENV PATH="$HOME/.cargo/bin:$PATH"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv ${VIRTUAL_ENV}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy and install dependencies for kserve
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv pip install -r uv.lock
COPY kserve kserve
RUN cd kserve && uv pip install -r uv.lock

# Install dependencies for krbcontext, hdfs, and requests-kerberos
RUN pip install --no-cache-dir krbcontext==0.10 hdfs~=2.6.0 requests-kerberos==0.14.0

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN pip install --no-cache-dir tomli
RUN mkdir -p third_party/library && python3 pip-licenses.py

FROM ${BASE_IMAGE} AS prod

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Create a non-root user for running the application
RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder ./storage-initializer /storage-initializer

# Set permissions for entrypoint and working directories
RUN chmod +x /storage-initializer/scripts/initializer-entrypoint
RUN mkdir /work
WORKDIR /work

# Set a writable /mnt folder to avoid permission issue on Huggingface download.
RUN chown -R kserve:kserve /mnt

USER 1000
ENTRYPOINT ["/storage-initializer/scripts/initializer-entrypoint"]
