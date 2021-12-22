# Copyright 2021 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from setuptools import setup

tests_require = [
    'pytest',
    'pytest-tornasync',
    'mypy'
]

with open('requirements.txt') as f:
    REQUIRES = f.readlines()

setup(
    name='grpc_image_transformer',
    version='0.1.0',
    author_email='dsun20@bloomberg.net',
    url='https://github.com/kserve/kserve/python/custom_transformer',
    description='gRPCImageTransformer',
    python_requires='>=3.7',
    install_requires=REQUIRES,
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
