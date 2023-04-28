#!/usr/bin/env bash

set -e
set -x

python -m poetry publish --build
