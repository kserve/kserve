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
    name='kfserver',
    version='0.1.0',
    author_email='ellisbigelow@google.com',
    license='../../LICENSE.txt',
    url='https://github.com/kubeflow/kfserving/python/kfserving/kfserving',
    description='Model Server for arbitrary python ML frameworks.',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages("kfserving"),
    install_requires=[
        "tornado >= 1.4.1",
        "argparse >= 1.4.0",
        "minio >= 4.0.9",
        "google-cloud-storage >= 1.16.0",
        "numpy",
        "azure-storage-blob >= 2.0.1"
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
