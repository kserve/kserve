IPEX_EXTRA_INDEX_URL = https://pytorch-extension.intel.com/release-whl/stable/cpu/us/
TORCH_EXTRA_INDEX_URL = https://download.pytorch.org/whl/cpu
TORCH_VERSION ?= 2.6.0
TORCHVISION_VERSION ?= 0.21.0

dev_install:
	poetry install --with test --no-interaction

install_dependencies:
	poetry install --with=test --no-interaction --no-root
	
install_cpu_dependencies:
	poetry source add --priority=supplemental pytorch-cpu ${TORCH_EXTRA_INDEX_URL}
	poetry add --source pytorch-cpu 'torch~='${TORCH_VERSION} 'torchaudio~='${TORCH_VERSION} 'torchvision~='${TORCHVISION_VERSION}
	poetry lock --no-update
	pip install --no-cache-dir --extra-index-url ${TORCH_EXTRA_INDEX_URL} --extra-index-url ${IPEX_EXTRA_INDEX_URL} 'intel_extension_for_pytorch~='${TORCH_VERSION}
	poetry install --with=test --no-interaction --no-root

test: type_check
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports huggingfaceserver 
