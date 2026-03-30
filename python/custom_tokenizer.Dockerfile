ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install system dependencies
RUN --mount=type=cache,sharing=locked,target=/var/cache/apt --mount=type=cache,sharing=locked,target=/var/lib/apt/lists \
    apt-get update && apt-get install -y --no-install-recommends build-essential python3-dev

# Install uv and ensure it's in PATH
COPY --from=ghcr.io/astral-sh/uv:0.7 /uv /usr/local/bin/uv

# Create Python virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy only storage metadata for kserve's editable path dep resolution
COPY storage/pyproject.toml storage/uv.lock storage/

# ------------------ Install kserve ------------------
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN --mount=type=cache,target=/root/.cache/uv cd kserve && uv sync --active

COPY kserve kserve
RUN --mount=type=cache,target=/root/.cache/uv cd kserve && uv sync --active

# ------------------ Install custom_tokenizer ------------------
COPY custom_tokenizer/pyproject.toml custom_tokenizer/uv.lock custom_tokenizer/
RUN --mount=type=cache,target=/root/.cache/uv cd custom_tokenizer && uv sync --active

COPY custom_tokenizer custom_tokenizer
RUN --mount=type=cache,target=/root/.cache/uv cd custom_tokenizer && uv sync --active

# Generate third-party licenses
COPY pyproject.toml pyproject.toml
COPY third_party/pip-licenses.py pip-licenses.py
RUN mkdir -p third_party/library && python3 pip-licenses.py


# ------------------ Final Production Image ------------------
FROM ${BASE_IMAGE} AS prod

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Create non-root user
RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve third_party third_party
COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder custom_tokenizer custom_tokenizer

USER 1000
ENV PYTHONPATH=/custom_tokenizer
ENTRYPOINT ["python", "-m", "custom_tokenizer.transformer"]
