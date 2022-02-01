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
    name='rfserver',
    version='0.2.1',
    author_email='Andrew.Butler@ibm.com',
    license='https://github.com/kserve/kserve/LICENSE',
    url='https://github.com/kserve/kserve/python/rfserver',
    description='Model Server implementation for AI eXplainability using LIME. \
                 Not intended for use outside KServe Frameworks Images',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages("rfserver"),
    install_requires=[
        "kserve",
        "scikit-learn",
        "scikit-image",
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
