ARG PYTHON_VERSION=3.12
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends python3-dev curl build-essential && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install uv
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    ln -s /root/.local/bin/uv /usr/local/bin/uv

# Create virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# ========== Install kserve dependencies ==========
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv sync --active --no-cache

COPY kserve kserve
RUN cd kserve && uv sync --active --no-cache

# ========== Install kserve storage dependencies ==========
COPY storage/pyproject.toml storage/uv.lock storage/
RUN cd storage && uv sync --active --no-cache

COPY storage storage
RUN cd storage && uv pip install . --no-cache

# ========== Install autogluonserver dependencies ==========
COPY autogluonserver autogluonserver
RUN cd autogluonserver && uv sync --active --no-cache

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN pip install --no-cache-dir tomli
RUN mkdir -p third_party/library && python3 pip-licenses.py


# =================== Final stage ===================
FROM ${BASE_IMAGE} AS prod

# Runtime deps for AutoGluon backends (LightGBM, XGBoost, etc.) that use OpenMP
RUN apt-get update && apt-get install -y --no-install-recommends libgomp1 && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder storage storage
COPY --from=builder autogluonserver autogluonserver

USER 1000
ENV PYTHONPATH=/autogluonserver
ENTRYPOINT ["python", "-m", "autogluonserver"]
