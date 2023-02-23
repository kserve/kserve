# Copyright 2023 The KServe Authors.
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

import setuptools

TESTS_REQUIRES = [
    'pytest',
    'pytest-cov',
    'mypy',
]

with open('requirements.txt') as f:
    REQUIRES = f.readlines()

with open(pathlib.Path(__file__).parent.parent / 'VERSION') as version_file:
    version = version_file.read().strip()

setuptools.setup(
    name='kserve-storage',
    version=version,
    author="The KServe Authors",
    author_email='ellisbigelow@google.com, hejinchi@cn.ibm.com, dsun20@bloomberg.net',
    license="Apache License Version 2.0",
    url="https://github.com/kserve/kserve/tree/master/python/storage-initializer",
    description="Python module for kserve storage initializer.",
    long_description="Kserve storage is used by storage initializer for downloading models from various storage \
                      providers.",
    python_requires='>=3.7',
    package_data={'': ['requirements.txt']},
    include_package_data=True,
    zip_safe=False,
    classifiers=[
        'Intended Audience :: Developers',
        'Intended Audience :: Education',
        'Intended Audience :: Science/Research',
        'Programming Language :: Python :: 3',
        'Programming Language :: Python :: 3.7',
        'Programming Language :: Python :: 3.8',
        'Programming Language :: Python :: 3.9',
        "License :: OSI Approved :: Apache Software License",
        "Operating System :: OS Independent",
        'Topic :: Scientific/Engineering',
        'Topic :: Scientific/Engineering :: Artificial Intelligence',
        'Topic :: Software Development',
        'Topic :: Software Development :: Libraries',
        'Topic :: Software Development :: Libraries :: Python Modules',
    ],
    install_requires=REQUIRES,
    tests_require=TESTS_REQUIRES,
    extras_require={'test': TESTS_REQUIRES}
)
