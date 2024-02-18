#!/bin/bash

set +e

echo "Installing helm-docs"
go install github.com/norwoodj/helm-docs/cmd/helm-docs@v1.12.0

helm-docs --chart-search-root=charts --output-file=README.md
