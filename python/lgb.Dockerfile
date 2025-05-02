ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends python3-dev curl build-essential && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install uv and ensure it's in PATH
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    ln -s /root/.local/bin/uv /usr/local/bin/uv

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Install dependencies for kserve using uv
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv sync --active --no-cache
COPY kserve kserve

# Install dependencies for lgbserver using uv
COPY lgbserver/pyproject.toml lgbserver/uv.lock lgbserver/
RUN cd lgbserver && uv sync --active --no-cache
COPY lgbserver lgbserver
RUN cd lgbserver && poetry install --no-interaction --no-cache

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN pip install --no-cache-dir tomli
RUN mkdir -p third_party/library && python3 pip-licenses.py

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

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder lgbserver lgbserver

USER 1000
ENTRYPOINT ["python", "-m", "lgbserver"]
