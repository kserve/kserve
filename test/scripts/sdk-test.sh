#!/bin/bash

# Run KFServing SDK unit tests
pip install --upgrade pytest

pushd sdk/test >/dev/null
  pytest
popd