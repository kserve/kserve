ARG PYTHON_VERSION=3.9
ARG JAVA_VERSION=11
ARG BASE_IMAGE=openjdk:${JAVA_VERSION}-slim
ARG VENV_PATH=venv
ARG WORK_DIR=/model-server

FROM ${BASE_IMAGE} as builder

ARG PYTHON_VERSION
# Install python
RUN apt-get update && \
    apt-get install -y --no-install-recommends "python${PYTHON_VERSION}" "python${PYTHON_VERSION}-dev" "python${PYTHON_VERSION}-venv" gcc && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.7.1

RUN python3 -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip install poetry==${POETRY_VERSION}
ENV PATH="$PATH:${POETRY_HOME}/bin"

ARG WORK_DIR
WORKDIR ${WORK_DIR}
RUN chmod -R g=u ${WORK_DIR}

# To activate virtual env we have to set VIRTUAL_ENV environment variable
ARG VENV_PATH
ENV VIRTUAL_ENV="${WORK_DIR}/${VENV_PATH}"
RUN python${PYTHON_VERSION} -m venv ${VENV_PATH}
ENV PATH="${VIRTUAL_ENV}/bin:${PATH}"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

COPY pmmlserver/pyproject.toml pmmlserver/poetry.lock pmmlserver/
RUN cd pmmlserver && poetry install --no-root --no-interaction --no-cache
COPY pmmlserver pmmlserver
RUN cd pmmlserver && poetry install --no-interaction --no-cache


FROM ${BASE_IMAGE} as prod

ARG WORK_DIR
WORKDIR ${WORK_DIR}

ARG PYTHON_VERSION
# Install python
RUN apt-get update && \
    apt-get install -y --no-install-recommends "python${PYTHON_VERSION}" && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${WORK_DIR}/${VENV_PATH}
ENV PATH="${VIRTUAL_ENV}/bin:${PATH}"

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000

COPY third_party third_party
COPY --from=builder --chown=kserve:0 ${WORK_DIR} .

ENTRYPOINT ["python3", "-m", "pmmlserver"]
