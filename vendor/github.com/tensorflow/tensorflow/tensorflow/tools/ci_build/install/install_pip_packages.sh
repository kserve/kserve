#!/usr/bin/env bash
# Copyright 2015 The TensorFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ==============================================================================

set -e

# We don't apt-get install so that we can install a newer version of pip.
# Only needed for Ubuntu 14.04 and 16.04; not needed for 18.04 and Debian 8,9?
# Run easy_install before easy_install3, so that the default pip points to pip2,
# to match the default python version of 2.7.
easy_install3 -U pip==9.0.3
easy_install -U pip==9.0.3

# Install pip packages from whl files to avoid the time-consuming process of
# building from source.

# Pin wheel==0.31.1 to work around issue
# https://github.com/pypa/auditwheel/issues/102
pip2 install wheel==0.31.1
pip3 install wheel==0.31.1

# Install last working version of setuptools. This must happen before we install
# absl-py, which uses install_requires notation introduced in setuptools 20.5.
pip2 install --upgrade setuptools==39.1.0
pip3 install --upgrade setuptools==39.1.0

pip2 install virtualenv
pip3 install virtualenv

# Install six.
pip2 install --upgrade six==1.10.0
pip3 install --upgrade six==1.10.0

# Install absl-py.
pip2 install --upgrade absl-py
pip3 install --upgrade absl-py

# Install werkzeug.
pip2 install --upgrade werkzeug==0.11.10
pip3 install --upgrade werkzeug==0.11.10

# Install bleach. html5lib will be picked up as a dependency.
pip2 install --upgrade bleach==2.0.0
pip3 install --upgrade bleach==2.0.0

# Install markdown.
pip2 install --upgrade markdown==2.6.8
pip3 install --upgrade markdown==2.6.8

# Install protobuf.
pip2 install --upgrade protobuf==3.6.0
pip3 install --upgrade protobuf==3.6.0

# Remove obsolete version of six, which can sometimes confuse virtualenv.
rm -rf /usr/lib/python3/dist-packages/six*

# numpy needs to be installed from source to fix segfaults. See:
# https://github.com/tensorflow/tensorflow/issues/6968
# This workaround isn't needed for Ubuntu 16.04 or later.
if $(cat /etc/*-release | grep -q 14.04); then
  pip2 install --no-binary=:all: --upgrade numpy==1.14.5
  pip3 install --no-binary=:all: --upgrade numpy==1.14.5
else
  pip2 install --upgrade numpy==1.14.5
  pip3 install --upgrade numpy==1.14.5
fi

pip2 install scipy==1.1.0
pip3 install scipy==1.1.0

pip2 install scikit-learn==0.18.1
pip3 install scikit-learn==0.18.1

# pandas required by `inflow`
pip2 install pandas==0.19.2
pip3 install pandas==0.19.2

# Benchmark tests require the following:
pip2 install psutil
pip3 install psutil
pip2 install py-cpuinfo
pip3 install py-cpuinfo

# pylint tests require the following:
pip2 install pylint==1.6.4
pip3 install pylint==1.6.4

# pep8 tests require the following:
pip2 install pep8
pip3 install pep8

# tf.mock require the following for python2:
pip2 install mock

pip2 install portpicker
pip3 install portpicker

# TensorFlow Serving integration tests require the following:
pip2 install grpcio
pip3 install grpcio

# Eager-to-graph execution needs astor, gast and termcolor:
pip2 install --upgrade astor
pip3 install --upgrade astor
pip2 install --upgrade gast
pip3 install --upgrade gast
pip2 install --upgrade termcolor
pip3 install --upgrade termcolor

# Keras
pip2 install keras_applications==1.0.6 --no-deps
pip3 install keras_applications==1.0.6 --no-deps
pip2 install keras_preprocessing==1.0.5 --no-deps
pip3 install keras_preprocessing==1.0.5 --no-deps
pip2 install --upgrade h5py==2.8.0
pip3 install --upgrade h5py==2.8.0

# Estimator
pip2 install tensorflow_estimator --no-deps
pip3 install tensorflow_estimator --no-deps
