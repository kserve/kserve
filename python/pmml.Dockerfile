ARG PYTHON_VERSION=3.11
ARG JAVA_VERSION=21
ARG BASE_IMAGE=openjdk:${JAVA_VERSION}-slim-bookworm
ARG VENV_PATH=/prod_venv

FROM ${BASE_IMAGE} AS builder

ARG PYTHON_VERSION
# Install python
RUN apt-get update && \
    apt-get install -y --no-install-recommends "python${PYTHON_VERSION}" "python${PYTHON_VERSION}-dev" "python${PYTHON_VERSION}-venv" \
    curl \
    gcc build-essential && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install uv
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    ln -s /root/.local/bin/uv /usr/local/bin/uv

# Setup virtual environment
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Install dependencies for kserve using uv
COPY kserve/pyproject.toml kserve/uv.lock kserve/
RUN cd kserve && uv sync --active --no-cache
COPY kserve kserve

# Install dependencies for pmmlserver using uv
COPY pmmlserver/pyproject.toml pmmlserver/uv.lock pmmlserver/
RUN cd pmmlserver && uv sync --active --no-cache
COPY pmmlserver pmmlserver

# ---------- Production image ----------
FROM ${BASE_IMAGE} AS prod

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

# Create non-root user
RUN useradd kserve -m -u 1000 -d /home/kserve

# Copy venv and code from builder stage
COPY --from=builder --chown=kserve:kserve ${VIRTUAL_ENV} ${VIRTUAL_ENV}
COPY --from=builder kserve kserve
COPY --from=builder pmmlserver pmmlserver

USER 1000
ENTRYPOINT ["python3", "-m", "pmmlserver"]
