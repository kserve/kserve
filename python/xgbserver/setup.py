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
import pathlib

from setuptools import setup, find_packages

with open(pathlib.Path(__file__).parent.parent / 'VERSION') as version_file:
    version = version_file.read().strip()

tests_require = [
    'pytest',
    'pytest-asyncio',
    'pytest-tornasync',
    'mypy'
]

setup(
    name='xgbserver',
    version=version,
    author_email='ellisbigelow@google.com',
    license='../../LICENSE.txt',
    url='https://github.com/kserve/kserve/python/xgbserver',
    description='Model Server implementation for XGBoost. \
                 Not intended for use outside KServe Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>3.7',
    packages=find_packages("xgbserver"),
    install_requires=[
        f"kserve[storage]>={version}",
        "xgboost == 1.5.0",
        "scikit-learn == 1.0.1",
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
