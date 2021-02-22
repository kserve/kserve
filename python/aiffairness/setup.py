# Copyright 2019 kubeflow.org.
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
    name='aifserver',
    version='0.2.1',
    author_email='ryan.zegray@ibm.com',
    license='https://github.com/kubeflow/kfserving/LICENSE',
    url='https://github.com/kubeflow/kfserving/python/aifserver',
    description='Model Server implementation for AI fairness. \
                 Not intended for use outside KFServing Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages("aifserver"),
    install_requires=[
        "kfserving>=0.2.1",
        "argparse >= 1.4.0",
        "numpy >= 1.8.2",
        "aif360 >= 0.2.3",
        "nest_asyncio>=1.4.0",
        "requests[security]>=2.24.0"
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
