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
    name='sklearnserver',
    version=version,
    author_email='singhan@us.ibm.com',
    license='https://github.com/kserve/kserve/LICENSE',
    url='https://github.com/kserve/kserve/python/sklearnserver',
    description='Model Server implementation for scikit-learn. \
                 Not intended for use outside KServe Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>3.7',
    packages=find_packages("sklearnserver"),
    install_requires=[
        f"kserve>={version}",
        f"kserve-storage>={version}",
        "scikit-learn == 1.0.1",
        "joblib >= 0.13.0",
        "pandas >= 1.3.5"
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
