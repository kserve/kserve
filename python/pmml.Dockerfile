ARG PYTHON_VERSION=3.9
ARG JAVA_VERSION=11
ARG BASE_IMAGE=openjdk:${JAVA_VERSION}-slim
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} as builder

ARG PYTHON_VERSION
# Install python
RUN apt-get update && \
    apt-get install -y --no-install-recommends "python${PYTHON_VERSION}" "python${PYTHON_VERSION}-dev" "python${PYTHON_VERSION}-venv" gcc && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.4.0

RUN python3 -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip install poetry==${POETRY_VERSION}
ENV PATH="$PATH:${POETRY_HOME}/bin"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python${PYTHON_VERSION} -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

COPY pmmlserver/pyproject.toml pmmlserver/poetry.lock pmmlserver/
RUN cd pmmlserver && poetry install --no-root --no-interaction --no-cache
COPY pmmlserver pmmlserver
RUN cd pmmlserver && poetry install --no-interaction --no-cache


FROM ${BASE_IMAGE} as prod

COPY third_party third_party

ARG PYTHON_VERSION
# Install python
RUN apt-get update && \
    apt-get install -y --no-install-recommends "python${PYTHON_VERSION}" && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder pmmlserver pmmlserver

USER 1000
ENTRYPOINT ["python3", "-m", "pmmlserver"]
