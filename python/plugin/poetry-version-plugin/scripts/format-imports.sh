#!/usr/bin/env bash

set -e
set -x

# Sort imports one per line, so autoflake can remove unused imports
isort --force-single-line-imports poetry_version_plugin tests
sh ./scripts/format.sh
