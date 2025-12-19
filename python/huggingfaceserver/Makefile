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
# The key issue: kserve[llm] depends on vllm==0.11.2 which requires torchvision==0.24.0,
# but torch==2.8.0+cpu requires torchvision==0.23.0+cpu. To avoid this conflict:
# - We temporarily replace kserve[llm] with kserve (no vllm dependency)
# - setup_vllm.sh must run LAST to ensure torch 2.8.0+cpu is not overwritten
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
