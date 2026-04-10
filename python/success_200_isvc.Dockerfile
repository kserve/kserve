ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

RUN --mount=type=cache,sharing=locked,target=/var/cache/apt --mount=type=cache,sharing=locked,target=/var/lib/apt/lists \
    apt-get update && apt-get install -y gcc python3-dev

# Install uv
COPY --from=ghcr.io/astral-sh/uv:0.7 /uv /usr/local/bin/uv

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy only storage metadata for kserve's editable path dep resolution
COPY storage/pyproject.toml storage/uv.lock storage/

COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN --mount=type=cache,target=/root/.cache/uv cd kserve && uv sync --active

COPY kserve kserve
RUN --mount=type=cache,target=/root/.cache/uv cd kserve && uv sync --active

COPY test_resources/graph/success_200_isvc/pyproject.toml test_resources/graph/success_200_isvc/uv.lock test_resources/graph/success_200_isvc/
RUN --mount=type=cache,target=/root/.cache/uv cd test_resources/graph/success_200_isvc && uv sync --active
COPY test_resources/graph/success_200_isvc test_resources/graph/success_200_isvc
RUN --mount=type=cache,target=/root/.cache/uv cd test_resources/graph/success_200_isvc && uv sync --active

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
RUN mkdir -p third_party/library && python3 pip-licenses.py

FROM ${BASE_IMAGE} AS prod

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder test_resources/graph/success_200_isvc success_200_isvc

USER 1000
ENTRYPOINT ["python", "-m", "success_200_isvc.model"]
