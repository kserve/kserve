#!/bin/bash

set -e

VLLM_VERSION=v0.11.2
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

VENV_PATH="${VENV_PATH:-/mnt/python/huggingfaceserver-cpu-venv}"
source ${VENV_PATH}/bin/activate
mkdir $VLLM_DIR
cd $VLLM_DIR

git clone --branch $VLLM_VERSION --depth 1 https://github.com/vllm-project/vllm.git .

case $VLLM_TARGET_DEVICE in
    cpu)
        uv pip install -r requirements/cpu-build.txt --torch-backend cpu --index-strategy unsafe-best-match
        uv pip install -r requirements/cpu.txt --torch-backend cpu --index-strategy unsafe-best-match
        ;;
esac

VLLM_TARGET_DEVICE=${VLLM_TARGET_DEVICE} uv pip install . --no-build-isolation --index-strategy unsafe-best-match

cd ..
rm -rf $VLLM_DIR
