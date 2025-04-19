ARG PYTHON_VERSION=3.11
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.8.3

# Required for building packages for arm64 arch
RUN apt-get update && apt-get install -y --no-install-recommends python3-dev build-essential && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN python3 -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip install poetry==${POETRY_VERSION}
ENV PATH="$PATH:${POETRY_HOME}/bin"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

COPY sklearnserver/pyproject.toml sklearnserver/poetry.lock sklearnserver/
RUN cd sklearnserver && poetry install --no-root --no-interaction --no-cache
COPY sklearnserver sklearnserver
RUN cd sklearnserver && poetry install --no-interaction --no-cache


FROM ${BASE_IMAGE} AS prod

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

# Make all files also owned by the root group
COPY --from=builder --chown=1000:0 $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder --chown=1000:0 kserve kserve/
COPY --from=builder --chown=1000:0 sklearnserver sklearnserver/

# Give users in the root group the same permissions as the users
# to allow running the image with arbitrary UIDs
RUN chmod -R g=u "$VIRTUAL_ENV" kserve sklearnserver

USER 1000
ENTRYPOINT ["python", "-m", "sklearnserver"]
