ARG BASE_IMAGE=python:3.9-slim-bullseye
FROM $BASE_IMAGE as builder

ENV POETRY_VERSION=1.3.1 \
    POETRY_HOME=/opt/poetry
RUN python3 -m venv $POETRY_HOME && $POETRY_HOME/bin/pip install poetry==$POETRY_VERSION
ENV PATH="$PATH:$POETRY_HOME/bin"

# activate virtual env
ENV VIRTUAL_ENV=/prod_venv
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

COPY custom_transformer/pyproject.toml custom_transformer/poetry.lock custom_transformer/
RUN cd custom_transformer && poetry install --no-root --no-interaction --no-cache
COPY custom_transformer custom_transformer
RUN cd custom_transformer && poetry install --no-interaction --no-cache


FROM python:3.9-slim-bullseye as prod

COPY third_party third_party

# activate virtual env
ENV VIRTUAL_ENV=/prod_venv
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY --from=builder $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder custom_transformer custom_transformer

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "custom_transformer.model_grpc"]


