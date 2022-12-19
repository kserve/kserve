ARG BASE_IMAGE=python:3.9-slim-bullseye
FROM $BASE_IMAGE as builder

# pip 20.x breaks xgboost wheels https://github.com/dmlc/xgboost/issues/5221
#RUN pip install --no-cache-dir pip==19.3.1 && pip install --no-cache-dir -e ./kserve

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

COPY xgbserver/pyproject.toml xgbserver/poetry.lock xgbserver/
RUN cd xgbserver && poetry install --no-root --no-interaction --no-cache
COPY xgbserver xgbserver
RUN cd xgbserver && poetry install --no-interaction --no-cache


FROM python:3.9-slim-bullseye as prod

COPY third_party third_party

RUN apt-get update && \
    apt-get install -y --no-install-recommends libgomp1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# activate virtual env
ENV VIRTUAL_ENV=/prod_venv
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY --from=builder $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder xgbserver xgbserver

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "xgbserver"]

