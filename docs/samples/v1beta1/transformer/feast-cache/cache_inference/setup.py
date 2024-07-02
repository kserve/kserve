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

tests_require = ["pytest", "mypy"]

setup(
    name="cache_inference",
    version="1.0.0",
    author_email="chhuang@us.ibm.com",
    license="../../LICENSE.txt",
    url="https://github.com/kserve/kserve/docs/samples/v1beta1/transformer/feast/cache_inference",
    description="Driver transformer",
    python_requires=">=3.9",
    packages=find_packages("cache_inference"),
    install_requires=["kserve", "requests>=2.22.0", "numpy>=1.16.3"],
    tests_require=tests_require,
    extras_require={"test": tests_require},
)
