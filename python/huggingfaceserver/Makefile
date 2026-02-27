IPEX_EXTRA_INDEX_URL = https://pytorch-extension.intel.com/release-whl/stable/cpu/us/
TORCH_EXTRA_INDEX_URL = https://download.pytorch.org/whl/cpu

dev_install:
	uv sync --active --group test

install_dependencies:
	uv sync --active --group test

# CPU build workflow:
# 1. First run install_cpu_dependencies to install huggingfaceserver dependencies
# 2. Then run setup_vllm.sh LAST to build vLLM from source with torch CPU
#
# kserve[llm] pulls vllm from PyPI as a pre-built GPU wheel, which brings in GPU
# torch. The CPU build instead needs vllm built from source with torch+cpu. These
# two installation paths are incompatible, so we temporarily strip the [llm] extra
# to prevent PyPI vllm (and GPU torch) from being installed. setup_vllm.sh then
# handles the entire vllm + torch CPU installation from source.
install_cpu_dependencies:
	@echo "Replacing kserve[llm] with kserve to avoid vllm dependency conflict..."
	sed -i 's/kserve\[llm\]/kserve/' pyproject.toml
	uv lock
	@echo "Installing huggingfaceserver with test dependencies..."
	uv sync --active --group test
	@echo "Restoring original pyproject.toml..."
	sed -i 's/"kserve @/"kserve[llm] @/' pyproject.toml

test: type_check
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports huggingfaceserver 
