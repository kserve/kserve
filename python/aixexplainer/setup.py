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
    name='aixserver',
    version=version,
    author_email='Andrew.Butler@ibm.com',
    license='https://github.com/kserve/kserve/LICENSE',
    url='https://github.com/kserve/kserve/python/aixserver',
    description='Model Server implementation for AI eXplainability with LIME. \
                 Not intended for use outside KServe Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>3.7',
    packages=find_packages("aixserver"),
    install_requires=[
        f"kserve>={version}",
        "argparse >= 1.4.0",
        "aix360 >= 0.2.0",
        "lime >= 0.1.1.37",
        "nest_asyncio>=1.4.0",
        "cvxpy == 1.1.13"
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
