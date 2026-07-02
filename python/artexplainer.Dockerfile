ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Required for building packages for arm64 arch
RUN apt-get update && apt-get install -y --no-install-recommends curl python3-dev build-essential && \
    if [ "$(uname -m)" = "ppc64le" ]; then apt-get install pkg-config libssl-dev gcc gfortran cmake pkg-config libssl-dev libopenblas-dev libjpeg-dev libhdf5-dev wget -y; fi && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

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

# ------------------ kserve deps ------------------
COPY kserve/pyproject.toml kserve/uv.lock kserve/

# On ppc64le: patch pyproject.toml to add the ppc64le package index and sources,
# then regenerate uv.lock before syncing.
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
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
            kserve/pyproject.toml && \
        cd kserve && uv lock && \
        cp uv.lock /tmp/kserve_ppc64le_uv.lock && \
        cp pyproject.toml /tmp/kserve_ppc64le_pyproject.toml
    fi

RUN cd kserve && uv sync --active --no-cache

COPY kserve kserve

# On ppc64le: restore the patched pyproject.toml + uv.lock after COPY overwrites them
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
        rm -f kserve/pyproject.toml kserve/uv.lock && \
        cp /tmp/kserve_ppc64le_pyproject.toml kserve/pyproject.toml && \
        cp /tmp/kserve_ppc64le_uv.lock kserve/uv.lock && \
        rm -f /tmp/kserve_ppc64le_pyproject.toml /tmp/kserve_ppc64le_uv.lock
    fi

RUN cd kserve && uv sync --active --no-cache

# ------------------ artexplainer deps ------------------
COPY artexplainer/pyproject.toml artexplainer/uv.lock artexplainer/

# On ppc64le: patch pyproject.toml to add extra direct dependencies and the ppc64le package index/sources,
# then regenerate uv.lock before syncing.
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
        sed -i \
            -e '/^    "h5py/a\    "scikit-learn==1.6.1",' \
            -e '/^    "h5py/a\    "scipy==1.15.2",' \
            -e '/^    "h5py/a\    "ml-dtypes==0.5.1",' \
            artexplainer/pyproject.toml && \
        printf '%s\n' \
            '' \
            '[[tool.uv.index]]' \
            'name = "ppc64le-wheels"' \
            'url = "https://wheels.developerfirst.ibm.com/ppc64le/linux"' \
            'explicit = true' \
            '' \
            '[tool.uv.sources]' \
            'grpcio = { index = "ppc64le-wheels" }' \
            'grpcio-tools = { index = "ppc64le-wheels" }' \
            'scipy = { index = "ppc64le-wheels" }' \
            'numpy = { index = "ppc64le-wheels" }' \
            'pandas = { index = "ppc64le-wheels" }' \
            'psutil = { index = "ppc64le-wheels" }' \
            'pyyaml = { index = "ppc64le-wheels" }' \
            'uvloop = { index = "ppc64le-wheels" }' \
            'httptools = { index = "ppc64le-wheels" }' \
            'scikit-learn = { index = "ppc64le-wheels" }' \
            'pillow = { index = "ppc64le-wheels" }' \
            'h5py = { index = "ppc64le-wheels" }' \
            'ml-dtypes = { index = "ppc64le-wheels" }' \
            >> artexplainer/pyproject.toml && \
        cd artexplainer && uv lock && \
        cp uv.lock /tmp/artexplainer_ppc64le_uv.lock && \
        cp pyproject.toml /tmp/artexplainer_ppc64le_pyproject.toml
    fi

RUN cd artexplainer && uv sync --active --no-cache

COPY artexplainer artexplainer

# On ppc64le: restore the patched pyproject.toml + uv.lock after COPY overwrites them, then clean up
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
        rm -f artexplainer/pyproject.toml artexplainer/uv.lock && \
        cp /tmp/artexplainer_ppc64le_pyproject.toml artexplainer/pyproject.toml && \
        cp /tmp/artexplainer_ppc64le_uv.lock artexplainer/uv.lock && \
        rm -f /tmp/artexplainer_ppc64le_pyproject.toml /tmp/artexplainer_ppc64le_uv.lock
    fi

RUN cd artexplainer && uv sync --active --no-cache

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN pip install --no-cache-dir tomli
RUN mkdir -p third_party/library && python3 pip-licenses.py


# ------------------ Production stage ------------------
FROM ${BASE_IMAGE} AS prod

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder artexplainer artexplainer

USER 1000
ENV PYTHONPATH=/artexplainer
ENTRYPOINT ["python", "-m", "artserver"]
