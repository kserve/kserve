# vLLM CPU image plus NIXL, so NixlConnector can move the KV cache from the prefill
# pod to the decode pod over UCX/TCP. See README.md for why each step is here.
ARG VLLM_CPU_IMAGE=vllm/vllm-openai-cpu:v0.19.0
FROM ${VLLM_CPU_IMAGE}

# vLLM v0.19.0 declares nixl >= 0.7.1, < 0.10.0, but every nixl-cu12 wheel in that
# range (x86_64 and aarch64) links its UCX plugin against libcuda.so.1, so UCX never
# loads on a CPU image. 0.10.0 is the first release without that link, and vLLM
# v0.20.0 accepts <= 0.10.1.
# nixl is a meta package: pin the backing nixl-cu12 wheel too, or pip resolves the
# newest one.
ARG NIXL_VERSION=0.10.1
RUN pip install --no-cache-dir "nixl==${NIXL_VERSION}" "nixl-cu12==${NIXL_VERSION}" \
 && python3 -c "from nixl._api import nixl_agent, nixl_agent_config; \
a = nixl_agent('build-check', nixl_agent_config(backends=['UCX'])); \
assert 'UCX' in a.get_plugin_list(), a.get_plugin_list(); print('NIXL UCX backend OK')"

# Apple silicon (Docker Desktop) only. vLLM's oneDNN GEMM kills the engine core with
# SIGILL in create_onednn_mm there, with or without NIXL. true makes vLLM take its own
# torch.nn.functional.linear fallback. Leave it false everywhere else.
ARG DISABLE_ONEDNN_GEMM=false
RUN if [ "${DISABLE_ONEDNN_GEMM}" = "true" ]; then \
      f="$(python3 -c 'import os, vllm; print(os.path.dirname(vllm.__file__))')/_custom_ops.py" \
      && sed -i 's/^_supports_onednn = .*/_supports_onednn = False/' "$f" \
      && grep -q '^_supports_onednn = False' "$f" \
      && echo "oneDNN GEMM disabled"; \
    fi
