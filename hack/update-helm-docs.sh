#!/bin/bash
set -e

echo "Installing pre-commit"
pip install pre-commit

echo "Installing helm-docs"
go install github.com/norwoodj/helm-docs/cmd/helm-docs@latest

pre-commit install; pre-commit run --all
