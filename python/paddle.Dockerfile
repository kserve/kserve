# syntax=docker/dockerfile:1.4

ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends curl python3-dev build-essential && \
    if [ "$(uname -m)" = "ppc64le" ]; then apt-get install -y libtiff5-dev libjpeg-dev libopenjp2-7-dev zlib1g-dev \
    libfreetype6-dev liblcms2-dev libwebp-dev tcl8.6-dev tk8.6-dev python3-tk \
    libharfbuzz-dev libfribidi-dev libxcb1-dev; fi && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install uv and ensure it's in PATH
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    ln -s /root/.local/bin/uv /usr/local/bin/uv

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy storage metadata for editable dependency resolution
COPY storage/pyproject.toml storage/uv.lock storage/

# Install kserve dependencies using uv
COPY kserve/pyproject.toml kserve/uv.lock kserve/

# On ppc64le: patch pyproject.toml to add the ppc64le package index and sources,
# then regenerate uv.lock before syncing.
RUN --mount=type=cache,target=/root/.cache/uv \
    if [ "$(uname -m)" = "ppc64le" ]; then \
        sed -i \
            -e '/^index-strategy\s*=.*/a \\' \
            -e '/^index-strategy\s*=.*/a [[tool.uv.index]]' \
            -e '/^index-strategy\s*=.*/a name = "ppc64le-wheels"' \
            -e '/^index-strategy\s*=.*/a url = "https://wheels.developerfirst.ibm.com/ppc64le/linux"' \
            -e '/^index-strategy\s*=.*/a explicit = true' \
            -e '/^\s*"pyasn1>=[^,]*"$/s/"$/",/' \
            -e '/^\s*"pyasn1>=/a\    "httptools==0.6.4",' \
            -e '/^\s*"pyasn1>=/a\    "uvloop==0.21.0",' \
            -e '/^kserve-storage\s*=.*/a grpcio = { index = "ppc64le-wheels" }' \
            -e '/^kserve-storage\s*=.*/a grpcio-tools = { index = "ppc64le-wheels" }' \
            -e '/^kserve-storage\s*=.*/a numpy = { index = "ppc64le-wheels" }' \
            -e '/^kserve-storage\s*=.*/a pandas = { index = "ppc64le-wheels" }' \
            -e '/^kserve-storage\s*=.*/a psutil = { index = "ppc64le-wheels" }' \
            -e '/^kserve-storage\s*=.*/a pyyaml = { index = "ppc64le-wheels" }' \
            -e '/^kserve-storage\s*=.*/a httptools = { index = "ppc64le-wheels" }' \
            -e '/^kserve-storage\s*=.*/a uvloop = { index = "ppc64le-wheels" }' \
            -e '/^kserve-storage\s*=.*/a pillow = { index = "ppc64le-wheels" }' \
            kserve/pyproject.toml && \
        cd kserve && uv lock && \
        cp uv.lock /tmp/kserve_ppc64le_uv.lock && \
        cp pyproject.toml /tmp/kserve_ppc64le_pyproject.toml; \
    fi

RUN --mount=type=cache,target=/root/.cache/uv \
    cd kserve && uv sync --active

COPY kserve kserve

# On ppc64le: restore the patched pyproject.toml + uv.lock after COPY overwrites them
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
        rm -f kserve/pyproject.toml kserve/uv.lock && \
        cp /tmp/kserve_ppc64le_pyproject.toml kserve/pyproject.toml && \
        cp /tmp/kserve_ppc64le_uv.lock kserve/uv.lock && \
        rm -f /tmp/kserve_ppc64le_pyproject.toml /tmp/kserve_ppc64le_uv.lock; \
    fi
    
RUN --mount=type=cache,target=/root/.cache/uv \
    cd kserve && uv sync --active

# Install kserve-storage
COPY storage storage


