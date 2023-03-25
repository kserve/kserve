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

tests_require = [
    'pytest',
    'pytest-tornasync',
    'mypy'
]

with open(pathlib.Path(__file__).parent.parent / 'VERSION') as version_file:
    version = version_file.read().strip()

setup(
    name='alibiexplainer',
    version=version,
    author_email='cc@seldon.io',
    license='../../LICENSE.txt',
    url='https://github.com/kserve/kserve/python/alibiexplainer',
    description='Model Explanation Server. \
                 Not intended for use outside KServe Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>=3.7',
    packages=find_packages("alibiexplainer"),
    install_requires=[
        f"kserve[storage]>={version}",
        "nest_asyncio>=1.4.0",
        "alibi==0.6.4",
        "joblib>=0.13.2",
        "xgboost==1.6.1",
        "shap==0.41.0",
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
