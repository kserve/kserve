ARG PYTHON_VERSION=3.7
ARG BASE_IMAGE=python:${PYTHON_VERSION}-slim-bullseye
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} as builder

# Install Poetry
ARG POETRY_HOME=/opt/poetry
# FIXME: aixexplainer installation fails with poetry 1.4.0
ARG POETRY_VERSION=1.3.2

RUN python3 -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip install poetry==${POETRY_VERSION}
ENV PATH="$PATH:${POETRY_HOME}/bin"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry version $(cat ${VERSION}) && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry version $(cat ${VERSION}) && poetry install --no-interaction --no-cache

RUN apt update && apt install -y build-essential

COPY aixexplainer/pyproject.toml aixexplainer/poetry.lock aixexplainer/
RUN cd aixexplainer && poetry version $(cat ${VERSION}) && poetry install --no-root --no-interaction --no-cache
COPY aixexplainer aixexplainer
RUN cd aixexplainer && poetry version $(cat ${VERSION}) && poetry install --no-interaction --no-cache


FROM ${BASE_IMAGE} as prod

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY --from=builder $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder aixexplainer aixexplainer

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "aixserver"]
