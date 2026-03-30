ARG PYTHON_VERSION=3.12
ARG JAVA_VERSION=21
ARG BASE_IMAGE=eclipse-temurin:${JAVA_VERSION}-jdk-noble
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

ARG PYTHON_VERSION
# Install python
RUN --mount=type=cache,sharing=locked,target=/var/cache/apt --mount=type=cache,sharing=locked,target=/var/lib/apt/lists \
    apt-get update && \
    apt-get install -y --no-install-recommends \
    "python${PYTHON_VERSION}" \
    "python${PYTHON_VERSION}-dev" \
    "python${PYTHON_VERSION}-venv" \
    python3-pip \
    gcc build-essential

# Install uv
COPY --from=ghcr.io/astral-sh/uv:0.7 /uv /usr/local/bin/uv

# Setup virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy storage metadata for editable dependency resolution
COPY storage/pyproject.toml storage/uv.lock storage/

# Install dependencies for kserve using uv
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN --mount=type=cache,target=/root/.cache/uv cd kserve && uv sync --active

COPY kserve kserve
RUN --mount=type=cache,target=/root/.cache/uv cd kserve && uv sync --active

# Copy and install dependencies for kserve-storage using uv
COPY storage/pyproject.toml storage/uv.lock storage/
RUN --mount=type=cache,target=/root/.cache/uv cd storage && uv sync --active

COPY storage storage
RUN --mount=type=cache,target=/root/.cache/uv cd storage && uv pip install .

# Install dependencies for pmmlserver using uv
COPY pmmlserver/pyproject.toml pmmlserver/uv.lock pmmlserver/
RUN --mount=type=cache,target=/root/.cache/uv cd pmmlserver && uv sync --active

COPY pmmlserver pmmlserver
RUN --mount=type=cache,target=/root/.cache/uv cd pmmlserver && uv sync --active

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
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
