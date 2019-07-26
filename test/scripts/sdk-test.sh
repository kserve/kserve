#!/bin/bash

# Run KFServing SDK unit tests
pip install --upgrade pytest

pushd python/kfserving/test >/dev/null
  pytest
popd

