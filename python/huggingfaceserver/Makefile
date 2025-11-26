IPEX_EXTRA_INDEX_URL = https://pytorch-extension.intel.com/release-whl/stable/cpu/us/
TORCH_EXTRA_INDEX_URL = https://download.pytorch.org/whl/cpu
TORCH_VERSION ?= 2.9.0
TORCHVISION_VERSION ?= 0.24.0
IPEX_VERSION ?= 2.8.0

dev_install:
	uv sync --active --group test

install_dependencies:
	uv sync --active --group test

# Install CPU-specific PyTorch packages
# Note: We use exact version with +cpu suffix to ensure CPU-only wheels are installed.
# The --index-strategy unsafe-best-match allows uv to find +cpu versions from PyTorch index.
# markupsafe is constrained to avoid version 3.0.2 which lacks cp312 wheels on PyTorch index.
# IPEX version must match PyTorch major.minor version (e.g., IPEX 2.8.x for PyTorch 2.8.x).
install_cpu_dependencies:
	uv add --active \
		"torch==${TORCH_VERSION}+cpu" \
		"torchaudio==${TORCH_VERSION}+cpu" \
		"torchvision==${TORCHVISION_VERSION}+cpu" \
		"markupsafe>=2.0.0,<3.0.0" \
		--index ${TORCH_EXTRA_INDEX_URL} \
		--index-strategy unsafe-best-match

	uv lock

	uv add --active \
		"intel_extension_for_pytorch==${IPEX_VERSION}+cpu" \
		--index ${IPEX_EXTRA_INDEX_URL} \
		--index-strategy unsafe-best-match

	uv sync --active --no-cache --dev --group test

test: type_check
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports huggingfaceserver 
