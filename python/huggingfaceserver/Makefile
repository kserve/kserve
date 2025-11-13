IPEX_EXTRA_INDEX_URL = https://pytorch-extension.intel.com/release-whl/stable/cpu/us/
TORCH_EXTRA_INDEX_URL = https://download.pytorch.org/whl/cpu
TORCH_VERSION ?= 2.7.0
TORCHVISION_VERSION ?= 0.22.0

dev_install:
	uv sync --active --group test

install_dependencies:
	uv sync --active --group test

install_cpu_dependencies:
	# Install PyTorch CPU stack using PyPI as main index and PyTorch CPU index as extra
	uv add --active \
		"torch~=${TORCH_VERSION}" \
		"torchaudio~=${TORCH_VERSION}" \
		"torchvision~=${TORCHVISION_VERSION}" \
		--index https://pypi.org/simple \
		--extra-index-url ${TORCH_EXTRA_INDEX_URL}

	uv lock

	# Install Intel Extension for PyTorch using IPEX index as extra
	uv add --active \
		"intel_extension_for_pytorch~=${TORCH_VERSION}" \
		--index https://pypi.org/simple \
		--extra-index-url ${IPEX_EXTRA_INDEX_URL}

	uv sync --active --no-cache --dev --group test

test: type_check
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports huggingfaceserver
