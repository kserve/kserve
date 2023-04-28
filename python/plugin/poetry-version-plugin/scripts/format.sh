#!/usr/bin/env bash

set -e
set -x

autoflake --remove-all-unused-imports --recursive --remove-unused-variables --in-place poetry_version_plugin tests
black poetry_version_plugin tests
isort poetry_version_plugin tests
