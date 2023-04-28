#!/usr/bin/env bash

set -e
set -x

mypy poetry_version_plugin
black poetry_version_plugin tests --check
isort poetry_version_plugin tests --check-only
