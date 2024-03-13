ARG PYTHON_VERSION=3.9
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bullseye
ARG VENV_PATH=venv
ARG WORK_DIR=/model-server

FROM ${BASE_IMAGE} as builder

RUN apt-get update && apt-get install -y gcc python3-dev

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
RUN python3 -m venv ${VENV_PATH}
ENV PATH="${VIRTUAL_ENV}/bin:${PATH}"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

RUN echo $(pwd)
RUN echo $(ls)
COPY test_resources/graph/error_404_isvc/pyproject.toml test_resources/graph/error_404_isvc/poetry.lock test_resources/graph/error_404_isvc/
RUN cd test_resources/graph/error_404_isvc && poetry install --no-root --no-interaction --no-cache
COPY test_resources/graph/error_404_isvc test_resources/graph/error_404_isvc
RUN cd test_resources/graph/error_404_isvc && poetry install --no-interaction --no-cache


FROM ${BASE_IMAGE} as prod

ARG WORK_DIR
WORKDIR ${WORK_DIR}

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000

# To activate virtual env we have to set VIRTUAL_ENV environment variable
ARG VENV_PATH
ENV VIRTUAL_ENV=${WORK_DIR}/${VENV_PATH}
ENV PATH="${VIRTUAL_ENV}/bin:${PATH}"

COPY third_party third_party
COPY --from=builder --chown=kserve:0 ${WORK_DIR} .

ENTRYPOINT ["python", "-m", "test_resources.graph.error_404_isvc.model"]
