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
    'pytest-asyncio',
    'pytest-tornasync',
    'mypy'
]
setup(
    name='sklearnserver',
    version='0.4.0',
    author_email='singhan@us.ibm.com',
    license='https://github.com/kserve/kserve/LICENSE',
    url='https://github.com/kserve/kserve/python/sklearnserver',
    description='Model Server implementation for scikit-learn. \
                 Not intended for use outside KServe Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages("sklearnserver"),
    install_requires=[
        "kfserving>=0.4.0",
        "scikit-learn == 0.23.2",
        "argparse >= 1.4.0",
        "numpy >= 1.8.2",
        "joblib >= 0.13.0"
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
