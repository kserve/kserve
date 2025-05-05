#!/bin/bash

set -e

TORCH_EXTRA_INDEX_URL="https://download.pytorch.org/whl/cpu"
VLLM_VERSION=v0.8.5
VLLM_DIR=vllm-clone
VLLM_TARGET_DEVICE="${VLLM_TARGET_DEVICE:-cpu}"

case $VLLM_TARGET_DEVICE in
  cpu)
    echo "Installing vllm for CPU"
    ;;
  *)
    echo "Unknown target device: $VLLM_TARGET_DEVICE"
    exit 1
      ;;
esac

source $(poetry env info -p)/bin/activate
mkdir $VLLM_DIR
cd $VLLM_DIR

git clone --branch $VLLM_VERSION --depth 1 https://github.com/vllm-project/vllm.git .
pip install --upgrade pip

case $VLLM_TARGET_DEVICE in
    cpu)
        pip uninstall -y torch torchvision torchaudio && \
        pip install -r requirements/build.txt -r requirements/cpu.txt --extra-index-url ${TORCH_EXTRA_INDEX_URL}
        ;;
esac

PIP_EXTRA_INDEX_URL=${TORCH_EXTRA_INDEX_URL} VLLM_TARGET_DEVICE=${VLLM_TARGET_DEVICE} python -m pip install -v .

cd ..
rm -rf $VLLM_DIR
