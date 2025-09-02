ARG PYTHON_VERSION=3.11
ARG JAVA_VERSION=21
ARG BASE_IMAGE=openjdk:${JAVA_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

ARG PYTHON_VERSION
# Install python
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    "python${PYTHON_VERSION}" \
    "python${PYTHON_VERSION}-dev" \
    "python${PYTHON_VERSION}-venv" \
    python3-pip \
    curl \
    gcc build-essential && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install uv
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
ENV PATH="/root/.local/bin:$PATH"

# Setup virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Install dependencies for kserve using uv
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv sync --active --no-cache

COPY kserve kserve
RUN cd kserve && uv sync --active --no-cache

# Copy and install dependencies for kserve-storage using uv
COPY storage/pyproject.toml storage/uv.lock storage/
RUN cd storage && uv sync --active --no-cache

COPY storage storage
RUN cd storage && uv pip install . --no-cache

# Install dependencies for pmmlserver using uv
COPY pmmlserver/pyproject.toml pmmlserver/uv.lock pmmlserver/
RUN cd pmmlserver && uv sync --active --no-cache

COPY pmmlserver pmmlserver
RUN cd pmmlserver && uv sync --active --no-cache

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN uv pip install --no-cache-dir tomli
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
RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder storage storage
COPY --from=builder pmmlserver pmmlserver

USER 1000
ENV PYTHONPATH=/pmmlserver
ENTRYPOINT ["python3", "-m", "pmmlserver"]
