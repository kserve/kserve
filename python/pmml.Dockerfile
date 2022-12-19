ARG BASE_IMAGE=openjdk:11-slim
FROM $BASE_IMAGE as builder

ENV PYTHON="python3.9"
# install python
RUN apt-get update && \
    apt-get install -y --no-install-recommends $PYTHON ${PYTHON}-dev ${PYTHON}-venv && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

ENV POETRY_VERSION=1.3.1 \
    POETRY_HOME=/opt/poetry
RUN $PYTHON -m venv $POETRY_HOME && $POETRY_HOME/bin/pip install poetry==$POETRY_VERSION
ENV PATH="$PATH:$POETRY_HOME/bin"

# activate virtual env
ENV VIRTUAL_ENV=/prod_venv
RUN $PYTHON -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache

COPY pmmlserver/pyproject.toml pmmlserver/poetry.lock pmmlserver/
RUN cd pmmlserver && poetry install --no-root --no-interaction --no-cache
COPY pmmlserver pmmlserver
RUN cd pmmlserver && poetry install --no-interaction --no-cache


FROM openjdk:11-slim as prod

COPY third_party third_party

ENV PYTHON="python3.9"
# install python
RUN apt-get update && \
    apt-get install -y --no-install-recommends $PYTHON && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# activate virtual env
ENV VIRTUAL_ENV=/prod_venv
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY --from=builder $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder pmmlserver pmmlserver

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python3", "-m", "pmmlserver"]