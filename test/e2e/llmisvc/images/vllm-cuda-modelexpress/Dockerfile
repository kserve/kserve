ARG BASE_IMAGE=vllm/vllm-openai:v0.31.0
FROM ${BASE_IMAGE}

ARG MODELEXPRESS_VERSION=0.6.0
ARG RUN_USER=2000
RUN uv pip freeze --system | grep -E '^[A-Za-z0-9_.-]+==' > /tmp/base-constraints.txt && \
    uv pip install --system --no-cache -c /tmp/base-constraints.txt \
        "modelexpress==${MODELEXPRESS_VERSION}" && \
    rm /tmp/base-constraints.txt && \
    python3 -c "import modelexpress, runai_model_streamer, vllm; print(vllm.__version__)" && \
    mkdir -p /home/vllm && \
    chown ${RUN_USER}:0 /home/vllm && \
    chmod 0775 /home/vllm
ENV HOME=/home/vllm
USER ${RUN_USER}
