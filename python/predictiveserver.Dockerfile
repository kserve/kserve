ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
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

# ========== Copy all source code first ==========
COPY kserve kserve
COPY storage storage
COPY sklearnserver sklearnserver
COPY xgbserver xgbserver
COPY lgbserver lgbserver
COPY predictiveserver predictiveserver

# ========== Install everything through predictiveserver ==========
# predictiveserver depends on all other packages, so installing it will install everything
RUN cd predictiveserver && uv pip install --no-cache .

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
# TODO: Remove this when upgrading to python 3.11+
RUN pip install --no-cache-dir tomli
RUN mkdir -p third_party/library && python3 pip-licenses.py


# =================== Final stage ===================
FROM ${BASE_IMAGE} AS prod

# Install runtime dependencies (libgomp for lightgbm, xgboost, etc.)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder --chown=kserve:kserve kserve kserve
COPY --from=builder --chown=kserve:kserve storage storage
COPY --from=builder --chown=kserve:kserve sklearnserver sklearnserver
COPY --from=builder --chown=kserve:kserve xgbserver xgbserver
COPY --from=builder --chown=kserve:kserve lgbserver lgbserver
COPY --from=builder --chown=kserve:kserve predictiveserver predictiveserver

USER 1000
ENV PYTHONPATH=/predictiveserver:/sklearnserver:/xgbserver:/lgbserver
LABEL io.kserve.runtime="predictiveserver" \
      io.kserve.frameworks="sklearn,lightgbm,xgboost"
ENTRYPOINT ["python", "-m", "predictiveserver"]
