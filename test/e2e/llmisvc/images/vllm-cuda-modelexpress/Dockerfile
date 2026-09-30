ARG BASE_IMAGE=ghcr.io/llm-d/llm-d-cuda:v0.9.0
FROM ${BASE_IMAGE}

ARG MODELEXPRESS_VERSION=0.6.0
ARG BASE_USER=2000
USER root
RUN (python3 -m pip --version >/dev/null 2>&1 || python3 -m ensurepip) && \
    python3 -m pip freeze | grep -E '^[A-Za-z0-9_.-]+==' > /tmp/base-constraints.txt && \
    python3 -m pip install --no-cache-dir -c /tmp/base-constraints.txt \
        "modelexpress==${MODELEXPRESS_VERSION}" && \
    rm /tmp/base-constraints.txt && \
    python3 -c "import modelexpress, runai_model_streamer, vllm; print(vllm.__version__)"
USER ${BASE_USER}
