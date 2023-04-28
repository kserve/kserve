#!/usr/bin/env bash

set -e
set -x

coverage run --source=poetry_version_plugin,tests -m pytest "${@}"
coverage combine
coverage xml
