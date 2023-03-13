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

setup(
    name='paddleserver',
    version=version,
    author_email='zhangzhengyuan0604@gmail.com',
    license='https://github.com/kserve/kserve/LICENSE',
    description='Model Server implementation for Paddle. \
                 Not intended for use outside KServe Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages("paddleserver"),
    install_requires=[
        f"kserve[storage]>={version}",
        "paddlepaddle>=2.0.2"
    ],
    extras_require={'test': ['opencv-python']}
)
