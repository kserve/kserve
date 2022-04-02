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

from setuptools import setup, find_packages

tests_require = [
    'pytest',
    'pytest-asyncio',
    'pytest-tornasync',
    'mypy'
]

setup(
    name='lgbserver',
    version='0.7.0',
    author_email='linyiming@ainnovation.com',
    license='../../LICENSE.txt',
    url='https://github.com/kserve/kserve/python/lgbserver',
    description='Model Server implementation for LightGBM. \
                 Not intended for use outside KServe Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages("lgbserver"),
    install_requires=[
        "kserve>=0.7.0",
        "lightgbm == 3.3.2",
        "pandas == 1.3.5",
        "argparse >= 1.4.0",
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
