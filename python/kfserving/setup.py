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

import setuptools

REQUIRES = [
    'certifi >= 14.05.14',
    'six >= 1.10',
    'python_dateutil >= 2.5.3',
    'setuptools >= 21.0.0',
    'urllib3 >= 1.15.1',
    'kubernetes >= 9.0.0',
    'tornado >= 1.4.1',
    'argparse >= 1.4.0',
    'minio >= 4.0.9',
    'google-cloud-storage >= 1.16.0',
    'azure-storage-blob >= 2.0.1',
    'numpy',
]

setuptools.setup(
    name='kfserving',
    version='0.0.2',
    author="Kubeflow Authors",
    author_email='ellisbigelow@google.com',
    license='../../LICENSE.txt',
    description="Python SDK for KFServing",
    url="https://github.com/kubeflow/kfserving/python/kfserving",
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=[
        'kfserving',
        #'kfserving.api',
        #'kfserving.constants',
        'kfserving.models',
        #'kfserving.utils'
    ],
    package_data={},
    include_package_data=False,
    zip_safe=False,
    classifiers=[
        'Intended Audience :: Developers',
        'Intended Audience :: Education',
        'Intended Audience :: Science/Research',
        'Programming Language :: Python :: 3',
        'Programming Language :: Python :: 3.5',
        'Programming Language :: Python :: 3.6',
        'Programming Language :: Python :: 3.7',
        "License :: OSI Approved :: Apache Software License",
        "Operating System :: OS Independent",
        'Topic :: Scientific/Engineering',
        'Topic :: Scientific/Engineering :: Artificial Intelligence',
        'Topic :: Software Development',
        'Topic :: Software Development :: Libraries',
        'Topic :: Software Development :: Libraries :: Python Modules',
    ],
    install_requires=REQUIRES,
)
