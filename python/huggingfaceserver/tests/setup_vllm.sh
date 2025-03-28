#!/bin/bash

set -e

echo "Installing vllm openvino"

VLLM_VERSION=v0.8.1
VLLM_DIR=vllm-clone

source $(poetry env info -p)/bin/activate

mkdir $VLLM_DIR
cd $VLLM_DIR
git clone --branch $VLLM_VERSION --depth 1 https://github.com/vllm-project/vllm.git .
pip install --upgrade pip && \
pip install -r requirements/build.txt --extra-index-url https://download.pytorch.org/whl/cpu && \
PIP_EXTRA_INDEX_URL="https://download.pytorch.org/whl/cpu" VLLM_TARGET_DEVICE="openvino" python -m pip install -v .

cd ..
rm -rf $VLLM_DIR
