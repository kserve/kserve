ARG PYTHON_VERSION=3.12
ARG JAVA_VERSION=21
ARG BASE_IMAGE=eclipse-temurin:${JAVA_VERSION}-jdk-noble
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

ARG PYTHON_VERSION
# Install python
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
    apt-get update && \
    apt-get install -y --no-install-recommends \
    "python${PYTHON_VERSION}" \
    "python${PYTHON_VERSION}-dev" \
    "python${PYTHON_VERSION}-venv" \
    python3-pip \
    curl \
    wget \
    libssl-dev \
    pkg-config \
    libopenblas-dev \
    apt-transport-https \
    gpg \
    gcc build-essential \
    libjpeg-dev \
    zlib1g-dev \
    libtiff-dev \
    libfreetype6-dev \
    liblcms2-dev \
    libwebp-dev \
    libopenjp2-7-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*; \
    else \
    apt-get update && \
    apt-get install -y --no-install-recommends \
    "python${PYTHON_VERSION}" \
    "python${PYTHON_VERSION}-dev" \
    "python${PYTHON_VERSION}-venv" \
    python3-pip \
    curl \
    gcc build-essential && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*; \
    fi

# Install uv
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    ln -s /root/.local/bin/uv /usr/local/bin/uv

# Setup virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy storage metadata for editable dependency resolution
COPY storage/pyproject.toml storage/uv.lock storage/

# Install dependencies for kserve using uv
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
            -e '/^kserve-storage\s*=.*/a scikit-learn = { index = "ppc64le-wheels" }' \
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
            -e '/^    "pyasn1>=/a\    "google-crc32c==1.7.1",' \
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

# Install dependencies for pmmlserver using uv
COPY pmmlserver/pyproject.toml pmmlserver/uv.lock pmmlserver/

# On ppc64le: add transitive deps that need wheel pinning, append the ppc64le
# index + sources to pmmlserver/pyproject.toml, then regenerate uv.lock.
RUN --mount=type=cache,target=/root/.cache/uv \
    if [ "$(uname -m)" = "ppc64le" ]; then \
        sed -i \
            -e '/^\s*"setuptools<71\.0\.0,>=70\.0\.0",$/a\    "pyjnius==1.6.1",' \
            -e '/^\s*"setuptools<71\.0\.0,>=70\.0\.0",$/a\    "jpype1==1.5.2",' \
            -e '/^\s*"setuptools<71\.0\.0,>=70\.0\.0",$/a\    "hf-xet==1.5.1",' \
            -e '/^\s*"setuptools<71\.0\.0,>=70\.0\.0",$/a\    "google-crc32c==1.7.1",' \
            pmmlserver/pyproject.toml && \
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
            'pyjnius = { index = "ppc64le-wheels" }' \
            'hf-xet = { index = "ppc64le-wheels" }' \
            'google-crc32c = { index = "ppc64le-wheels" }' \
            'jpype1 = { index = "ppc64le-wheels" }' \
            >> pmmlserver/pyproject.toml && \
        cd pmmlserver && uv lock && \
        cp uv.lock /tmp/pmmlserver_ppc64le_uv.lock && \
        cp pyproject.toml /tmp/pmmlserver_ppc64le_pyproject.toml; \
    fi

RUN --mount=type=cache,target=/root/.cache/uv \
    cd pmmlserver && uv sync --active

COPY pmmlserver pmmlserver

# On ppc64le: restore the patched pyproject.toml + uv.lock after COPY overwrites them, then clean up
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
        rm -f pmmlserver/pyproject.toml pmmlserver/uv.lock && \
        cp /tmp/pmmlserver_ppc64le_pyproject.toml pmmlserver/pyproject.toml && \
        cp /tmp/pmmlserver_ppc64le_uv.lock pmmlserver/uv.lock && \
        rm -f /tmp/pmmlserver_ppc64le_pyproject.toml /tmp/pmmlserver_ppc64le_uv.lock; \
    fi

RUN --mount=type=cache,target=/root/.cache/uv \
    cd pmmlserver && uv sync --active

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN --mount=type=cache,target=/root/.cache/uv \
    uv pip install tomli
RUN mkdir -p third_party/library && python3 pip-licenses.py

# ---------- Production image ----------
FROM ${BASE_IMAGE} AS prod

ARG PYTHON_VERSION
# Install python
RUN apt-get update && \
    apt-get install -y --no-install-recommends "python${PYTHON_VERSION}" && \
    ln -s /usr/bin/python${PYTHON_VERSION} /usr/bin/python3 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Create non-root user
RUN useradd kserve -m -u 1001 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder storage storage
COPY --from=builder pmmlserver pmmlserver

USER 1001
ENV PYTHONPATH=/pmmlserver
ENTRYPOINT ["python3", "-m", "pmmlserver"]