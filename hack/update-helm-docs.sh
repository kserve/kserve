#!/bin/bash

set +e

echo "Installing pre-commit"
pip install pre-commit==3.6.1

echo "Installing helm-docs"
go install github.com/norwoodj/helm-docs/cmd/helm-docs@v1.12.0

pre-commit install; pre-commit run --all
