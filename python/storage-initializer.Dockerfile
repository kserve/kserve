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

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    gcc \
    libkrb5-dev \
    krb5-config \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir krbcontext==0.10 hdfs~=2.6.0 requests-kerberos==0.14.0


FROM python:3.9-slim-bullseye as prod

COPY third_party third_party

# activate virtual env
ENV VIRTUAL_ENV=/prod_venv
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY --from=builder $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY ./storage-initializer /storage-initializer

RUN chmod +x /storage-initializer/scripts/initializer-entrypoint
RUN mkdir /work
WORKDIR /work

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["/storage-initializer/scripts/initializer-entrypoint"]
