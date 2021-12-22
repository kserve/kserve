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
    'pytest-tornasync',
    'mypy'
]
setup(
    name='artserver',
    version='0.7.0',
    author_email='Andrew.Butler@ibm.com',
    license='https://github.com/kserve/kserve/LICENSE',
    url='https://github.com/kserve/kserve/python/artserver',
    description='Model Server implementation for AI Robustness Toolbox. \
                 Not intended for use outside KServe Frameworks Images',
    python_requires='>3.7',
    packages=find_packages("artserver"),
    install_requires=[
        "kserve>=0.7.0",
        "argparse >= 1.4.0",
        "numpy >= 1.8.2",
        "adversarial-robustness-toolbox[keras] == 1.4.1",
        "nest_asyncio>=1.4.0"
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
