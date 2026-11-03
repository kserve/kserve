ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install all system dependencies first
RUN apt-get update && apt-get install -y --no-install-recommends python3-dev curl build-essential && \
    if [ "$(uname -m)" = "ppc64le" ]; then apt-get install pkg-config libssl-dev -y; fi && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
# Install uv
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    ln -s /root/.local/bin/uv /usr/local/bin/uv

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Install Python dependencies
COPY storage/pyproject.toml storage/uv.lock storage/

# On ppc64le: patch pyproject.toml to add the ppc64le package index and sources,
# then regenerate uv.lock before syncing.
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
        sed -i \
            -e '/^    "hf-xet/a\    "google-crc32c==1.7.1",' \
            -e '/^    "hf-xet/a\    "pyyaml==6.0.2",' \
            storage/pyproject.toml && \
        printf '%s\n' \
            '' \
            '[tool.uv]' \
            'index-strategy = "unsafe-best-match"' \
            '' \
            '[[tool.uv.index]]' \
            'name = "ppc64le-wheels"' \
            'url = "https://wheels.developerfirst.ibm.com/ppc64le/linux"' \
            'explicit = true' \
            '' \
            '[tool.uv.sources]' \
            'pyyaml = { index = "ppc64le-wheels" }' \
            'google-crc32c = { index = "ppc64le-wheels" }' \
            'hf-xet = { index = "ppc64le-wheels" }' \
            >> storage/pyproject.toml && \
        cd storage && uv lock && \
        cp uv.lock /tmp/storage_ppc64le_uv.lock && \
        cp pyproject.toml /tmp/storage_ppc64le_pyproject.toml; \
    fi

RUN cd storage && uv sync --active --extra confidential --no-cache

COPY storage storage

# On ppc64le: restore the patched pyproject.toml + uv.lock after COPY overwrites them, then clean up
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
        rm -f storage/pyproject.toml storage/uv.lock && \
        cp /tmp/storage_ppc64le_pyproject.toml storage/pyproject.toml && \
        cp /tmp/storage_ppc64le_uv.lock storage/uv.lock && \
        rm -f /tmp/storage_ppc64le_pyproject.toml /tmp/storage_ppc64le_uv.lock; \
    fi

RUN cd storage && uv pip install ".[confidential]" --no-cache

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    gcc \
    libkrb5-dev \
    krb5-config \
    && rm -rf /var/lib/apt/lists/*

# Install Kerberos-related packages
RUN uv pip install --no-cache \
    krbcontext==0.10 \
    hdfs~=2.6.0 \
    requests-kerberos==0.14.0

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

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder storage storage
COPY ./storage-initializer /storage-initializer

RUN chmod +x /storage-initializer/scripts/initializer-entrypoint
RUN mkdir /work
WORKDIR /work

# Set a writable /mnt folder to avoid permission issue on Huggingface download. See https://huggingface.co/docs/hub/spaces-sdks-docker#permissions
RUN chown -R kserve:kserve /mnt
USER 1000
ENTRYPOINT ["/storage-initializer/scripts/initializer-entrypoint"]