# On ppc64le: append ppc64le index + sources to storage/pyproject.toml,
# regenerate uv.lock, then sync (same pattern as kserve/lgbserver).
RUN --mount=type=cache,target=/root/.cache/uv \
    if [ "$(uname -m)" = "ppc64le" ]; then \
        sed -i \
            -e '/^    "pyasn1>=[^,]*"$/s/"$/",/' \
            -e '/^    "pyasn1>=/a\    "google-crc32c==1.8.0",' \
            -e '/^    "pyasn1>=/a\    "pyyaml==6.0.2",' \
            storage/pyproject.toml && \
        printf '%s\n' \
            '' \
            '[tool.uv]' \
            'index-strategy = "unsafe-best-match"' \
            'package = true' \
            '' \
            '[build-system]' \
            'requires = ["setuptools>=61.0"]' \
            'build-backend = "setuptools.build_meta"' \
            '' \
            '[[tool.uv.index]]' \
            'name = "ppc64le-wheels"' \
            'url = "https://wheels.developerfirst.ibm.com/ppc64le/linux"' \
            'explicit = true' \
            '' \
            '[tool.uv.sources]' \
            'google-crc32c = { index = "ppc64le-wheels" }' \
            'hf-xet = { index = "ppc64le-wheels" }' \
            'pyyaml = { index = "ppc64le-wheels" }' \
            >> storage/pyproject.toml && \
        cd storage && uv lock && \
        uv sync --active; \
    else \
        cd storage && uv pip install .; \
    fi

RUN --mount=type=cache,target=/root/.cache/uv \
    cd storage && uv pip install .

# Install paddleserver dependencies using uv
COPY paddleserver/pyproject.toml paddleserver/uv.lock paddleserver/

# On ppc64le: paddlepaddle has no PyPI wheel; install it from the ppc64le index.
# Patch pyproject.toml to add the index + source, regenerate uv.lock, then sync.
RUN --mount=type=cache,target=/root/.cache/uv \
    if [ "$(uname -m)" = "ppc64le" ]; then \
        printf '%s\n' \
            '' \
            '[[tool.uv.index]]' \
            'name = "ppc64le-wheels"' \
            'url = "https://wheels.developerfirst.ibm.com/ppc64le/linux"' \
            'explicit = true' \
            '' \
            '[tool.uv.sources]' \
            'paddlepaddle = { index = "ppc64le-wheels" }' \
            'pillow = { index = "ppc64le-wheels" }' \
            >> paddleserver/pyproject.toml && \
        cd paddleserver && uv lock && \
        cp uv.lock /tmp/paddleserver_ppc64le_uv.lock && \
        cp pyproject.toml /tmp/paddleserver_ppc64le_pyproject.toml; \
    fi

RUN --mount=type=cache,target=/root/.cache/uv \
    cd paddleserver && uv sync --active

COPY paddleserver paddleserver

# On ppc64le: restore patched files after COPY overwrites them
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
        cp /tmp/paddleserver_ppc64le_pyproject.toml paddleserver/pyproject.toml && \
        cp /tmp/paddleserver_ppc64le_uv.lock paddleserver/uv.lock && \
        rm -f /tmp/paddleserver_ppc64le_pyproject.toml /tmp/paddleserver_ppc64le_uv.lock; \
    fi

RUN --mount=type=cache,target=/root/.cache/uv \
    cd paddleserver && uv sync --active

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN pip install --no-cache-dir tomli
RUN mkdir -p third_party/library && python3 pip-licenses.py

# ---------- Production image ----------
FROM ${BASE_IMAGE} AS prod

RUN apt-get update && \
    apt-get install -y --no-install-recommends libgomp1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="${VIRTUAL_ENV}/bin:$PATH"

# Create non-root user
RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder storage storage
COPY --from=builder paddleserver paddleserver

USER 1000
ENV PYTHONPATH=/paddleserver
ENTRYPOINT ["python", "-m", "paddleserver"]